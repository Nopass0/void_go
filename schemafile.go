package voidorm

import (
	"bufio"
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// SchemaDatasource describes the datasource block in a .schema file.
type SchemaDatasource struct {
	Name     string
	Provider string
	URL      string
}

// SchemaGenerator describes the generator block in a .schema file.
type SchemaGenerator struct {
	Name     string
	Provider string
	Output   string
}

// SchemaModel binds a rendered model name to its collection schema.
type SchemaModel struct {
	Name   string
	Schema CollectionSchema
}

// SchemaProject is the full .schema document model used by the Go SDK.
type SchemaProject struct {
	Datasource SchemaDatasource
	Generator  SchemaGenerator
	Models     []SchemaModel
}

// SchemaManager provides schema pull helpers for SDK and CLI usage.
type SchemaManager struct {
	client *Client
}

// Schema returns a schema helper bound to this client.
func (c *Client) Schema() *SchemaManager {
	return &SchemaManager{client: c}
}

// Pull fetches all collection schemas from the server and returns a schema project.
func (m *SchemaManager) Pull(ctx context.Context) (*SchemaProject, error) {
	databases, err := m.client.ListDatabases(ctx)
	if err != nil {
		return nil, err
	}

	usedNames := map[string]int{}
	var models []SchemaModel

	for _, database := range databases {
		if database == "__void" {
			continue
		}

		collections, err := m.client.DB(database).ListCollections(ctx)
		if err != nil {
			return nil, err
		}

		for _, collection := range collections {
			schema, err := m.client.DB(database).Collection(collection).GetSchema(ctx)
			if err != nil {
				return nil, err
			}

			copySchema := *schema
			copySchema.Database = database
			copySchema.Collection = collection

			name := copySchema.Model
			if name == "" {
				name = defaultModelName(database, collection)
			}
			name = uniqueModelName(name, usedNames)
			copySchema.Model = name

			models = append(models, SchemaModel{
				Name:   name,
				Schema: copySchema,
			})
		}
	}

	sort.Slice(models, func(i, j int) bool {
		left := models[i].Schema.Database + "/" + models[i].Schema.Collection
		right := models[j].Schema.Database + "/" + models[j].Schema.Collection
		return left < right
	})

	return &SchemaProject{
		Datasource: SchemaDatasource{
			Name:     "db",
			Provider: "voiddb",
			URL:      `env("VOIDDB_URL")`,
		},
		Generator: SchemaGenerator{
			Name:     "client",
			Provider: "voiddb-client-go",
			Output:   "../generated",
		},
		Models: models,
	}, nil
}

// RenderSchemaFile renders the schema project to a .schema file.
func RenderSchemaFile(project *SchemaProject) string {
	if project == nil {
		return ""
	}

	var b strings.Builder
	datasource := project.Datasource
	if datasource.Name == "" {
		datasource.Name = "db"
	}
	if datasource.Provider == "" {
		datasource.Provider = "voiddb"
	}
	if datasource.URL == "" {
		datasource.URL = `env("VOIDDB_URL")`
	}

	generator := project.Generator
	if generator.Name == "" {
		generator.Name = "client"
	}
	if generator.Provider == "" {
		generator.Provider = "voiddb-client-go"
	}
	if generator.Output == "" {
		generator.Output = "../generated"
	}

	fmt.Fprintf(&b, "datasource %s {\n", datasource.Name)
	fmt.Fprintf(&b, "  provider = %q\n", datasource.Provider)
	fmt.Fprintf(&b, "  url      = %s\n", datasource.URL)
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "generator %s {\n", generator.Name)
	fmt.Fprintf(&b, "  provider = %q\n", generator.Provider)
	fmt.Fprintf(&b, "  output   = %q\n", generator.Output)
	b.WriteString("}\n\n")

	grouped := map[string][]SchemaModel{}
	var databases []string
	for _, model := range project.Models {
		database := model.Schema.Database
		if database == "" {
			database = "app"
		}
		if _, ok := grouped[database]; !ok {
			databases = append(databases, database)
		}
		grouped[database] = append(grouped[database], model)
	}
	sort.Strings(databases)

	for _, database := range databases {
		fmt.Fprintf(&b, "database {\n  name = %q\n\n", database)
		models := grouped[database]
		sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })

		for _, model := range models {
			fmt.Fprintf(&b, "  model %s {\n", model.Name)
			for _, field := range model.Schema.Fields {
				fmt.Fprintf(&b, "    %s %s", field.Name, schemaTypeLiteral(field))
				if field.IsID {
					b.WriteString(" @id")
				}
				if field.Unique {
					b.WriteString(" @unique")
				}
				if field.DefaultExpr != nil && *field.DefaultExpr != "" {
					fmt.Fprintf(&b, " @default(%s)", *field.DefaultExpr)
				} else if field.Default != nil && *field.Default != "" {
					fmt.Fprintf(&b, " @default(%s)", quoteDefaultLiteral(field, *field.Default))
				}
				if field.AutoUpdatedAt {
					b.WriteString(" @updatedAt")
				}
				if field.MappedName != "" {
					fmt.Fprintf(&b, " @map(%q)", field.MappedName)
				}
				b.WriteString("\n")
			}

			for _, index := range model.Schema.Indexes {
				prefix := "@@index"
				if index.Unique {
					prefix = "@@unique"
				}
				if index.Primary {
					prefix = "@@id"
				}
				fmt.Fprintf(&b, "    %s([%s])", prefix, strings.Join(index.Fields, ", "))
				if index.Name != "" {
					fmt.Fprintf(&b, " @name(%q)", index.Name)
				}
				b.WriteString("\n")
			}

			if model.Schema.Collection != "" {
				fmt.Fprintf(&b, "    @@map(%q)\n", model.Schema.Collection)
			}
			b.WriteString("  }\n\n")
		}
		b.WriteString("}\n\n")
	}

	return strings.TrimSpace(b.String()) + "\n"
}

// ParseSchemaFile parses the simplified .schema format used by the Go SDK.
func ParseSchemaFile(source string) (*SchemaProject, error) {
	project := &SchemaProject{}
	scanner := bufio.NewScanner(strings.NewReader(source))

	var currentDB string
	var section string
	var currentModel *SchemaModel

	for scanner.Scan() {
		line := cleanSchemaLine(scanner.Text())
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "datasource "):
			section = "datasource"
			project.Datasource.Name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "datasource "), "{"))
		case strings.HasPrefix(line, "generator "):
			section = "generator"
			project.Generator.Name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "generator "), "{"))
		case strings.HasPrefix(line, "provider = "):
			value := strings.TrimSpace(strings.TrimPrefix(line, "provider = "))
			value = strings.Trim(value, `"`)
			if section == "datasource" {
				project.Datasource.Provider = value
			} else if section == "generator" {
				project.Generator.Provider = value
			}
		case strings.HasPrefix(line, "url      = ") || strings.HasPrefix(line, "url = "):
			value := strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
			if section == "datasource" {
				project.Datasource.URL = value
			}
		case strings.HasPrefix(line, "output   = ") || strings.HasPrefix(line, "output = "):
			value := strings.Trim(strings.TrimSpace(strings.SplitN(line, "=", 2)[1]), `"`)
			if section == "generator" {
				project.Generator.Output = value
			}
		case line == "database {":
			section = "database"
			currentDB = ""
		case strings.HasPrefix(line, "name = ") && currentModel == nil:
			currentDB = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "name = ")), `"`)
		case strings.HasPrefix(line, "model "):
			section = "model"
			name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "model "), "{"))
			currentModel = &SchemaModel{
				Name: name,
				Schema: CollectionSchema{
					Database: currentDB,
					Model:    name,
				},
			}
		case line == "}" && currentModel != nil:
			if currentModel.Schema.Collection == "" {
				currentModel.Schema.Collection = defaultCollectionName(currentModel.Name)
			}
			project.Models = append(project.Models, *currentModel)
			currentModel = nil
			section = "database"
		case line == "}" && currentModel == nil:
			section = ""
			currentDB = ""
		default:
			if currentModel == nil {
				continue
			}

			if strings.HasPrefix(line, "@@map(") {
				value, err := extractCallArg(line)
				if err != nil {
					return nil, err
				}
				currentModel.Schema.Collection = value
				continue
			}

			if strings.HasPrefix(line, "@@") {
				index, err := parseIndexLine(line)
				if err != nil {
					return nil, err
				}
				currentModel.Schema.Indexes = append(currentModel.Schema.Indexes, *index)
				continue
			}

			field, err := parseFieldLine(line)
			if err != nil {
				return nil, err
			}
			currentModel.Schema.Fields = append(currentModel.Schema.Fields, *field)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return project, nil
}

func cleanSchemaLine(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		line = line[:idx]
	}
	return strings.TrimSpace(line)
}

func parseFieldLine(line string) (*SchemaField, error) {
	tokens := strings.Fields(line)
	if len(tokens) < 2 {
		return nil, fmt.Errorf("invalid field line: %s", line)
	}

	field := &SchemaField{
		Name: tokens[0],
	}

	baseType, required, list := parseSchemaTypeToken(tokens[1])
	field.Type = baseType
	field.Required = required
	field.List = list
	field.PrismaType = prismaTypeForField(*field)

	for _, token := range tokens[2:] {
		switch {
		case token == "@id":
			field.IsID = true
			field.Required = true
		case token == "@unique":
			field.Unique = true
		case token == "@updatedAt":
			field.AutoUpdatedAt = true
		case strings.HasPrefix(token, "@default("):
			value, err := extractCallArg(token)
			if err != nil {
				return nil, err
			}
			if isExpressionDefault(value) {
				field.DefaultExpr = stringPtr(value)
				field.Default = stringPtr(value)
			} else {
				literal := strings.Trim(value, `"`)
				field.Default = stringPtr(literal)
			}
		case strings.HasPrefix(token, "@map("):
			value, err := extractCallArg(token)
			if err != nil {
				return nil, err
			}
			field.MappedName = value
		}
	}

	if field.IsID && field.MappedName == "" && field.Name != "_id" {
		field.MappedName = "_id"
	}
	field.PrismaType = prismaTypeForField(*field)
	return field, nil
}

func parseIndexLine(line string) (*SchemaIndex, error) {
	index := &SchemaIndex{}
	switch {
	case strings.HasPrefix(line, "@@unique("):
		index.Unique = true
	case strings.HasPrefix(line, "@@id("):
		index.Primary = true
	default:
	}

	start := strings.Index(line, "[")
	end := strings.Index(line, "]")
	if start < 0 || end < 0 || end <= start {
		return nil, fmt.Errorf("invalid index line: %s", line)
	}
	fields := strings.Split(line[start+1:end], ",")
	for _, field := range fields {
		index.Fields = append(index.Fields, strings.TrimSpace(field))
	}
	if at := strings.Index(line, "@name("); at >= 0 {
		name, err := extractCallArg(line[at:])
		if err != nil {
			return nil, err
		}
		index.Name = name
	}
	return index, nil
}

func parseSchemaTypeToken(token string) (SchemaFieldType, bool, bool) {
	list := strings.HasSuffix(token, "[]")
	if list {
		token = strings.TrimSuffix(token, "[]")
	}
	required := !strings.HasSuffix(token, "?")
	token = strings.TrimSuffix(token, "?")

	switch token {
	case "String":
		return FieldString, required, list
	case "Float", "Int", "BigInt", "Decimal":
		return FieldNumber, required, list
	case "Boolean":
		return FieldBoolean, required, list
	case "DateTime":
		return FieldDateTime, required, list
	case "Json":
		return FieldObject, required, list
	case "Blob":
		return FieldBlob, required, list
	default:
		return FieldString, required, list
	}
}

func schemaTypeLiteral(field SchemaField) string {
	base := prismaTypeForField(field)
	if field.List {
		base += "[]"
	} else if !field.Required {
		base += "?"
	}
	return base
}

func quoteDefaultLiteral(field SchemaField, value string) string {
	switch field.Type {
	case FieldString, FieldDateTime, FieldBlob, FieldObject:
		return strconv.Quote(value)
	default:
		return value
	}
}

func isExpressionDefault(value string) bool {
	return strings.HasSuffix(value, "()")
}

func extractCallArg(token string) (string, error) {
	start := strings.Index(token, "(")
	end := strings.LastIndex(token, ")")
	if start < 0 || end < 0 || end <= start {
		return "", fmt.Errorf("invalid decorator token: %s", token)
	}
	value := strings.TrimSpace(token[start+1 : end])
	return strings.Trim(value, `"`), nil
}

func prismaTypeForField(field SchemaField) string {
	if field.PrismaType != "" {
		return field.PrismaType
	}
	switch field.Type {
	case FieldString:
		return "String"
	case FieldNumber:
		return "Float"
	case FieldBoolean:
		return "Boolean"
	case FieldDateTime:
		return "DateTime"
	case FieldBlob:
		return "Blob"
	default:
		return "Json"
	}
}

func defaultModelName(database, collection string) string {
	base := toPascal(collection)
	if database == "" || database == "default" {
		return base
	}
	return toPascal(database) + base
}

func uniqueModelName(name string, used map[string]int) string {
	seen := used[name]
	if seen == 0 {
		used[name] = 1
		return name
	}
	used[name] = seen + 1
	return fmt.Sprintf("%s%d", name, used[name])
}

func defaultCollectionName(model string) string {
	if model == "" {
		return "items"
	}
	return strings.ToLower(model[:1]) + model[1:] + "s"
}

func toPascal(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	})
	var out strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		out.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			out.WriteString(part[1:])
		}
	}
	return out.String()
}

func stringPtr(value string) *string {
	return &value
}
