package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/text/language"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"text/template"
	"time"
	"unicode"

	cases "golang.org/x/text/cases"
	packages "golang.org/x/tools/go/packages"
	yaml "gopkg.in/yaml.v3"
)

// Config represents the configuration yaml file
type Config struct {
	AuthToken        string          `yaml:"auth_token"`
	AuthTokenStorage string          `yaml:"auth_token_storage"`
	PrettierPath     string          `yaml:"prettier_path"`
	Hooks            string          `yaml:"hooks"`
	UseDateObject    bool            `yaml:"use_date_object"`
	Packages         []PackageConfig `yaml:"packages"`
}

// PackageConfig represents the configuration for a Go package
type PackageConfig struct {
	Path         string            `yaml:"path"`
	OutputPath   string            `yaml:"output_path"`
	TypeMappings map[string]string `yaml:"type_mappings"`
}

type HeaderInfo struct {
	HeaderKey  string
	SafeName   string
	Source     string
	StorageKey string
}

type HandlerInfo struct {
	Name       string
	Method     string
	Path       string
	InputType  string
	OutputType string
	URLParams  []string
	Headers    []HeaderInfo
}

type TypeInfo struct {
	Name     string
	FullName string
	Fields   []FieldInfo
}

type FieldInfo struct {
	PackageName string
	Name        string
	Type        string
	JSONName    string
	IsArray     bool
	IsOptional  bool
}

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]
	switch command {
	case "init":
		if err := initConfig(); err != nil {
			fmt.Printf("Error initializing config: %v\n", err)
			os.Exit(1)
		}
	case "generate":
		printVersion()
		if err := generate(true); err != nil {
			fmt.Printf("Error generating files: %v\n", err)
			os.Exit(1)
		}
	case "version":
		printVersion()
	case "help":
		printHelp()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printVersion()
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println("Usage: go2type <command>")
	fmt.Println("Available commands:")
	fmt.Println("  init      Initialize a new configuration file")
	fmt.Println("  generate  Generate TypeScript files based on the configuration")
	fmt.Println("  version   Print the version of go2type")
	fmt.Println("  help      Print this help message")
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.AuthTokenStorage != "localStorage" && config.AuthTokenStorage != "sessionStorage" {
		config.AuthTokenStorage = "localStorage"
	}

	return &config, nil
}

func initConfig() error {
	// Check if config file already exists
	if _, err := os.Stat("go2type.yaml"); err == nil {
		return fmt.Errorf("configuration file 'go2type.yaml' already exists. Please remove it or use a different name if you want to create a new configuration")
	}

	config := Config{
		AuthToken:        "session_token",
		AuthTokenStorage: "localStorage",
		Hooks:            "false",
	}

	// Find Go handlers with @Method comments
	cfgPackages, err := findGoHandlers(".")
	if err != nil {
		return fmt.Errorf("error finding Go handlers: %v", err)
	}

	// Check for react-query or react
	nodeModulesPath, frontendPath, _ := findNodeModules()
	if nodeModulesPath != "" {
		reactQueryPath := filepath.Join(nodeModulesPath, "@tanstack", "react-query")
		reactPath := filepath.Join(nodeModulesPath, "react")
		if _, err := os.Stat(reactQueryPath); err == nil {
			config.Hooks = "react-query"
		} else if _, err := os.Stat(reactPath); err == nil {
			config.Hooks = "true"
		}
	}

	// Find Prettier
	prettierPath, err := exec.LookPath("prettier")
	if err != nil && nodeModulesPath != "" {
		// Find Prettier in node_modules
		prettierBinPath := filepath.Join(nodeModulesPath, ".bin", "prettier")
		if _, err := os.Stat(prettierBinPath); err == nil {
			config.PrettierPath = prettierBinPath
		} else {
			// If not found in node_modules, search in system PATH
			prettierPath, err := exec.LookPath("prettier")
			if err == nil {
				config.PrettierPath = prettierPath
			} else {
				fmt.Println("Prettier not found in node_modules or system PATH")
			}
		}
	} else {
		config.PrettierPath = prettierPath
	}

	// Use a map to prevent duplicate package entries
	uniquePackages := make(map[string]PackageConfig)

	for _, pkg := range cfgPackages {
		pkg.TypeMappings = map[string]string{
			"null.String":   "null | string",
			"null.Bool":     "null | boolean",
			"uuid.UUID":     "string /* uuid */",
			"uuid.NullUUID": "null | string /* uuid */",
		}
		if existingPkg, ok := uniquePackages[pkg.Path]; ok {
			// If the package already exists, just update the output path if it's empty
			if existingPkg.OutputPath == "" {
				outputPath := filepath.Join(frontendPath, "src", "api.generated.ts")
				existingPkg.OutputPath = outputPath
				uniquePackages[pkg.Path] = existingPkg
			}
		} else {
			// If it's a new package, set the output path
			pkg.OutputPath = ""
			uniquePackages[pkg.Path] = pkg
		}
	}

	// Convert the map back to a slice
	for _, pkg := range uniquePackages {
		config.Packages = append(config.Packages, pkg)
	}

	// Write config to file
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("error marshaling config: %v", err)
	}

	if err := os.WriteFile("go2type.yaml", data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %v", err)
	}

	fmt.Println("Configuration file 'go2type.yaml' has been created.")
	return nil
}

func findNodeModules() (string, string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", "", err
	}

	var nodeModulesPath, parentPath string
	err = filepath.Walk(currentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && info.Name() == "node_modules" {
			nodeModulesPath, err = filepath.Rel(currentDir, path)
			if err != nil {
				return err
			}
			parentPath, err = filepath.Rel(currentDir, filepath.Dir(path))
			if err != nil {
				return err
			}
			return filepath.SkipDir
		}
		return nil
	})

	if err != nil {
		return "", "", err
	}

	if nodeModulesPath == "" {
		return "", "", os.ErrNotExist
	}

	return nodeModulesPath, parentPath, nil
}

func findGoHandlers(root string) ([]PackageConfig, error) {
	var goPackages []PackageConfig

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && (info.Name() == "vendor" || info.Name() == "node_modules") {
			return filepath.SkipDir
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return nil // Skip files that can't be parsed
			}

			for _, decl := range f.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok {
					if fn.Doc != nil {
						for _, comment := range fn.Doc.List {
							if strings.Contains(comment.Text, "@Method") {
								goPackages = append(goPackages, PackageConfig{
									Path:         filepath.Dir(path),
									TypeMappings: make(map[string]string),
								})
								return nil // Found a handler, move to next directory
							}
						}
					}
				}
			}
		}

		return nil
	})

	return goPackages, err
}

func generate(shouldFormat bool) error {
	config, err := loadConfig("go2type.yaml")
	if err != nil {
		return fmt.Errorf("error loading config: %v", err)
	}

	for _, pkg := range config.Packages {
		absPath, err := filepath.Abs(pkg.Path)
		if err != nil {
			fmt.Printf("Error resolving absolute path for %s: %v\n", pkg.Path, err)
			continue
		}

		pkgTypes, handlers, err := parsePackage(absPath, pkg.TypeMappings, config.UseDateObject)
		if err != nil {
			fmt.Printf("Error parsing package %s: %v\n", pkg.Path, err)
			continue
		}

		useHooks := config.Hooks == "true" || config.Hooks == "react-query"
		useReactQuery := config.Hooks == "react-query"

		authTokenStorage := "localStorage"
		if config.AuthTokenStorage == "sessionStorage" {
			authTokenStorage = config.AuthTokenStorage
		} else if config.AuthTokenStorage != "localStorage" && config.AuthTokenStorage != "" {
			fmt.Printf("Warning: Unknown auth token storage type %s. Using localStorage instead.\n", config.AuthTokenStorage)
		}

		opts := GenerateFileOptions{
			Types:            pkgTypes,
			Handlers:         handlers,
			OutputFile:       pkg.OutputPath,
			AuthToken:        config.AuthToken,
			AuthTokenStorage: authTokenStorage,
			PrettierPath:     config.PrettierPath,
			UseHooks:         useHooks,
			UseReactQuery:    useReactQuery,
			ShouldFormat:     shouldFormat,
			UseDateObject:    config.UseDateObject,
		}

		if err := generateFile(opts); err != nil {
			fmt.Printf("Error generating file for package %s: %v\n", pkg.Path, err)
			continue
		}

		fmt.Printf("Generated file for package %s at %s\n", pkg.Path, pkg.OutputPath)
	}

	return nil
}

type TemplatePiece struct {
	Name   string
	Tmpl   string
	Render bool
}

// GenerateFileOptions contains all the options for generating a file
type GenerateFileOptions struct {
	Types            []TypeInfo
	Handlers         []HandlerInfo
	OutputFile       string
	AuthToken        string
	AuthTokenStorage string
	PrettierPath     string
	UseHooks         bool
	UseReactQuery    bool
	ShouldFormat     bool
	UseDateObject    bool
}

func generateFile(opts GenerateFileOptions) error {
	dir := filepath.Dir(opts.OutputFile)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	funcMap := template.FuncMap{
		"last": func(x interface{}) interface{} {
			v := reflect.ValueOf(x)
			return v.Index(v.Len() - 1).Interface()
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"firstWord": func(s string) string {
			return strings.Split(s, " ")[0]
		},
		"inputHeaders": func(headers []HeaderInfo) []HeaderInfo {
			var result []HeaderInfo
			for _, h := range headers {
				if h.Source == "input" {
					result = append(result, h)
				}
			}
			return result
		},
	}

	file, err := os.Create(opts.OutputFile)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer func() {
		err := file.Close()
		if err != nil {
			log.Printf("Error closing file: %v", err)
		}
	}()

	data := TemplateData{
		Version:          Version,
		Timestamp:        time.Now().Format(time.RFC3339),
		Types:            opts.Types,
		Handlers:         opts.Handlers,
		AuthToken:        opts.AuthToken,
		AuthTokenStorage: opts.AuthTokenStorage,
		UseHooks:         opts.UseHooks,
		UseReactQuery:    opts.UseReactQuery,
		UseDateObject:    opts.UseDateObject,
	}

	// Create a new template and add the helper functions
	tmpl := template.New("typescript").Funcs(funcMap)

	// Define the order of template pieces
	templatePieces := []TemplatePiece{
		{Name: "headerTemplate", Tmpl: headerTemplate, Render: true},
		{Name: "typesTemplate", Tmpl: typesTemplate, Render: true},
		{Name: "queryFunctionTemplate", Tmpl: queryFunctionTemplate, Render: true},
		{Name: "reactQueryHookTemplate", Tmpl: reactQueryHookTemplate, Render: opts.UseReactQuery},
		{Name: "reactHookTemplate", Tmpl: reactHookTemplate, Render: opts.UseHooks && !opts.UseReactQuery},
		{Name: "queryDictionaryTemplate", Tmpl: queryDictionaryTemplate, Render: true},
	}

	// Parse and execute each template piece
	for _, piece := range templatePieces {
		if !piece.Render {
			continue
		}
		t, err := tmpl.Parse(piece.Tmpl)
		if err != nil {
			return fmt.Errorf("error parsing template piece %s: %v", piece.Name, err)
		}

		if err := t.Execute(file, data); err != nil {
			log.Printf("Error executing template: %s: %v", piece.Name, err)
			return fmt.Errorf("error executing template piece: %s: %v", piece.Name, err)
		}
	}

	if opts.ShouldFormat {
		// Format the generated code
		if err := formatCode(opts.OutputFile, opts.PrettierPath); err != nil {
			fmt.Printf("Warning: Failed to format %s: %v\n", opts.OutputFile, err)
		}
	}

	return nil
}

type TypeRegistry struct {
	Types map[string]TypeInfo
}

func (tr *TypeRegistry) AddType(t TypeInfo) {
	tr.Types[t.Name] = t
}

func (tr *TypeRegistry) GetType(name string) (TypeInfo, bool) {
	t, ok := tr.Types[name]
	return t, ok
}

func parsePackage(packagePath string, customTypeMappings map[string]string, useDateObject bool) ([]TypeInfo, []HandlerInfo, error) {
	// Merge default and custom type mappings
	typeMappings := make(map[string]string)
	for k, v := range defaultTypeMappings {
		typeMappings[k] = v
	}
	for k, v := range customTypeMappings {
		typeMappings[k] = v
	}
	if useDateObject {
		typeMappings["time.Time"] = "Date"
		typeMappings["pgtype.Timestamptz"] = "Date"
	} else {
		typeMappings["time.Time"] = "string /* date-time */"
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, packagePath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("error parsing directory: %v", err)
	}

	registry := &TypeRegistry{
		Types: make(map[string]TypeInfo),
	}
	var handlers []HandlerInfo
	importMap := make(map[string]string)

	// Get the module name and path
	moduleName, modulePath, err := getModuleInfo(packagePath)
	if err != nil {
		return nil, nil, fmt.Errorf("error getting module info: %v", err)
	}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			// Parse imports
			for _, imp := range file.Imports {
				if imp.Name != nil {
					importMap[imp.Name.Name] = strings.Trim(imp.Path.Value, "\"")
				} else {
					parts := strings.Split(strings.Trim(imp.Path.Value, "\""), "/")
					importMap[parts[len(parts)-1]] = strings.Trim(imp.Path.Value, "\"")
				}
			}

			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.TypeSpec:
					if structType, ok := node.Type.(*ast.StructType); ok {
						typeInfo := parseType(node.Name.Name, structType, typeMappings)
						registry.AddType(typeInfo)
					}
				case *ast.FuncDecl:
					if node.Doc != nil {
						if handler := parseHandlerComments(node); handler != nil {
							handlers = append(handlers, *handler)
						}
					}
				}
				return true
			})
		}
	}

	// Resolve nested types and external package types
	for _, t := range registry.Types {
		resolveNestedAndExternalTypes(&t, registry, packagePath, modulePath, typeMappings, importMap, moduleName)
	}

	// Convert registry to slice
	var allTypes []TypeInfo
	for _, t := range registry.Types {
		allTypes = append(allTypes, t)
	}

	// Filter types to include only those used in handlers
	usedTypes := filterUsedTypes(allTypes, handlers)

	return usedTypes, handlers, nil
}

func resolveNestedAndExternalTypes(t *TypeInfo, registry *TypeRegistry, currentPackagePath, modulePath string, typeMappings map[string]string, importMap map[string]string, moduleName string) {
	for i, field := range t.Fields {
		//nestedType, ok := registry.GetType(strings.Split(field.Type, " ")[0])

		if strings.Contains(field.PackageName, ".") {
			parts := strings.Split(field.PackageName, ".")
			packageName, typeName := parts[0], parts[1]

			fullPackagePath, ok := importMap[packageName]
			if !ok {
				fmt.Printf("Warning: Could not find import for package %s\n", packageName)
				continue
			}

			isExternalPackage := !strings.HasPrefix(fullPackagePath, moduleName)

			var resolvedType TypeInfo
			var err error

			if isExternalPackage {
				// For external packages, use parseExternalType
				resolvedType, err = parseExternalType(currentPackagePath, fullPackagePath, typeName, typeMappings, moduleName, field.PackageName)
				if err != nil {
					fmt.Printf("Warning: Failed to resolve external type %s: %v\n", field.PackageName, err)
					continue
				}

				// Use the mapped type directly for external types
				t.Fields[i].Type = resolvedType.Fields[0].Type
			} else {
				// For internal packages, parse the type structure
				resolvedType, err = parseInternalType(currentPackagePath, modulePath, fullPackagePath, typeName, typeMappings, moduleName)
				if err != nil {
					fmt.Printf("Warning: Failed to resolve internal type %s: %v\n", field.PackageName, err)
					continue
				}

				// Rename the type if there's a clash
				newTypeName := fmt.Sprintf("%s%s", cases.Title(language.Und, cases.NoLower).String(packageName), typeName)
				if t.Fields[i].IsArray {
					newTypeName = fmt.Sprintf("Array<%s>", newTypeName)
				}

				t.Fields[i].Type = newTypeName

				tName := strings.Split(strings.TrimSuffix(strings.TrimPrefix(newTypeName, "Array<"), ">"), " ")[0]

				// Add the internal type to the registry
				registry.AddType(TypeInfo{Name: tName, FullName: packageName, Fields: resolvedType.Fields})
			}
		} else if nestedType, ok := registry.GetType(strings.Split(field.PackageName, " ")[0]); ok {
			// This is a nested type, resolve it recursively
			resolveNestedAndExternalTypes(&nestedType, registry, currentPackagePath, modulePath, typeMappings, importMap, moduleName)
			registry.AddType(nestedType)
		}
	}
}

func parseExternalType(currentPackagePath, importPath, typeName string, typeMappings map[string]string, moduleName string, fullTypeName string) (TypeInfo, error) {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedSyntax,
		Dir:  filepath.Dir(currentPackagePath),
	}

	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return TypeInfo{}, fmt.Errorf("failed to load package %s: %v", importPath, err)
	}

	if len(pkgs) == 0 {
		return TypeInfo{}, fmt.Errorf("no packages found for %s", importPath)
	}

	pkg := pkgs[0]
	if pkg.Types == nil {
		return TypeInfo{}, fmt.Errorf("types information not available for package %s", importPath)
	}

	typeName = strings.TrimPrefix(typeName, "*")
	if strings.Contains(typeName, " ") {
		typeName = strings.Split(typeName, " ")[0]
	}

	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		return TypeInfo{}, fmt.Errorf("type %s not found in package %s", typeName, importPath)
	}

	isExternalPackage := !strings.HasPrefix(pkg.PkgPath, moduleName)

	if isExternalPackage {
		// Check custom mappings first
		if mappedType, ok := typeMappings[fullTypeName]; ok {

			return TypeInfo{
				Name:     typeName,
				FullName: fullTypeName,
				Fields: []FieldInfo{
					{
						PackageName: fullTypeName,
						Name:        typeName,
						Type:        mappedType,
						JSONName:    typeName,
						IsOptional:  false,
					},
				},
			}, nil
		}

		// Then check default mappings
		if mappedType, ok := defaultTypeMappings[fullTypeName]; ok {

			return TypeInfo{
				Name:     typeName,
				FullName: fullTypeName,
				Fields: []FieldInfo{
					{
						PackageName: fullTypeName,
						Name:        typeName,
						Type:        mappedType,
						JSONName:    typeName,
						IsOptional:  false,
					},
				},
			}, nil
		}

		// If no mapping found for external type, error out
		return TypeInfo{}, fmt.Errorf("no type mapping for external type")
	} else {
		// For internal packages, parse the type structure
		return parseTypeObject(obj, typeMappings)
	}
}

func getModuleInfo(packagePath string) (string, string, error) {
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}} {{.Dir}}")
	cmd.Dir = packagePath
	output, err := cmd.Output()
	if err != nil {
		return "", "", err
	}
	parts := strings.Fields(strings.TrimSpace(string(output)))
	if len(parts) != 2 {
		return "", "", fmt.Errorf("unexpected output format from 'go list -m'")
	}
	return parts[0], parts[1], nil
}

func parseInternalType(currentPackagePath, modulePath, importPath, typeName string, typeMappings map[string]string, moduleName string) (TypeInfo, error) {
	pkgPath := filepath.Join(modulePath, strings.TrimPrefix(importPath, moduleName))

	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedSyntax,
		Dir:  modulePath,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return TypeInfo{}, fmt.Errorf("failed to load package %s: %v", pkgPath, err)
	}

	if len(pkgs) == 0 {
		return TypeInfo{}, fmt.Errorf("no packages found for %s", pkgPath)
	}

	pkg := pkgs[0]
	if pkg.Types == nil {
		return TypeInfo{}, fmt.Errorf("types information not available for package %s", pkgPath)
	}

	typeName = strings.TrimPrefix(typeName, "*")
	if strings.Contains(typeName, " ") {
		typeName = strings.Split(typeName, " ")[0]
	}
	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		return TypeInfo{}, fmt.Errorf("type %s not found in package %s", typeName, pkgPath)
	}

	return parseTypeObject(obj, typeMappings)
}

func parseTypeObject(obj types.Object, typeMappings map[string]string) (TypeInfo, error) {
	typeInfo := TypeInfo{Name: obj.Name(), FullName: obj.Name()}

	switch t := obj.Type().Underlying().(type) {
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			field := t.Field(i)
			fieldType, packageName, isOptional := parseFieldTypeFromTypes(field.Type(), typeMappings)
			jsonTag := reflect.StructTag(t.Tag(i)).Get("json")
			jsonName := strings.Split(jsonTag, ",")[0]
			if jsonName == "" {
				jsonName = field.Name()
			}

			typeInfo.Fields = append(typeInfo.Fields, FieldInfo{
				PackageName: packageName,
				Name:        jsonName,
				Type:        fieldType,
				JSONName:    jsonName,
				IsOptional:  isOptional,
			})
		}
	case *types.Basic, *types.Slice, *types.Map, *types.Interface:
		fieldType, packageName, isOptional := parseFieldTypeFromTypes(obj.Type(), typeMappings)

		typeInfo.Fields = append(typeInfo.Fields, FieldInfo{
			PackageName: packageName,
			Name:        obj.Name(),
			Type:        fieldType,
			JSONName:    obj.Name(),
			IsOptional:  isOptional,
		})
	default:
		return TypeInfo{}, fmt.Errorf("unsupported type kind for %s: %T", obj.Name(), t)
	}

	return typeInfo, nil
}

func filterUsedTypes(allTypes []TypeInfo, handlers []HandlerInfo) []TypeInfo {
	usedTypeSet := make(map[string]bool)
	var queue []string

	// Initialize the queue with types directly used in handlers
	for _, handler := range handlers {
		if handler.InputType != "" {
			queue = append(queue, handler.InputType)
		}
		if handler.OutputType != "" {
			queue = append(queue, handler.OutputType)
		}
	}

	// Process the queue
	for len(queue) > 0 {
		typeName := queue[0]
		queue = queue[1:]

		if !usedTypeSet[typeName] {
			usedTypeSet[typeName] = true

			// Find the type and add its nested types to the queue
			for _, t := range allTypes {
				tName := strings.Split(strings.TrimSuffix(strings.TrimPrefix(t.Name, "Array<"), ">"), " ")[0]

				if tName == typeName {
					for _, field := range t.Fields {
						fieldType := strings.Split(strings.TrimSuffix(strings.TrimPrefix(field.Type, "Array<"), ">"), " ")[0]
						if !usedTypeSet[fieldType] {
							queue = append(queue, fieldType)
						}
					}
					break
				}
			}
		}
	}

	// Filter types based on the used type set
	var usedTypes []TypeInfo
	for _, t := range allTypes {
		tName := strings.Split(strings.TrimSuffix(strings.TrimPrefix(t.Name, "Array<"), ">"), " ")[0]
		if usedTypeSet[tName] {
			usedTypes = append(usedTypes, t)
		}
	}

	return usedTypes
}
func parseType(name string, structType *ast.StructType, typeMappings map[string]string) TypeInfo {
	var fields []FieldInfo
	for _, field := range structType.Fields.List {
		if len(field.Names) > 0 {
			fieldName := field.Names[0].Name
			fieldType, trueType, isOptional, isArray := parseFieldType(field.Type, typeMappings)
			jsonName := getJSONTag(field.Tag)

			typescriptFieldName := fieldName
			if jsonName != "" {
				typescriptFieldName = jsonName
			}

			// Add "| null" only if the field is optional
			if isOptional {
				fieldType += " | null"
			}

			fields = append(fields, FieldInfo{
				PackageName: trueType,
				Name:        typescriptFieldName,
				Type:        fieldType,
				JSONName:    jsonName,
				IsOptional:  isOptional,
				IsArray:     isArray,
			})
		}
	}
	return TypeInfo{FullName: name, Name: name, Fields: fields}
}

func parseFieldType(expr ast.Expr, typeMappings map[string]string) (string, string, bool, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		if mappedType, ok := typeMappings[t.Name]; ok {
			return mappedType, t.Name, false, false
		}
		return t.Name, t.Name, false, false
	case *ast.SelectorExpr:
		fullType := fmt.Sprintf("%s.%s", t.X, t.Sel)
		if mappedType, ok := typeMappings[fullType]; ok {
			return mappedType, fullType, false, false
		}
		return fullType, fullType, false, false
	case *ast.StarExpr:
		innerType, in2type, _, _ := parseFieldType(t.X, typeMappings)
		return innerType, in2type, true, false
	case *ast.ArrayType:
		elemType, elemType2, _, _ := parseFieldType(t.Elt, typeMappings)
		return fmt.Sprintf("Array<%s>", elemType), elemType2, false, true
	case *ast.MapType:
		keyType, _, _, _ := parseFieldType(t.Key, typeMappings)
		valueType, _, _, _ := parseFieldType(t.Value, typeMappings)
		return fmt.Sprintf("{ [key: %s]: %s }", keyType, valueType), "map", false, false
	default:
		return "unknown", "unknown", false, false
	}
}

// Update parseFieldTypeFromTypes to handle more complex types
func parseFieldTypeFromTypes(t types.Type, typeMappings map[string]string) (string, string, bool) {

	switch t := t.(type) {
	case *types.Basic:
		if mappedType, ok := defaultTypeMappings[t.Name()]; ok {
			return mappedType, t.Name(), false
		}
		return t.Name(), t.Name(), false
	case *types.Pointer:
		elemType, actualElemType, _ := parseFieldTypeFromTypes(t.Elem(), typeMappings)
		return elemType + " | null", actualElemType, true
	case *types.Slice:
		elemType, actualElemType, _ := parseFieldTypeFromTypes(t.Elem(), typeMappings)
		return fmt.Sprintf("Array<%s>", elemType), actualElemType, true
	case *types.Map:
		keyType, _, _ := parseFieldTypeFromTypes(t.Key(), typeMappings)
		valueType, _, _ := parseFieldTypeFromTypes(t.Elem(), typeMappings)
		return fmt.Sprintf("{ [key: %s]: %s }", keyType, valueType), "map", false
	case *types.Interface:
		return "any", "any", false
	default:
		typeName := ExtractAfterLastSlash(t.String())
		if mappedType, ok := typeMappings[typeName]; ok {
			return mappedType, typeName, false
		}
		return "unknown", "unknown", false
	}
}

func getJSONTag(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}
	fullTag := reflect.StructTag(strings.Trim(tag.Value, "`"))
	jsonTag := fullTag.Get("json")
	if jsonTag == "" {
		return ""
	}
	parts := strings.Split(jsonTag, ",")
	return parts[0] // Return only the name part of the JSON tag
}

func parseHandlerComments(fn *ast.FuncDecl) *HandlerInfo {
	var method, path, inputType, outputType string
	var urlParams []string
	var headers []HeaderInfo
	for _, comment := range fn.Doc.List {
		text := comment.Text
		switch {
		case strings.Contains(text, "@Method"):
			method = strings.TrimSpace(strings.Split(text, "@Method")[1])
		case strings.Contains(text, "@Path"):
			path = strings.TrimSpace(strings.Split(text, "@Path")[1])
			// Extract URL parameters
			parts := strings.Split(path, "/")
			for _, part := range parts {
				if strings.HasPrefix(part, ":") {
					urlParams = append(urlParams, strings.TrimPrefix(part, ":"))
				}
			}
		case strings.Contains(text, "@Input"):
			inputType = strings.TrimSpace(strings.Split(text, "@Input")[1])
		case strings.Contains(text, "@Output"):
			outputType = strings.TrimSpace(strings.Split(text, "@Output")[1])
		case strings.Contains(text, "@Header"):
			headerInfo := parseHeaderDirective(strings.TrimSpace(strings.Split(text, "@Header")[1]))
			headers = append(headers, headerInfo)
		}
	}

	if method != "" && path != "" {
		return &HandlerInfo{
			Name:       formatHookName(fn.Name.Name),
			Method:     method,
			Path:       path,
			InputType:  inputType,
			OutputType: outputType,
			URLParams:  urlParams,
			Headers:    headers,
		}
	}

	return nil
}

func toTypescriptSafeHeader(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace hyphens with underscores
	s = strings.ReplaceAll(s, "-", "_")

	// Remove any character that's not a letter, number, or underscore
	reg := regexp.MustCompile("[^a-z0-9_]+")
	s = reg.ReplaceAllString(s, "")

	// Ensure it doesn't start with a number
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}

	return s
}

func parseHeaderDirective(directive string) HeaderInfo {
	parts := strings.Split(directive, ":")
	if len(parts) == 3 {
		return HeaderInfo{
			HeaderKey:  parts[1],
			SafeName:   toTypescriptSafeHeader(parts[1]),
			Source:     parts[0],
			StorageKey: parts[2],
		}
	} else if len(parts) == 2 {
		storageKey := ""
		if parts[0] != "input" {
			storageKey = parts[1]
		}
		return HeaderInfo{
			HeaderKey:  parts[1],
			SafeName:   toTypescriptSafeHeader(parts[1]),
			Source:     parts[0],
			StorageKey: storageKey,
		}
	}
	// If the directive is not in the expected format, default to input
	return HeaderInfo{
		HeaderKey:  directive,
		SafeName:   toTypescriptSafeHeader(directive),
		Source:     "input",
		StorageKey: "",
	}
}

func formatHookName(name string) string {
	name = strings.TrimSuffix(name, "Handler")
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

var defaultTypeMappings = map[string]string{
	"int":                 "number",
	"int8":                "number",
	"int16":               "number",
	"int32":               "number",
	"int64":               "number",
	"uint":                "number",
	"uint8":               "number",
	"uint16":              "number",
	"uint32":              "number",
	"uint64":              "number",
	"float32":             "number",
	"float64":             "number",
	"string":              "string",
	"bool":                "boolean",
	"byte":                "number",
	"rune":                "number",
	"error":               "Error",
	"uuid.UUID":           "string /* uuid */",
	"pgtypes.Timestamptz": "string /* date-time */",
	"types.Interface":     "any",
}

func formatCode(filePath string, prettierPath string) error {
	// Try Prettier first
	if prettierPath != "" {
		configPath, err := findPrettierConfig(filepath.Dir(filePath))
		args := []string{"--write"}
		if err == nil {
			args = append(args, "--config", configPath)
		}
		args = append(args, filePath)

		cmd := exec.Command(prettierPath, args...)
		output, err := cmd.CombinedOutput()
		if err == nil {
			fmt.Printf("Formatted %s with Prettier (config: %s)\n", filePath, configPath)
			return nil
		}
		fmt.Printf("Prettier failed: %v\n%s\n", err, output)
	}

	// try clang-format
	cmd := exec.Command("clang-format", "-i", filePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		fmt.Printf("Formatted %s with clang-format\n", filePath)
		return nil
	}

	// If all formatters fail, return an error
	return fmt.Errorf("failed to format %s: %v\n%s", filePath, err, output)
}

func isCommandAvailable(name string) bool {
	cmd := exec.Command("which", name)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func findPrettierConfig(startPath string) (string, error) {
	currentPath := startPath
	for {
		configFiles := []string{".prettierrc", ".prettierrc.json", ".prettierrc.yml", ".prettierrc.yaml", ".prettierrc.js", "prettier.config.js"}
		for _, configFile := range configFiles {
			possibleConfig := filepath.Join(currentPath, configFile)
			if _, err := os.Stat(possibleConfig); err == nil {
				return possibleConfig, nil
			}
		}

		// Stop if we're at the root directory
		if currentPath == filepath.Dir(currentPath) {
			break
		}

		// Move up one directory
		currentPath = filepath.Dir(currentPath)
	}

	return "", fmt.Errorf("could not find Prettier config")
}

// ExtractAfterLastSlash returns the substring after the last '/' in the input string.
// If no '/' is found, it returns the whole string.
func ExtractAfterLastSlash(s string) string {
	lastIndex := strings.LastIndex(s, "/")
	if lastIndex != -1 {
		return s[lastIndex+1:]
	}
	return s
}
