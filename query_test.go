package voidorm

import (
	"encoding/json"
	"testing"
)

func TestQuerySpecSingleWhere(t *testing.T) {
	spec := NewQuery().
		Where("age", Gte, 18).
		OrderBy("created_at", Desc).
		Limit(10).
		Spec()

	if spec.Where == nil {
		t.Fatal("expected where node")
	}
	if spec.Where.Field != "age" || spec.Where.Op != Gte {
		t.Fatalf("unexpected where leaf: %+v", spec.Where)
	}
	if len(spec.OrderBy) != 1 || spec.OrderBy[0].Field != "created_at" {
		t.Fatalf("unexpected order_by: %+v", spec.OrderBy)
	}
	if spec.Limit == nil || *spec.Limit != 10 {
		t.Fatalf("unexpected limit: %+v", spec.Limit)
	}
}

func TestQuerySpecMultipleWhereUsesAND(t *testing.T) {
	spec := NewQuery().
		Where("active", Eq, true).
		Where("age", Gte, 18).
		Spec()

	if spec.Where == nil || len(spec.Where.AND) != 2 {
		t.Fatalf("expected AND node, got %+v", spec.Where)
	}
}

func TestQueryJSON(t *testing.T) {
	data, err := NewQuery().
		Where("active", Eq, true).
		Limit(5).
		JSON()
	if err != nil {
		t.Fatalf("JSON() returned error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("query JSON is invalid: %v", err)
	}
	if _, ok := parsed["where"]; !ok {
		t.Fatalf("expected where in JSON: %s", string(data))
	}
}
