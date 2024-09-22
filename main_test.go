package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	content := `
auth_token: test_token
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
	tmpfile, err := ioutil.TempFile("", "go2type-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

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
}

func TestParsePackage(t *testing.T) {
	// Create a temporary directory with Go files
	tmpdir, err := ioutil.TempDir("", "go2type-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Create api.go
	apiContent := `
package api

import "time"

type User struct {
	ID        int       ` + "`json:\"id\"`" + `
	Name      string    ` + "`json:\"name\"`" + `
	CreatedAt time.Time ` + "`json:\"created_at\"`" + `
}

// @Method GET
// @Path /users/:id
// @Input GetUserInput
// @Output User
// @Header X-Authorization
func GetUserHandler() {}

// @Method POST
// @Path /users
// @Input CreateUserInput
// @Output ComplexUser
// @Header input:Content-Type
// @Header localStorage:X-Custom-Header
// @Header sessionStorage:X-Session-Header
func CreateUserHandler() {}
`

	if err := ioutil.WriteFile(filepath.Join(tmpdir, "api.go"), []byte(apiContent), 0644); err != nil {
		t.Fatalf("Failed to write api.go file: %v", err)
	}

	// Create models.go
	modelsContent := `
package models

import "github.com/google/uuid"

type CustomType string

type ComplexUser struct {
	ID   uuid.UUID  ` + "`json:\"id\"`" + `
	Info UserInfo   ` + "`json:\"info\"`" + `
	Tags []string   ` + "`json:\"tags\"`" + `
}

type UserInfo struct {
	Email    string     ` + "`json:\"email\"`" + `
	Settings CustomType ` + "`json:\"settings\"`" + `
}
`

	if err := ioutil.WriteFile(filepath.Join(tmpdir, "models.go"), []byte(modelsContent), 0644); err != nil {
		t.Fatalf("Failed to write models.go file: %v", err)
	}

	// Test parsing the package
	customTypeMappings := map[string]string{
		"CustomType": "string",
		"uuid.UUID":  "string",
	}
	types, handlers, err := parsePackage(tmpdir, customTypeMappings, true)
	if err != nil {
		t.Fatalf("Failed to parse package: %v", err)
	}

	// Verify the parsed types
	expectedTypes := []TypeInfo{
		{
			Name: "ComplexUser",
			Fields: []FieldInfo{
				{Name: "id", Type: "string", JSONName: "id"},
				{Name: "info", Type: "UserInfo", JSONName: "info"},
				{Name: "tags", Type: "Array<string>", JSONName: "tags"},
			},
		},
		{
			Name: "UserInfo",
			Fields: []FieldInfo{
				{Name: "email", Type: "string", JSONName: "email"},
				{Name: "settings", Type: "string", JSONName: "settings"},
			},
		},
		{
			Name: "User",
			Fields: []FieldInfo{
				{Name: "id", Type: "number", JSONName: "id"},
				{Name: "name", Type: "string", JSONName: "name"},
				{Name: "created_at", Type: "Date", JSONName: "created_at"},
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
			Headers:    []HeaderInfo{{OriginalName: "X-Authorization", SafeName: "x_authorization", Source: "input"}},
		},
		{
			Name:       "CreateUser",
			Method:     "POST",
			Path:       "/users",
			InputType:  "CreateUserInput",
			OutputType: "ComplexUser",
			Headers: []HeaderInfo{
				{OriginalName: "Content-Type", SafeName: "content_type", Source: "input"},
				{OriginalName: "X-Custom-Header", SafeName: "x_custom_header", Source: "localStorage"},
				{OriginalName: "X-Session-Header", SafeName: "x_session_header", Source: "sessionStorage"},
			},
		},
	}

	if !reflect.DeepEqual(handlers, expectedHandlers) {
		t.Errorf("Parsed handlers do not match expected.\nGot: %+v\nWant: %+v", handlers, expectedHandlers)
	}
}

// Helper function to compare two slices of TypeInfo, ignoring order
func compareTypes(a, b []TypeInfo) bool {
	if len(a) != len(b) {
		return false
	}

	typeMap := make(map[string]TypeInfo)
	for _, t := range b {
		typeMap[t.Name] = t
	}

	for _, t := range a {
		if et, ok := typeMap[t.Name]; !ok || !reflect.DeepEqual(t, et) {
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
	tmpdir, err := ioutil.TempDir("", "go2type-npm")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	// Defer cleanup, but only if the test passes
	defer func() {
		if !t.Failed() {
			t.Logf("Cleaning up temporary directory: %s", tmpdir)
			os.RemoveAll(tmpdir)
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
	content, err := ioutil.ReadFile(packageJSONPath)
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
	tmpdir, err := ioutil.TempDir("", "go2type-output")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	// Defer cleanup, but only if the test passes
	defer func() {
		if !t.Failed() {
			os.RemoveAll(tmpdir)
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
			Headers:    []HeaderInfo{{OriginalName: "Authorization", SafeName: "authorization", Source: "input"}},
		},
		{
			Name:       "CreateUser",
			Method:     "POST",
			Path:       "/users",
			InputType:  "CreateUserInput",
			OutputType: "User",
			Headers: []HeaderInfo{
				{OriginalName: "Content-Type", SafeName: "content_type", Source: "input"},
				{OriginalName: "X-Custom-Header", SafeName: "x_custom_header", Source: "localStorage"},
			},
		},
	}

	testCases := []struct {
		name            string
		useHooks        bool
		useReactQuery   bool
		useDateObject   bool
		expectedContent []string
		outputFile      string
	}{
		{
			name:          "No hooks, no date object",
			outputFile:    filepath.Join(tmpdir, "no_hooks__no_date_obj.ts"),
			useHooks:      false,
			useReactQuery: false,
			useDateObject: false,
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const GetUserQuery",
				"export const CreateUserQuery",
			},
		},
		{
			name:          "React hooks, no date object",
			outputFile:    filepath.Join(tmpdir, "react_hooks__no_date_obj.ts"),
			useHooks:      true,
			useReactQuery: false,
			useDateObject: false,
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const useGetUser",
				"export const useCreateUser",
				"useState<User | null>",
			},
		},
		{
			name:          "React Query hooks, no date object",
			outputFile:    filepath.Join(tmpdir, "react_query_hooks__no_date_obj.ts"),
			useHooks:      true,
			useReactQuery: true,
			useDateObject: false,
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"export const useGetUser",
				"export const useCreateUser",
				"useQuery<User, APIError>",
				"useMutation<User, APIError",
			},
		},
		{
			name:          "React Query hooks, with date object",
			outputFile:    filepath.Join(tmpdir, "react_query_hooks__date_obj.ts"),
			useHooks:      true,
			useReactQuery: true,
			useDateObject: true,
			expectedContent: []string{
				"export type User",
				"export type GetUserInput",
				"export type CreateUserInput",
				"created_at: Date;",
				"updated_at: Date;",
				"const parseDate = (dateString: string): Date => new Date(dateString);",
				"export const useGetUser",
				"export const useCreateUser",
				"useQuery<User, APIError>",
				"useMutation<User, APIError",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			opts := GenerateFileOptions{
				Types:         types,
				Handlers:      handlers,
				OutputFile:    tc.outputFile,
				AuthToken:     "test_token",
				UseHooks:      tc.useHooks,
				UseReactQuery: tc.useReactQuery,
				ShouldFormat:  false,
				UseDateObject: tc.useDateObject,
			}

			if err := generateFile(opts); err != nil {
				t.Fatalf("Failed to generate file: %v", err)
			}

			// Read the generated file
			content, err := ioutil.ReadFile(tc.outputFile)
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

	if err := ioutil.WriteFile(tsconfigPath, []byte(tsconfig), 0644); err != nil {
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
	if err := ioutil.WriteFile(filepath.Join(dir, "package.json"), packageJSONBytes, 0644); err != nil {
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
