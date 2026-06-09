package config

import (
	"encoding/json"
	"testing"
)

func TestJSONSchemaWellFormedAndCoversTopLevel(t *testing.T) {
	b, err := JSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	if doc["$schema"] == nil || doc["type"] != "object" {
		t.Fatalf("schema missing $schema/type: %v", doc)
	}
	props, ok := doc["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties object")
	}
	// Every top-level yaml key on File must appear (generated from the struct).
	for _, k := range []string{"version", "server", "store", "engine", "auth", "rules", "platforms", "recipes"} {
		if props[k] == nil {
			t.Fatalf("schema missing top-level property %q", k)
		}
	}
	// A nested field should be reachable too.
	store, _ := props["store"].(map[string]any)
	sp, _ := store["properties"].(map[string]any)
	if sp["redis_addr"] == nil {
		t.Fatalf("schema missing nested store.redis_addr: %v", store)
	}
}
