package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	voidorm "github.com/Nopass0/void_go"
)

const (
	defaultConfigPath = ".voiddb-go/config.json"
	defaultSchemaPath = ".voiddb-go/schema/app.schema"
	defaultOutputPath = ".voiddb-go/generated/models.go"
)

type cliConfig struct {
	Schema  string `json:"schema"`
	Output  string `json:"output"`
	Package string `json:"package"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	loadEnvFiles(".env", ".env.local", ".voiddb-go/.env", ".voiddb-go/.env.local")

	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return nil
	}

	cmd := normalizeCommand(args[0])
	switch cmd {
	case "init":
		return commandInit()
	case "pull":
		return commandPull()
	case "gen":
		return commandGen()
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func commandInit() error {
	cfg := loadConfig()
	if cfg.Schema == "" {
		cfg.Schema = defaultSchemaPath
	}
	if cfg.Output == "" {
		cfg.Output = defaultOutputPath
	}
	if cfg.Package == "" {
		cfg.Package = "voiddbgen"
	}

	if err := writeJSON(defaultConfigPath, cfg); err != nil {
		return err
	}

	project := defaultProject()
	if err := writeText(cfg.Schema, voidorm.RenderSchemaFile(project)); err != nil {
		return err
	}

	generated := voidorm.GenerateGoTypes(project, voidorm.TypegenOptions{Package: cfg.Package})
	if err := writeText(cfg.Output, generated); err != nil {
		return err
	}

	envExample := "VOIDDB_URL=http://localhost:7700\nVOIDDB_USERNAME=admin\nVOIDDB_PASSWORD=admin\nVOIDDB_TOKEN=\n"
	if err := writeText(".env.example", envExample); err != nil {
		return err
	}

	fmt.Printf("Wrote config -> %s\n", defaultConfigPath)
	fmt.Printf("Wrote schema -> %s\n", cfg.Schema)
	fmt.Printf("Generated types -> %s\n", cfg.Output)
	return nil
}

func commandPull() error {
	cfg := loadConfig()
	client, err := newClientFromConfig()
	if err != nil {
		return err
	}

	project, err := client.Schema().Pull(ctx())
	if err != nil {
		return err
	}

	if cfg.Schema == "" {
		cfg.Schema = defaultSchemaPath
	}
	if cfg.Output == "" {
		cfg.Output = defaultOutputPath
	}
	if cfg.Package == "" {
		cfg.Package = "voiddbgen"
	}

	if err := writeText(cfg.Schema, voidorm.RenderSchemaFile(project)); err != nil {
		return err
	}
	generated := voidorm.GenerateGoTypes(project, voidorm.TypegenOptions{Package: cfg.Package})
	if err := writeText(cfg.Output, generated); err != nil {
		return err
	}

	fmt.Printf("Pulled schema -> %s\n", cfg.Schema)
	fmt.Printf("Generated types -> %s\n", cfg.Output)
	return nil
}

func commandGen() error {
	cfg := loadConfig()
	if cfg.Schema == "" {
		cfg.Schema = defaultSchemaPath
	}
	if cfg.Output == "" {
		cfg.Output = defaultOutputPath
	}
	if cfg.Package == "" {
		cfg.Package = "voiddbgen"
	}

	source, err := os.ReadFile(cfg.Schema)
	if err != nil {
		return err
	}
	project, err := voidorm.ParseSchemaFile(string(source))
	if err != nil {
		return err
	}
	generated := voidorm.GenerateGoTypes(project, voidorm.TypegenOptions{Package: cfg.Package})
	if err := writeText(cfg.Output, generated); err != nil {
		return err
	}
	fmt.Printf("Generated types -> %s\n", cfg.Output)
	return nil
}

func newClientFromConfig() (*voidorm.Client, error) {
	cfg := loadConfig()
	client, err := voidorm.New(voidorm.Config{
		URL:   firstNonEmpty(os.Getenv("VOIDDB_URL"), os.Getenv("VOID_URL")),
		Token: firstNonEmpty(os.Getenv("VOIDDB_TOKEN"), os.Getenv("VOID_TOKEN")),
	})
	if err != nil {
		return nil, err
	}

	if client.Token() == "" {
		username := firstNonEmpty(os.Getenv("VOIDDB_USERNAME"), os.Getenv("VOID_USERNAME"))
		password := firstNonEmpty(os.Getenv("VOIDDB_PASSWORD"), os.Getenv("VOID_PASSWORD"))
		if username == "" || password == "" {
			return nil, errors.New("VOIDDB_URL and auth are required; set VOIDDB_TOKEN or VOIDDB_USERNAME/VOIDDB_PASSWORD")
		}
		if _, err := client.Login(ctx(), username, password); err != nil {
			return nil, err
		}
	}

	if cfg.Package == "" {
		cfg.Package = "voiddbgen"
	}
	return client, nil
}

func defaultProject() *voidorm.SchemaProject {
	now := "now()"
	return &voidorm.SchemaProject{
		Datasource: voidorm.SchemaDatasource{
			Name:     "db",
			Provider: "voiddb",
			URL:      `env("VOIDDB_URL")`,
		},
		Generator: voidorm.SchemaGenerator{
			Name:     "client",
			Provider: "voiddb-client-go",
			Output:   "../generated",
		},
		Models: []voidorm.SchemaModel{
			{
				Name: "User",
				Schema: voidorm.CollectionSchema{
					Database:   "app",
					Collection: "users",
					Model:      "User",
					Fields: []voidorm.SchemaField{
						{Name: "id", Type: voidorm.FieldString, Required: true, IsID: true, PrismaType: "String", MappedName: "_id"},
						{Name: "email", Type: voidorm.FieldString, Required: true, Unique: true, PrismaType: "String"},
						{Name: "name", Type: voidorm.FieldString, Required: true, PrismaType: "String"},
						{Name: "createdAt", Type: voidorm.FieldDateTime, Required: true, PrismaType: "DateTime", DefaultExpr: &now, Default: &now},
						{Name: "updatedAt", Type: voidorm.FieldDateTime, Required: true, PrismaType: "DateTime", DefaultExpr: &now, Default: &now, AutoUpdatedAt: true},
					},
				},
			},
		},
	}
}

func normalizeCommand(cmd string) string {
	switch cmd {
	case "schema", "pull", "schema:pull":
		return "pull"
	case "generate", "types", "gen":
		return "gen"
	default:
		return cmd
	}
}

func printUsage() {
	fmt.Print(`VoidDB Go SDK CLI

Commands:
  vdbgo init    create .voiddb-go config, starter schema, and generated models
  vdbgo pull    pull live schema from VoidDB and regenerate Go types
  vdbgo gen     regenerate Go types from the local .schema file

Expected env:
  VOIDDB_URL
  VOIDDB_TOKEN
  or VOIDDB_USERNAME / VOIDDB_PASSWORD
`)
}

func loadEnvFiles(paths ...string) {
	for _, rel := range paths {
		data, err := os.ReadFile(rel)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, value)
			}
		}
	}
}

func loadConfig() cliConfig {
	var cfg cliConfig
	data, err := os.ReadFile(defaultConfigPath)
	if err != nil {
		return cliConfig{
			Schema:  defaultSchemaPath,
			Output:  defaultOutputPath,
			Package: "voiddbgen",
		}
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.Schema == "" {
		cfg.Schema = defaultSchemaPath
	}
	if cfg.Output == "" {
		cfg.Output = defaultOutputPath
	}
	if cfg.Package == "" {
		cfg.Package = "voiddbgen"
	}
	return cfg
}

func writeJSON(path string, value interface{}) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeText(path, string(data)+"\n")
}

func writeText(path string, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), 0o644)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ctx() context.Context {
	return context.Background()
}
