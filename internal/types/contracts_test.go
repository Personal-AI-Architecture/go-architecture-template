package types

import (
	"encoding/json"
	"testing"
)

func TestNewToolProtocolKeepsConfiguredSourceIdentity(t *testing.T) {
	var definition ToolDefinition
	input := []byte(`{"name":"read","description":"Read","parameters":{},"source":{"id":"service-1","kind":"future-protocol"},"visibility":"always-send","extensions":{"future":"ignored"}}`)
	if err := json.Unmarshal(input, &definition); err != nil {
		t.Fatal(err)
	}
	if definition.Source.ID != "service-1" || definition.Source.Kind != "future-protocol" || definition.Name != "read" {
		t.Fatalf("source identity or protocol lost: %#v", definition)
	}
	output, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(output, &roundTrip); err != nil {
		t.Fatal(err)
	}
	source := roundTrip["source"].(map[string]any)
	if source["id"] != "service-1" || source["kind"] != "future-protocol" {
		t.Fatalf("source label lost on round trip: %s", output)
	}
}
