package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var Timestamp = fmt.Sprintf("%d", time.Now().Unix())

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	content := `
auth_token: test_token
auth_token_storage: sessionStorage
prettier_path: /usr/local/bin/prettier
hooks: react-query
use_date_object: true
packages:
  - path: ./internal/api
    output_path: ./frontend/src/api.generated.ts
    type_mappings:
      CustomType: string
  - path: ./internal/models
    output_path: ./frontend/src/models.generated.ts
    type_mappings:
      UUID: string
`
	tmpfile, err := os.CreateTemp("", "go2type-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Test loading the config
	config, err := loadConfig(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify the loaded config
	if config.AuthToken != "test_token" {
		t.Errorf("Expected AuthToken to be 'test_token', got '%s'", config.AuthToken)
	}

	if config.AuthTokenStorage != "sessionStorage" {
		t.Errorf("Expected AuthTokenStorage to be 'sessionStorage', got '%s'", config.AuthTokenStorage)
	}
	if config.PrettierPath != "/usr/local/bin/prettier" {
		t.Errorf("Expected PrettierPath to be '/usr/local/bin/prettier', got '%s'", config.PrettierPath)
	}
	if config.Hooks != "react-query" {
		t.Errorf("Expected Hooks to be 'react-query', got '%s'", config.Hooks)
	}
	if !config.UseDateObject {
		t.Errorf("Expected UseDateObject to be true")
	}
	if len(config.Packages) != 2 {
		t.Errorf("Expected 2 packages, got %d", len(config.Packages))
	}

	// Check first package
	if config.Packages[0].Path != "./internal/api" {
		t.Errorf("Expected first package path to be './internal/api', got '%s'", config.Packages[0].Path)
	}
	if config.Packages[0].OutputPath != "./frontend/src/api.generated.ts" {
		t.Errorf("Expected first package output path to be './frontend/src/api.generated.ts', got '%s'", config.Packages[0].OutputPath)
	}
	if config.Packages[0].TypeMappings["CustomType"] != "string" {
		t.Errorf("Expected CustomType mapping to be 'string', got '%s'", config.Packages[0].TypeMappings["CustomType"])
	}

	// Check second package
	if config.Packages[1].Path != "./internal/models" {
		t.Errorf("Expected second package path to be './internal/models', got '%s'", config.Packages[1].Path)
	}
	if config.Packages[1].OutputPath != "./frontend/src/models.generated.ts" {
		t.Errorf("Expected second package output path to be './frontend/src/models.generated.ts', got '%s'", config.Packages[1].OutputPath)
	}
	if config.Packages[1].TypeMappings["UUID"] != "string" {
		t.Errorf("Expected UUID mapping to be 'string', got '%s'", config.Packages[1].TypeMappings["UUID"])
	}

	// Test default value for AuthTokenStorage
	defaultConfig := `
auth_token: default_token
prettier_path: /usr/local/bin/prettier
`
	tmpfile2, err := os.CreateTemp("", "go2type-config-default-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file for default config: %v", err)
	}
	defer func() { _ = os.Remove(tmpfile2.Name()) }()

	if _, err := tmpfile2.Write([]byte(defaultConfig)); err != nil {
		t.Fatalf("Failed to write to temp file for default config: %v", err)
	}
	if err := tmpfile2.Close(); err != nil {
		t.Fatalf("Failed to close temp file for default config: %v", err)
	}

	defaultLoadedConfig, err := loadConfig(tmpfile2.Name())
	if err != nil {
		t.Fatalf("Failed to load default config: %v", err)
	}

	if defaultLoadedConfig.AuthTokenStorage != "localStorage" {
		t.Errorf("Expected default AuthTokenStorage to be 'localStorage', got '%s'", defaultLoadedConfig.AuthTokenStorage)
	}
}

func TestParsePackage(t *testing.T) {
	// Create a temporary directory for the test module
	tmpdir := createTempFolder(t.Name())

	// Defer cleanup, but only if the test passes
	defer func() {
		if !t.Failed() {
			t.Logf("Cleaning up temporary directory: %s", tmpdir)
			_ = os.RemoveAll(tmpdir)
		} else {
			t.Logf("Test failed. Temporary directory retained at: %s", tmpdir)
		}
	}()

	// Set up the module structure
	modulePath := filepath.Join(tmpdir, "testmodule")
	err := os.MkdirAll(filepath.Join(modulePath, "internal", "models"), 0755)
	if err != nil {
		t.Fatalf("Failed to create module structure: %v", err)
	}

	// Create go.mod file
	goModContent := `module github.com/example/testmodule

go 1.16
`
	err = os.WriteFile(filepath.Join(modulePath, "go.mod"), []byte(goModContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write go.mod file: %v", err)
	}

	// Create main.go with modified User struct
	mainContent := `
package main

import (
	"time"
	"github.com/example/testmodule/internal/models"
)

type StringArray []string

type User struct {
	ID        int                ` + "`json:\"id\"`" + `
	Name      string             ` + "`json:\"name\"`" + `
	CreatedAt time.Time          ` + "`json:\"created_at\"`" + `
	Tags      StringArray        ` + "`json:\"tags\"`" + `
	Info      models.UserInfo    ` + "`json:\"info\"`" + `
	InfoArray []models.UserInfo  ` + "`json:\"info_array\"`" + `
}

// @Method GET
// @Path /users/:id
// @Header localStorage:X-Auth-Token:auth_token
// @Header localStorage:X-Custom-Header
// @Input GetUserInput
// @Output User
func GetUserHandler() {}

// @Method POST
// @Path /users
// @Header sessionStorage:X-Session-ID:session_id
// @Header input:Content-Type
// @Input CreateUserInput
// @Output User
func CreateUserHandler() {}
`
	err = os.WriteFile(filepath.Join(modulePath, "main.go"), []byte(mainContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write main.go file: %v", err)
	}

	// Create internal/models/user_info.go
	userInfoContent := `
package models

type UserInfo struct {
	Email string ` + "`json:\"email\"`" + `
	Age   int    ` + "`json:\"age\"`" + `
}
`
	err = os.WriteFile(filepath.Join(modulePath, "internal", "models", "user_info.go"), []byte(userInfoContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write user_info.go file: %v", err)
	}

	// Run go mod tidy in modulePath
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = modulePath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to run go mod tidy: %v\nOutput: %s", err, output)
	}

	// Get the module info
	moduleName, modulePath, err := getModuleInfo(modulePath)
	if err != nil {
		t.Fatalf("Failed to get module info: %v", err)
	}

	if moduleName == "" {
		t.Fatalf("Failed to get module name")
	}

	// Test parsing the package
	customTypeMappings := map[string]string{
		"time.Time":   "Date",
		"StringArray": "Array<string>",
	}
	types, handlers, err := parsePackage(modulePath, customTypeMappings, true)
	if err != nil {
		t.Fatalf("Failed to parse package: %v", err)
	}

	// Verify the parsed types
	expectedTypes := []TypeInfo{
		{
			Name: "User",
			Fields: []FieldInfo{
				{PackageName: "int", Name: "id", Type: "number", JSONName: "id", IsOptional: false},
				{PackageName: "string", Name: "name", Type: "string", JSONName: "name", IsOptional: false},
				{PackageName: "time.Time", Name: "created_at", Type: "Date", JSONName: "created_at", IsOptional: false},
				{PackageName: "StringArray", Name: "tags", Type: "Array<string>", JSONName: "tags", IsOptional: false},
				{PackageName: "models.UserInfo", Name: "info", Type: "ModelsUserInfo", JSONName: "info", IsOptional: false},
				{PackageName: "models.UserInfo", Name: "info_array", Type: "Array<ModelsUserInfo>", JSONName: "info_array", IsOptional: false, IsArray: true},
			},
		},
		{
			Name: "ModelsUserInfo",
			Fields: []FieldInfo{
				{PackageName: "string", Name: "email", Type: "string", JSONName: "email", IsOptional: false},
				{PackageName: "int", Name: "age", Type: "number", JSONName: "age", IsOptional: false},
			},
		},
	}

	// Compare types, ignoring order
	if !compareTypes(types, expectedTypes) {
		t.Errorf("Parsed types do not match expected.\nGot: %+v\nWant: %+v", types, expectedTypes)
	}

	// Verify the parsed handlers
	expectedHandlers := []HandlerInfo{
		{
			Name:       "GetUser",
			Method:     "GET",
			Path:       "/users/:id",
			InputType:  "GetUserInput",
			OutputType: "User",
			URLParams:  []string{"id"},
			Headers: []HeaderInfo{
				{
					HeaderKey:  "X-Auth-Token",
					SafeName:   "x_auth_token",
					Source:     "localStorage",
					StorageKey: "auth_token",
				},
				{
					HeaderKey:  "X-Custom-Header",
					SafeName:   "x_custom_header",
					Source:     "localStorage",
					StorageKey: "X-Custom-Header", // Default to header key
				},
			},
		},
		{
			Name:       "CreateUser",
			Method:     "POST",
			Path:       "/users",
			InputType:  "CreateUserInput",
			OutputType: "User",
			Headers: []HeaderInfo{
				{
					HeaderKey:  "X-Session-ID",
					SafeName:   "x_session_id",
					Source:     "sessionStorage",
					StorageKey: "session_id",
				},
				{
					HeaderKey:  "Content-Type",
					SafeName:   "content_type",
					Source:     "input",
					StorageKey: "",
				},
			},
		},
	}

	if !reflect.DeepEqual(handlers, expectedHandlers) {
		t.Errorf("Parsed handlers do not match expected.\nGot: %+v\nWant: %+v", handlers, expectedHandlers)
	}
}

// Helper function to compare types regardless of order
func compareTypes(got, want []TypeInfo) bool {
	if len(got) != len(want) {
		return false
	}

	gotMap := make(map[string]TypeInfo)
	for _, t := range got {
		gotMap[t.Name] = t
	}

	for _, w := range want {
		g, ok := gotMap[w.Name]
		if !ok {
			return false
		}
		if !reflect.DeepEqual(g, w) {
			return false
		}
	}

	return true
}

func TestSetupNpmProject(t *testing.T) {
	// Skip if npm is not installed
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not found, skipping test")
	}

	// Create a temporary directory
	tmpdir := createTempFolder(t.Name())

	// Defer cleanup, but only if the test passes
	defer func() {
		if !t.Failed() {
			t.Logf("Cleaning up temporary directory: %s", tmpdir)
			_ = os.RemoveAll(tmpdir)
		} else {
			t.Logf("Test failed. Temporary directory retained at: %s", tmpdir)
		}
	}()

	// Run setupNpmProject
	if err := setupNpmProject(tmpdir); err != nil {
		t.Fatalf("Failed to setup npm project: %v", err)
	}

	// Check if package.json was created
	packageJSONPath := filepath.Join(tmpdir, "package.json")
	if _, err := os.Stat(packageJSONPath); os.IsNotExist(err) {
		t.Errorf("package.json was not created")
	}

	// Read and parse package.json
	content, err := os.ReadFile(packageJSONPath)
	if err != nil {
		t.Fatalf("Failed to read package.json: %v", err)
	}

	var packageJSON map[string]interface{}
	if err := json.Unmarshal(content, &packageJSON); err != nil {
		t.Fatalf("Failed to parse package.json: %v", err)
	}

	// Check package.json contents
	if name, ok := packageJSON["name"].(string); !ok || name != "go2type-test" {
		t.Errorf("Expected package name to be 'go2type-test', got '%v'", packageJSON["name"])
	}

	// Check if node_modules directory was created
	nodeModulesPath := filepath.Join(tmpdir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); os.IsNotExist(err) {
		t.Errorf("node_modules directory was not created")
	}

	// Check if TypeScript was installed
	tscPath := filepath.Join(nodeModulesPath, ".bin", "tsc")
	if _, err := os.Stat(tscPath); os.IsNotExist(err) {
		t.Errorf("TypeScript compiler (tsc) was not installed")
	}

	// Try running tsc
	cmd := exec.Command(tscPath, "--version")
	cmd.Dir = tmpdir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Failed to run tsc: %v\nOutput: %s", err, output)
	} else {
		t.Logf("TypeScript version: %s", strings.TrimSpace(string(output)))
	}

	// Check if @types/react was installed
	reactTypesPath := filepath.Join(nodeModulesPath, "@types", "react")
	if _, err := os.Stat(reactTypesPath); os.IsNotExist(err) {
		t.Errorf("@types/react was not installed")
	}

	// Check if @tanstack/react-query was installed
	reactQueryPath := filepath.Join(nodeModulesPath, "@tanstack", "react-query")
	if _, err := os.Stat(reactQueryPath); os.IsNotExist(err) {
		t.Errorf("@tanstack/react-query was not installed")
	}
}

func TestGenerateFile(t *testing.T) {
	// Skip if npm is not installed
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not found, skipping test")
	}

	// Create a temporary directory for output
	tmpdir := createTempFolder(t.Name())

	// Defer cleanup, but only if the test passes
	defer func() {
		if !t.Failed() {
			_ = os.RemoveAll(tmpdir)
		} else {
			t.Logf("Test failed. Temporary directory retained at: %s", tmpdir)
		}
	}()

	// Setup npm project in the temporary directory
	if err := setupNpmProject(tmpdir); err != nil {
		t.Fatalf("Failed to setup npm project: %v", err)
	}

	types := []TypeInfo{
		{
			Name: "User",
			Fields: []FieldInfo{
				{Name: "id", Type: "number", JSONName: "id"},
				{Name: "name", Type: "string", JSONName: "name"},
				{Name: "email", Type: "string", JSONName: "email"},
				{Name: "created_at", Type: "Date", JSONName: "created_at"},
				{Name: "updated_at", Type: "Date", JSONName: "updated_at"},
			},
		},
		{
			Name: "GetUserInput",
			Fields: []FieldInfo{
				{Name: "id", Type: "number", JSONName: "id"},
			},
		},
		{
			Name: "CreateUserInput",
			Fields: []FieldInfo{
				{Name: "name", Type: "string", JSONName: "name"},
				{Name: "email", Type: "string", JSONName: "email"},
			},
		},
	}

	handlers := []HandlerInfo{
		{
			Name:       "GetUser",
			Method:     "GET",
			Path:       "/users/:id",
			InputType:  "GetUserInput",
			OutputType: "User",
			URLParams:  []string{"id"},
			Headers: []HeaderInfo{
				{
					HeaderKey:  "X-Auth-Token",
					SafeName:   "x_auth_token",
					Source:     "localStorage",
					StorageKey: "auth_token",
				},
				{
					HeaderKey:  "X-Custom-Header",
					SafeName:   "x_custom_header",
					Source:     "localStorage",
					StorageKey: "X-Custom-Header", // Default to header key
				},
			},
		},
		{
			Name:       "CreateUser",
			Method:     "POST",
			Path:       "/users",
			InputType:  "CreateUserInput",
			OutputType: "User",
			Headers: []HeaderInfo{
				{
					HeaderKey:  "X-Session-ID",
					SafeName:   "x_session_id",
					Source:     "sessionStorage",
					StorageKey: "session_id",
				},
				{
					HeaderKey:  "Content-Type",
					SafeName:   "content_type",
					Source:     "input",
					StorageKey: "",
				},
			},
		},
	}

	testCases := []struct {
		name             string
		useHooks         bool
		useReactQuery    bool
		useDateObject    bool
		authTokenStorage string
		expectedContent  []string
		outputFile       string
	}{
		{
			name:             "No hooks, no date object, localStorage",
			outputFile:       filepath.Join(tmpdir, "no_hooks__no_date_obj__localStorage.ts"),
			useHooks:         false,
			useReactQuery:    false,
			useDateObject:    false,
			authTokenStorage: "localStorage",
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const GetUserQuery",
				"export const CreateUserQuery",
				"localStorage.getItem",
				"const token = localStorage.getItem",
			},
		},
		{
			name:             "React Query hooks, with date object, sessionStorage",
			outputFile:       filepath.Join(tmpdir, "react_query_hooks__date_obj__sessionStorage.ts"),
			useHooks:         true,
			useReactQuery:    true,
			useDateObject:    true,
			authTokenStorage: "sessionStorage",
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"created_at: Date;",
				"updated_at: Date;",
				"const parseDate = (dateString: string): Date => new Date(dateString);",
				"export const useGetUser",
				"export const useCreateUser",
				"useQuery<User, APIError, User",
				"useMutation<User, APIError",
				"sessionStorage.getItem",
				"const token = sessionStorage.getItem",
			},
		},
		{
			name:             "No hooks, no date object",
			outputFile:       filepath.Join(tmpdir, "no_hooks__no_date_obj.ts"),
			useHooks:         false,
			useReactQuery:    false,
			useDateObject:    false,
			authTokenStorage: "localStorage",
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const GetUserQuery",
				"export const CreateUserQuery",
				"const token = localStorage.getItem",
			},
		},
		{
			name:             "React hooks, no date object",
			outputFile:       filepath.Join(tmpdir, "react_hooks__no_date_obj.ts"),
			useHooks:         true,
			useReactQuery:    false,
			useDateObject:    false,
			authTokenStorage: "localStorage",
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const useGetUser",
				"export const useCreateUser",
				"useState<User | null>",
				"const token = localStorage.getItem",
			},
		},
		{
			name:             "React hooks, with date object",
			outputFile:       filepath.Join(tmpdir, "react_hooks__with_date_obj.ts"),
			useHooks:         true,
			useReactQuery:    false,
			useDateObject:    true,
			authTokenStorage: "localStorage",
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const useGetUser",
				"export const useCreateUser",
				"useState<User | null>",
				"const token = localStorage.getItem",
			},
		},
		{
			name:             "React Query hooks, no date object",
			outputFile:       filepath.Join(tmpdir, "react_query_hooks__no_date_obj.ts"),
			useHooks:         true,
			useReactQuery:    true,
			useDateObject:    false,
			authTokenStorage: "localStorage",
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const useGetUser",
				"export const useCreateUser",
				"useQuery<User, APIError, User",
				"useMutation<User, APIError",
				"const token = localStorage.getItem",
			},
		},
		{
			name:             "React Query hooks, with date object",
			outputFile:       filepath.Join(tmpdir, "react_query_hooks__date_obj.ts"),
			useHooks:         true,
			useReactQuery:    true,
			useDateObject:    true,
			authTokenStorage: "localStorage",
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"created_at: Date;",
				"updated_at: Date;",
				"const parseDate = (dateString: string): Date => new Date(dateString);",
				"export const useGetUser",
				"export const useCreateUser",
				"useQuery<User, APIError, User",
				"useMutation<User, APIError",
				"const token = localStorage.getItem",
			},
		},
		{
			name:             "React Query hooks with storage keys and defaults",
			outputFile:       filepath.Join(tmpdir, "react_query_hooks_with_storage_keys_and_defaults.ts"),
			useHooks:         true,
			useReactQuery:    true,
			useDateObject:    true,
			authTokenStorage: "localStorage",
			expectedContent: []string{
				"const x_auth_tokenValue = localStorage.getItem('auth_token');",
				"const x_custom_headerValue = localStorage.getItem('X-Custom-Header');",
				"const x_session_idValue = sessionStorage.getItem('session_id');",
				"headers['Content-Type'] = content_type;",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := GenerateFileOptions{
				Types:            types,
				Handlers:         handlers,
				OutputFile:       tc.outputFile,
				AuthToken:        "test_token",
				AuthTokenStorage: tc.authTokenStorage,
				UseHooks:         tc.useHooks,
				UseReactQuery:    tc.useReactQuery,
				ShouldFormat:     false,
				UseDateObject:    tc.useDateObject,
			}

			if err := generateFile(opts); err != nil {
				t.Fatalf("Failed to generate file: %v", err)
			}

			// Read the generated file
			content, err := os.ReadFile(tc.outputFile)
			if err != nil {
				t.Fatalf("Failed to read generated file: %v", err)
			}

			// Check for expected content
			for _, str := range tc.expectedContent {
				if !strings.Contains(string(content), str) {
					t.Errorf("Expected string not found in generated file: %s", str)
				}
			}

			// Check file size
			fileInfo, err := os.Stat(tc.outputFile)
			if err != nil {
				t.Fatalf("Failed to get file info: %v", err)
			}
			t.Logf("Generated file size: %d bytes", fileInfo.Size())

			if fileInfo.Size() == 0 {
				t.Errorf("Generated file is empty")
			}

			// Run TypeScript compilation
			if err := runTypeScriptCompilation(t, tmpdir, tc.outputFile); err != nil {
				t.Errorf("TypeScript compilation failed: %v", err)
			}
		})
	}
}
func runTypeScriptCompilation(t *testing.T, dir string, filePath string) error {
	tsconfigPath := filepath.Join(dir, "tsconfig.json")
	// Get the relative path of the file from the directory
	relFilePath, err := filepath.Rel(dir, filePath)
	if err != nil {
		return fmt.Errorf("failed to get relative path: %v", err)
	}

	tsconfig := fmt.Sprintf(`{
  "compilerOptions": {
    "target": "es2020",
    "module": "esnext",
    "lib": ["es2020", "dom"],
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "forceConsistentCasingInFileNames": true,
    "jsx": "react",
    "moduleResolution": "node",
    "allowSyntheticDefaultImports": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true
  },
  "include": ["%s"],
  "exclude": ["node_modules"]
}`, relFilePath)

	if err := os.WriteFile(tsconfigPath, []byte(tsconfig), 0644); err != nil {
		return fmt.Errorf("failed to create tsconfig.json: %v", err)
	}

	cmd := exec.Command("npx", "tsc", "--noEmit")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("TypeScript compilation failed: %v\nOutput: %s", err, output)
	}

	t.Logf("TypeScript compilation successful for %s", filePath)
	return nil
}

func setupNpmProject(dir string) error {
	// Create package.json
	packageJSON := map[string]interface{}{
		"name":    "go2type-test",
		"version": "1.0.0",
		"private": true,
	}
	packageJSONBytes, err := json.Marshal(packageJSON)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), packageJSONBytes, 0644); err != nil {
		return err
	}

	// Run npm install
	cmd := exec.Command("npm", "install", "typescript", "@types/react", "@tanstack/react-query")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("npm install failed: %v\nOutput: %s", err, output)
	}

	return nil
}

func createTempFolder(name string) string {
	// Get the system's temporary directory
	tempDir := os.TempDir()

	// Combine the temp directory path with the provided folder name
	fullPath := filepath.Join(tempDir, Timestamp, name)

	// Create the directory
	err := os.MkdirAll(fullPath, 0755)
	if err != nil {
		log.Fatalf("Failed to create temporary directory: %v", err)
	}

	return fullPath
}
