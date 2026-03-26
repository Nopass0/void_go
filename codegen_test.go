package voidorm

import (
	"strings"
	"testing"
)

func TestGenerateGoTypes(t *testing.T) {
	project := &SchemaProject{
		Models: []SchemaModel{
			{
				Name: "LowkeyUsers",
				Schema: CollectionSchema{
					Database:   "lowkey",
					Collection: "users",
					Model:      "LowkeyUsers",
					Fields: []SchemaField{
						{Name: "id", Type: FieldString, Required: true, IsID: true, MappedName: "_id"},
						{Name: "name", Type: FieldString, Required: true},
						{Name: "avatar", Type: FieldBlob, Required: false},
					},
				},
			},
		},
	}

	source := GenerateGoTypes(project, TypegenOptions{Package: "models"})
	for _, expected := range []string{
		"package models",
		"type VoidDBBlobRef struct",
		"type LowkeyUsers struct",
		"type LowkeyUsersCreateInput struct",
		"type LowkeyUsersPatchInput struct",
		"Avatar *VoidDBBlobRef",
		"`json:\"_id\"`",
	} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated source missing %q:\n%s", expected, source)
		}
	}
}
