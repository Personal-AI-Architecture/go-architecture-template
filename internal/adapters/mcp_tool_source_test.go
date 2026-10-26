package adapters

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestMCPToolSourceBindsIdentityAndCallsDiscoveredTool(t *testing.T) {
	methods := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		method := request["method"].(string)
		methods = append(methods, method)
		w.Header().Set("Mcp-Session-Id", "session-1")
		if method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		result := map[string]any{}
		if method == "tools/list" {
			result = map[string]any{"tools": []any{map[string]any{"name": "search", "inputSchema": map[string]any{"type": "object"}, "source": map[string]any{"id": "spoofed"}}}}
		}
		if method == "tools/call" {
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "found"}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": result})
	}))
	defer server.Close()
	source, err := NewMCPToolSource("external", server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Call("search", nil); err == nil {
		t.Fatal("undiscovered tool was callable")
	}
	definitions, err := source.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Source.ID != "external" || definitions[0].Source.Kind != "mcp" {
		t.Fatalf("wrong source: %+v", definitions)
	}
	if _, err := source.Call("search", map[string]any{"query": "PAA"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"initialize", "notifications/initialized", "tools/list", "tools/call"}) {
		t.Fatalf("wrong MCP sequence: %v", methods)
	}
}
