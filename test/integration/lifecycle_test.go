package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/gateway"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/memory"
)

func TestGatewayHTTPAndMemoryLifecycle(t *testing.T) {
	service := gateway.New(engine.NewAgentLoop(adapters.MockAdapter{Response: "http-ok"}, nil), memory.NewStore("/tmp/paa-go-test-memory"))
	server := httptest.NewServer(gateway.Handler(service, "test-token"))
	defer server.Close()

	health, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		t.Fatalf("unexpected health status: %d", health.StatusCode)
	}

	body, _ := json.Marshal(map[string]any{"content": []map[string]any{{"type": "text", "text": "Hello"}}})
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/conversations/messages", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", "test-token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected stream response: %d %s", response.StatusCode, response.Header.Get("Content-Type"))
	}
	conversationID := response.Header.Get("X-Conversation-ID")
	if conversationID == "" {
		t.Fatal("missing conversation ID")
	}
	list := service.Handle(context.Background(), "GET", "/conversations", nil, nil)
	if list.Body["total"] != 1 {
		t.Fatalf("unexpected conversation list: %#v", list.Body)
	}
	if len(service.Memory.Export()["history"].([]memory.Entry)) < 2 {
		t.Fatal("expected persisted message history")
	}
}
