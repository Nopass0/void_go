package voidorm

import (
	"strings"
	"testing"
)

func TestRenderAndParseSchemaFile(t *testing.T) {
	now := "now()"
	project := &SchemaProject{
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
		Models: []SchemaModel{
			{
				Name: "AppUsers",
				Schema: CollectionSchema{
					Database:   "app",
					Collection: "users",
					Model:      "AppUsers",
					Fields: []SchemaField{
						{Name: "id", Type: FieldString, Required: true, IsID: true, MappedName: "_id"},
						{Name: "avatar", Type: FieldBlob, Required: false},
						{Name: "createdAt", Type: FieldDateTime, Required: true, DefaultExpr: &now, Default: &now},
					},
				},
			},
		},
	}

	rendered := RenderSchemaFile(project)
	if !strings.Contains(rendered, "model AppUsers") {
		t.Fatalf("rendered schema missing model: %s", rendered)
	}
	if !strings.Contains(rendered, "avatar Blob?") {
		t.Fatalf("rendered schema missing blob field: %s", rendered)
	}

	parsed, err := ParseSchemaFile(rendered)
	if err != nil {
		t.Fatalf("ParseSchemaFile() returned error: %v", err)
	}
	if len(parsed.Models) != 1 {
		t.Fatalf("expected one model, got %d", len(parsed.Models))
	}
	if parsed.Models[0].Schema.Collection != "users" {
		t.Fatalf("unexpected collection: %+v", parsed.Models[0].Schema)
	}
	if parsed.Models[0].Schema.Fields[1].Type != FieldBlob {
		t.Fatalf("expected blob field, got %+v", parsed.Models[0].Schema.Fields[1])
	}
}

func TestPlanSchemaDiffKeepsUndeclaredDatabases(t *testing.T) {
	current := &SchemaProject{
		Models: []SchemaModel{
			{
				Name: "KeepUsers",
				Schema: CollectionSchema{
					Database:   "keep",
					Collection: "users",
					Model:      "KeepUsers",
					Fields: []SchemaField{
						{Name: "id", Type: FieldString, Required: true, IsID: true, MappedName: "_id"},
					},
				},
			},
			{
				Name: "AppUsers",
				Schema: CollectionSchema{
					Database:   "app",
					Collection: "users",
					Model:      "AppUsers",
					Fields: []SchemaField{
						{Name: "id", Type: FieldString, Required: true, IsID: true, MappedName: "_id"},
					},
				},
			},
			{
				Name: "AppLogs",
				Schema: CollectionSchema{
					Database:   "app",
					Collection: "logs",
					Model:      "AppLogs",
					Fields: []SchemaField{
						{Name: "id", Type: FieldString, Required: true, IsID: true, MappedName: "_id"},
					},
				},
			},
		},
	}
	desired := &SchemaProject{
		Models: []SchemaModel{
			{
				Name: "AppUsers",
				Schema: CollectionSchema{
					Database:   "app",
					Collection: "users",
					Model:      "AppUsers",
					Fields: []SchemaField{
						{Name: "id", Type: FieldString, Required: true, IsID: true, MappedName: "_id"},
						{Name: "email", Type: FieldString, Required: true},
					},
				},
			},
		},
	}

	plan := planSchemaDiff(current, desired, true)
	if len(plan.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d: %+v", len(plan.Operations), plan.Operations)
	}
	if plan.Operations[0].Type != SchemaOpSetSchema {
		t.Fatalf("expected first op to update schema, got %+v", plan.Operations[0])
	}
	if plan.Operations[1].Type != SchemaOpDeleteCollection || plan.Operations[1].Database != "app" || plan.Operations[1].Collection != "logs" {
		t.Fatalf("expected only app/logs drop, got %+v", plan.Operations[1])
	}
}
