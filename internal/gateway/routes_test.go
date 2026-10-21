package gateway

import (
	"context"
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
)

func TestCanonicalMessageRouteStreamsAndPersists(t *testing.T) {
	service := New(engine.NewAgentLoop(adapters.MockAdapter{Response: "gateway-ok"}, nil), nil)
	response := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"content": "Hello"}, map[string]string{
		"x-actor-id": "actor-1", "x-actor-permissions": "memory:read,memory:write",
	})
	if response.Status != 200 || response.Headers["X-Conversation-ID"] == "" || len(response.Stream) < 2 {
		t.Fatalf("unexpected response: %#v", response)
	}
	detail := service.Handle(context.Background(), "GET", "/conversations/"+response.Headers["X-Conversation-ID"], nil, nil)
	if len(detail.Body["messages"].([]map[string]any)) != 2 {
		t.Fatalf("expected persisted user and assistant messages: %#v", detail.Body)
	}
}

func TestPublicRequestRejectsProviderShapedMessages(t *testing.T) {
	service := New(engine.NewAgentLoop(adapters.MockAdapter{}, nil), nil)
	response := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"messages": []any{}}, map[string]string{"X-Actor-ID": "actor", "X-Actor-Permissions": "read"})
	if response.Status != 400 {
		t.Fatalf("expected invalid request: %#v", response)
	}
}
