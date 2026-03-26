package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	voidorm "github.com/Nopass0/void_go"
)

const (
	defaultConfigPath    = ".voiddb-go/config.json"
	defaultSchemaPath    = ".voiddb-go/schema/app.schema"
	defaultOutputPath    = ".voiddb-go/generated/models.go"
	defaultMigrationsDir = ".voiddb-go/migrations"
	migrationDB          = "__void"
	migrationCollection  = "orm_migrations"
)

type cliConfig struct {
	Schema     string `json:"schema"`
	Output     string `json:"output"`
	Package    string `json:"package"`
	Migrations string `json:"migrations"`
}

type migrationRecord struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	CreatedAt string                 `json:"created_at"`
	ForceDrop bool                   `json:"force_drop,omitempty"`
	Project   *voidorm.SchemaProject `json:"project"`
	Plan      *voidorm.SchemaPlan    `json:"plan"`
	Checksum  string                 `json:"checksum"`
}

type migrationStatus struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	AppliedAt *string `json:"applied_at,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	loadEnvFiles(".env", ".env.local", ".voiddb-go/.env", ".voiddb-go/.env.local")

	args := normalizeCommand(os.Args[1:])
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printUsage()
		return nil
	}

	switch args[0] {
	case "init":
		return commandInit(args[1:])
	case "gen":
		return commandGen(args[1:])
	case "schema":
		if len(args) < 2 {
			return errors.New("schema command requires one of: pull, plan, push")
		}
		switch args[1] {
		case "pull":
			return commandPull(args[2:])
		case "plan":
			return commandPlan(args[2:])
		case "push":
			return commandPush(args[2:])
		default:
			return fmt.Errorf("unknown schema command: %s", args[1])
		}
	case "migrate":
		if len(args) < 2 {
			return errors.New("migrate command requires one of: dev, deploy, status")
		}
		switch args[1] {
		case "dev":
			return commandMigrateDev(args[2:])
		case "deploy":
			return commandMigrateDeploy(args[2:])
		case "status":
			return commandMigrateStatus(args[2:])
		default:
			return fmt.Errorf("unknown migrate command: %s", args[1])
		}
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func commandInit(args []string) error {
	cfg := loadConfig()
	cfg = resolvedConfig(cfg, args)

	if err := writeJSON(defaultConfigPath, cfg); err != nil {
		return err
	}

	project := defaultProject()
	if err := writeText(cfg.Schema, voidorm.RenderSchemaFile(project)); err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.Migrations, 0o755); err != nil {
		return err
	}
	if err := writeGeneratedTypes(project, cfg.Output, cfg.Package); err != nil {
		return err
	}

	envExample := "VOIDDB_URL=http://localhost:7700\nVOIDDB_USERNAME=admin\nVOIDDB_PASSWORD=admin\nVOIDDB_TOKEN=\n"
	if err := writeText(".env.example", envExample); err != nil {
		return err
	}

	fmt.Printf("Wrote config -> %s\n", defaultConfigPath)
	fmt.Printf("Wrote schema -> %s\n", cfg.Schema)
	fmt.Printf("Wrote migrations dir -> %s\n", cfg.Migrations)
	fmt.Printf("Generated types -> %s\n", cfg.Output)
	return nil
}

func commandPull(args []string) error {
	cfg := resolvedConfig(loadConfig(), args)
	client, err := newClientFromConfig(args)
	if err != nil {
		return err
	}

	project, err := client.Schema().Pull(ctx())
	if err != nil {
		return err
	}

	out := firstNonEmpty(argValue(args, "out", ""), cfg.Schema)
	if err := writeText(out, voidorm.RenderSchemaFile(project)); err != nil {
		return err
	}
	if err := writeGeneratedTypes(project, cfg.Output, cfg.Package); err != nil {
		return err
	}

	fmt.Printf("Pulled schema -> %s\n", out)
	fmt.Printf("Generated types -> %s\n", cfg.Output)
	return nil
}

func commandGen(args []string) error {
	cfg := resolvedConfig(loadConfig(), args)
	project, err := readProject(cfg.Schema)
	if err != nil {
		return err
	}
	return writeGeneratedTypes(project, cfg.Output, cfg.Package)
}

func commandPlan(args []string) error {
	cfg := resolvedConfig(loadConfig(), args)
	project, err := readProject(cfg.Schema)
	if err != nil {
		return err
	}

	client, err := newClientFromConfig(args)
	if err != nil {
		return err
	}

	plan, err := client.Schema().Plan(ctx(), project, &voidorm.SchemaPlanOptions{
		ForceDrop: hasFlag(args, "force-drop"),
	})
	if err != nil {
		return err
	}

	return printPlan(plan, hasFlag(args, "json"))
}

func commandPush(args []string) error {
	cfg := resolvedConfig(loadConfig(), args)
	project, err := readProject(cfg.Schema)
	if err != nil {
		return err
	}

	client, err := newClientFromConfig(args)
	if err != nil {
		return err
	}

	plan, err := client.Schema().Push(ctx(), project, &voidorm.SchemaPushOptions{
		DryRun:    hasFlag(args, "dry-run"),
		ForceDrop: hasFlag(args, "force-drop"),
	})
	if err != nil {
		return err
	}

	if err := printPlan(plan, hasFlag(args, "json")); err != nil {
		return err
	}
	if hasFlag(args, "dry-run") {
		return nil
	}

	return writeGeneratedTypes(project, cfg.Output, cfg.Package)
}

func commandMigrateDev(args []string) error {
	cfg := resolvedConfig(loadConfig(), args)
	project, err := readProject(cfg.Schema)
	if err != nil {
		return err
	}

	client, err := newClientFromConfig(args)
	if err != nil {
		return err
	}

	forceDrop := hasFlag(args, "force-drop")
	plan, err := client.Schema().Plan(ctx(), project, &voidorm.SchemaPlanOptions{
		ForceDrop: forceDrop,
	})
	if err != nil {
		return err
	}
	if len(plan.Operations) == 0 {
		fmt.Println("No schema changes.")
		return nil
	}

	name := firstNonEmpty(argValue(args, "name", ""), "migration")
	record := migrationRecord{
		ID:        migrationID(name),
		Name:      name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		ForceDrop: forceDrop,
		Project:   project,
		Plan:      plan,
	}
	record.Checksum = checksumForMigration(record)

	targetDir := filepath.Join(cfg.Migrations, record.ID)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	if err := writeText(filepath.Join(targetDir, "schema.schema"), voidorm.RenderSchemaFile(project)); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(targetDir, "plan.json"), plan); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(targetDir, "migration.json"), record); err != nil {
		return err
	}

	fmt.Printf("Created migration %s\n", record.ID)
	if err := printPlan(plan, false); err != nil {
		return err
	}

	if hasFlag(args, "create-only") {
		return writeGeneratedTypes(project, cfg.Output, cfg.Package)
	}

	if _, err := client.Schema().Push(ctx(), project, &voidorm.SchemaPushOptions{
		ForceDrop: forceDrop,
	}); err != nil {
		return err
	}
	if err := markMigrationApplied(client, record); err != nil {
		return err
	}

	fmt.Printf("Applied migration %s\n", record.ID)
	return writeGeneratedTypes(project, cfg.Output, cfg.Package)
}

func commandMigrateDeploy(args []string) error {
	cfg := resolvedConfig(loadConfig(), args)
	client, err := newClientFromConfig(args)
	if err != nil {
		return err
	}

	migrations, err := loadLocalMigrations(cfg.Migrations)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		fmt.Printf("No migrations found in %s\n", cfg.Migrations)
		return nil
	}

	_, applied, err := loadAppliedMigrations(client)
	if err != nil {
		return err
	}

	appliedCount := 0
	for _, migration := range migrations {
		checksum := checksumForMigration(migration)
		if migration.Checksum != "" && migration.Checksum != checksum {
			return fmt.Errorf("migration checksum mismatch for %s", migration.ID)
		}
		migration.Checksum = checksum

		if _, ok := applied[migration.ID]; ok {
			fmt.Printf("Skipping already applied migration %s\n", migration.ID)
			continue
		}

		if _, err := client.Schema().Push(ctx(), migration.Project, &voidorm.SchemaPushOptions{
			ForceDrop: migration.ForceDrop,
		}); err != nil {
			return err
		}
		if err := markMigrationApplied(client, migration); err != nil {
			return err
		}
		fmt.Printf("Applied migration %s\n", migration.ID)
		appliedCount++
	}

	if appliedCount == 0 {
		fmt.Println("All migrations are already applied.")
		return nil
	}

	project, err := client.Schema().Pull(ctx())
	if err != nil {
		return err
	}
	if err := writeText(cfg.Schema, voidorm.RenderSchemaFile(project)); err != nil {
		return err
	}
	return writeGeneratedTypes(project, cfg.Output, cfg.Package)
}

func commandMigrateStatus(args []string) error {
	cfg := resolvedConfig(loadConfig(), args)
	client, err := newClientFromConfig(args)
	if err != nil {
		return err
	}

	migrations, err := loadLocalMigrations(cfg.Migrations)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		fmt.Printf("No migrations found in %s\n", cfg.Migrations)
		return nil
	}

	_, applied, err := loadAppliedMigrations(client)
	if err != nil {
		return err
	}

	rows := make([]migrationStatus, 0, len(migrations))
	for _, migration := range migrations {
		row := migrationStatus{
			ID:     migration.ID,
			Name:   migration.Name,
			Status: "PENDING",
		}
		if doc, ok := applied[migration.ID]; ok {
			row.Status = "APPLIED"
			if appliedAt := docString(doc, "applied_at"); appliedAt != "" {
				row.AppliedAt = stringPtr(appliedAt)
			}
		}
		rows = append(rows, row)
	}

	if hasFlag(args, "json") {
		data, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", data)
		return nil
	}

	for _, row := range rows {
		fmt.Printf("%-8s %s %s\n", row.Status, row.ID, row.Name)
	}
	return nil
}

func newClientFromConfig(args []string) (*voidorm.Client, error) {
	client, err := voidorm.New(voidorm.Config{
		URL:   firstNonEmpty(argValue(args, "url", ""), os.Getenv("VOIDDB_URL"), os.Getenv("VOID_URL")),
		Token: firstNonEmpty(argValue(args, "token", ""), os.Getenv("VOIDDB_TOKEN"), os.Getenv("VOID_TOKEN")),
	})
	if err != nil {
		return nil, err
	}

	if client.Token() == "" {
		username := firstNonEmpty(argValue(args, "username", ""), os.Getenv("VOIDDB_USERNAME"), os.Getenv("VOID_USERNAME"))
		password := firstNonEmpty(argValue(args, "password", ""), os.Getenv("VOIDDB_PASSWORD"), os.Getenv("VOID_PASSWORD"))
		if username == "" || password == "" {
			return nil, errors.New("VOIDDB_URL and auth are required; set VOIDDB_TOKEN or VOIDDB_USERNAME/VOIDDB_PASSWORD")
		}
		if _, err := client.Login(ctx(), username, password); err != nil {
			return nil, err
		}
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

func normalizeCommand(args []string) []string {
	if len(args) == 0 {
		return []string{"help"}
	}

	aliases := map[string][]string{
		"pull":        {"schema", "pull"},
		"schema:pull": {"schema", "pull"},
		"plan":        {"schema", "plan"},
		"schema:plan": {"schema", "plan"},
		"push":        {"schema", "push"},
		"schema:push": {"schema", "push"},
		"generate":    {"gen"},
		"types":       {"gen"},
		"gen":         {"gen"},
		"dev":         {"migrate", "dev"},
		"deploy":      {"migrate", "deploy"},
		"status":      {"migrate", "status"},
	}
	if mapped, ok := aliases[args[0]]; ok {
		return append(mapped, args[1:]...)
	}
	return args
}

func printUsage() {
	fmt.Print(`VoidDB Go SDK CLI

Short commands:
  vdbgo init
  vdbgo pull
  vdbgo plan
  vdbgo push
  vdbgo gen
  vdbgo dev --name add_users
  vdbgo deploy
  vdbgo status

Long commands:
  vdbgo schema pull [--out file]
  vdbgo schema plan [--schema file] [--force-drop] [--json]
  vdbgo schema push [--schema file] [--dry-run] [--force-drop] [--json]
  vdbgo migrate dev --name name [--schema file] [--dir dir] [--create-only] [--force-drop]
  vdbgo migrate deploy [--dir dir]
  vdbgo migrate status [--dir dir] [--json]

Expected env:
  VOIDDB_URL
  VOIDDB_TOKEN
  or VOIDDB_USERNAME / VOIDDB_PASSWORD
`)
}

func hasFlag(args []string, name string) bool {
	needle := "--" + name
	for _, arg := range args {
		if arg == needle {
			return true
		}
	}
	return false
}

func argValue(args []string, name, fallback string) string {
	exact := "--" + name
	prefixed := exact + "="
	for index := 0; index < len(args); index++ {
		token := args[index]
		if token == exact {
			if index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
				return args[index+1]
			}
			return fallback
		}
		if strings.HasPrefix(token, prefixed) {
			return strings.TrimPrefix(token, prefixed)
		}
	}
	return fallback
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
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
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
	if cfg.Migrations == "" {
		cfg.Migrations = defaultMigrationsDir
	}
	return cfg
}

func resolvedConfig(cfg cliConfig, args []string) cliConfig {
	cfg.Schema = firstNonEmpty(argValue(args, "schema", ""), cfg.Schema, defaultSchemaPath)
	cfg.Output = firstNonEmpty(argValue(args, "output", ""), cfg.Output, defaultOutputPath)
	cfg.Package = firstNonEmpty(argValue(args, "package", ""), cfg.Package, "voiddbgen")
	cfg.Migrations = firstNonEmpty(argValue(args, "dir", ""), cfg.Migrations, defaultMigrationsDir)
	return cfg
}

func readProject(path string) (*voidorm.SchemaProject, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return voidorm.ParseSchemaFile(string(source))
}

func writeGeneratedTypes(project *voidorm.SchemaProject, outputPath, packageName string) error {
	generated := voidorm.GenerateGoTypes(project, voidorm.TypegenOptions{Package: packageName})
	if err := writeText(outputPath, generated); err != nil {
		return err
	}
	fmt.Printf("Generated types -> %s\n", outputPath)
	return nil
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

func printPlan(plan *voidorm.SchemaPlan, asJSON bool) error {
	if asJSON {
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", data)
		return nil
	}
	if plan == nil || len(plan.Operations) == 0 {
		fmt.Println("No schema changes.")
		return nil
	}
	for idx, op := range plan.Operations {
		fmt.Printf("%d. %s\n", idx+1, op.Summary)
	}
	return nil
}

func loadLocalMigrations(dir string) ([]migrationRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	out := make([]migrationRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		target := filepath.Join(dir, entry.Name(), "migration.json")
		data, err := os.ReadFile(target)
		if err != nil {
			return nil, err
		}
		var record migrationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, fmt.Errorf("parse %s: %w", target, err)
		}
		out = append(out, record)
	}
	return out, nil
}

func ensureMigrationStore(client *voidorm.Client) (*voidorm.Collection, error) {
	dbs, err := client.ListDatabases(ctx())
	if err != nil {
		return nil, err
	}
	if !contains(dbs, migrationDB) {
		if err := client.CreateDatabase(ctx(), migrationDB); err != nil {
			return nil, err
		}
	}

	db := client.DB(migrationDB)
	collections, err := db.ListCollections(ctx())
	if err != nil {
		return nil, err
	}
	if !contains(collections, migrationCollection) {
		if err := db.CreateCollection(ctx(), migrationCollection); err != nil {
			return nil, err
		}
	}
	return db.Collection(migrationCollection), nil
}

func loadAppliedMigrations(client *voidorm.Client) (*voidorm.Collection, map[string]voidorm.Doc, error) {
	col, err := ensureMigrationStore(client)
	if err != nil {
		return nil, nil, err
	}
	rows, err := col.Find(ctx(), nil)
	if err != nil {
		return nil, nil, err
	}
	out := map[string]voidorm.Doc{}
	for _, row := range rows {
		if id := docString(row, "_id"); id != "" {
			out[id] = row
		}
	}
	return col, out, nil
}

func markMigrationApplied(client *voidorm.Client, migration migrationRecord) error {
	col, existing, err := loadAppliedMigrations(client)
	if err != nil {
		return err
	}

	payload := voidorm.Doc{
		"name":       migration.Name,
		"checksum":   migration.Checksum,
		"applied_at": time.Now().UTC().Format(time.RFC3339),
		"source":     "vdbgo",
	}
	if _, ok := existing[migration.ID]; ok {
		_, err = col.Patch(ctx(), migration.ID, payload)
		return err
	}
	payload["_id"] = migration.ID
	_, err = col.Insert(ctx(), payload)
	return err
}

func checksumForMigration(m migrationRecord) string {
	payload := struct {
		ID        string                 `json:"id"`
		Name      string                 `json:"name"`
		ForceDrop bool                   `json:"force_drop,omitempty"`
		Project   *voidorm.SchemaProject `json:"project"`
		Plan      *voidorm.SchemaPlan    `json:"plan"`
	}{
		ID:        m.ID,
		Name:      m.Name,
		ForceDrop: m.ForceDrop,
		Project:   m.Project,
		Plan:      m.Plan,
	}

	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func migrationID(name string) string {
	stamp := time.Now().UTC().Format("20060102T150405Z")
	return stamp + "_" + slugify(name)
}

func slugify(value string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.TrimSpace(strings.ToLower(value)) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case !lastUnderscore:
			b.WriteRune('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "migration"
	}
	return out
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func docString(doc voidorm.Doc, key string) string {
	raw, ok := doc[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringPtr(value string) *string {
	return &value
}

func ctx() context.Context {
	return context.Background()
}
