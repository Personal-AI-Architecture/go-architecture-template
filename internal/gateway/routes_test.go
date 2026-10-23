package gateway

import (
	"context"
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

type recordingAdapter struct{ requests []types.CompletionRequest }

func (a *recordingAdapter) Name() string { return "recording" }
func (a *recordingAdapter) Stream(_ context.Context, request types.CompletionRequest) ([]types.ModelStreamChunk, error) {
	a.requests = append(a.requests, request)
	return []types.ModelStreamChunk{{Type: "text-delta", Delta: map[string]any{"content": "answer"}}, {Type: "done", Delta: map[string]any{"finish_reason": "stop"}}}, nil
}

func TestCanonicalMessageRouteStreamsAndPersists(t *testing.T) {
	service := New(engine.NewAgentLoop(adapters.MockAdapter{Response: "gateway-ok"}, nil), nil)
	response := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"content": []map[string]any{{"type": "text", "text": "Hello"}}}, map[string]string{
		"x-actor-id": "actor-1", "x-actor-permissions": "memory:read,memory:write",
	})
	if response.Status != 200 || response.Headers["X-Conversation-ID"] == "" || len(response.Stream) < 2 {
		t.Fatalf("unexpected response: %#v", response)
	}
	detail := service.Handle(context.Background(), "GET", "/conversations/"+response.Headers["X-Conversation-ID"], nil, nil)
	if len(detail.Body["messages"].([]map[string]any)) != 2 {
		t.Fatalf("expected persisted user and assistant messages: %#v", detail.Body)
	}
	messages := detail.Body["messages"].([]map[string]any)
	if _, ok := messages[0]["content"].([]map[string]any); !ok {
		t.Fatalf("user content did not retain canonical parts: %#v", messages[0])
	}
	if _, ok := messages[1]["content"].([]map[string]any); !ok {
		t.Fatalf("assistant content did not use canonical parts: %#v", messages[1])
	}
}

func TestOldStringContentIsRejected(t *testing.T) {
	service := New(engine.NewAgentLoop(adapters.MockAdapter{}, nil), nil)
	response := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"content": "Hello"}, map[string]string{"X-Actor-ID": "actor", "X-Actor-Permissions": "read"})
	if response.Status != 400 {
		t.Fatalf("expected invalid request: %#v", response)
	}
}

func TestPublicRequestRejectsProviderShapedMessages(t *testing.T) {
	service := New(engine.NewAgentLoop(adapters.MockAdapter{}, nil), nil)
	response := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"messages": []any{}}, map[string]string{"X-Actor-ID": "actor", "X-Actor-Permissions": "read"})
	if response.Status != 400 {
		t.Fatalf("expected invalid request: %#v", response)
	}
}

func TestProviderSwitchAndSavedReplyProvenance(t *testing.T) {
	adapter := &recordingAdapter{}
	loop := engine.NewAgentLoop(adapter, nil)
	loop.ContextLimits = types.ContextLimits{"alpha": {"m1": 8192}, "beta": {"m2": 8192}}
	service := New(loop, nil)
	provider, model := "alpha", "m1"
	service.CurrentProvider, service.CurrentModel = func() string { return provider }, func() string { return model }
	headers := map[string]string{"X-Actor-ID": "owner", "X-Actor-Permissions": "read"}
	body := map[string]any{"content": []map[string]any{{"type": "text", "text": "hello"}}}
	first := service.Handle(context.Background(), "POST", "/conversations/messages", body, headers)
	provider, model = "beta", "m2"
	second := service.Handle(context.Background(), "POST", "/conversations/"+first.Headers["X-Conversation-ID"]+"/messages", body, headers)
	if first.Status != 200 || second.Status != 200 || len(adapter.requests) != 2 || adapter.requests[0].Provider != "alpha" || adapter.requests[1].Provider != "beta" || adapter.requests[1].Model != "m2" {
		t.Fatalf("provider switch did not reach model: %#v", adapter.requests)
	}
	detail := service.Handle(context.Background(), "GET", "/conversations/"+first.Headers["X-Conversation-ID"], nil, nil)
	messages := detail.Body["messages"].([]map[string]any)
	if messages[1]["provider"] != "alpha" || messages[1]["model"] != "m1" || messages[3]["provider"] != "beta" || messages[3]["model"] != "m2" {
		t.Fatalf("saved provenance missing: %#v", messages)
	}
}

func TestContextLimitBlocksOrOmitsWithoutChangingHistory(t *testing.T) {
	adapter := &recordingAdapter{}
	loop := engine.NewAgentLoop(adapter, nil)
	service := New(loop, nil)
	headers := map[string]string{"X-Actor-ID": "owner", "X-Actor-Permissions": "read"}
	body := map[string]any{"content": []map[string]any{{"type": "text", "text": "hello"}}}
	loop.ContextLimits = types.ContextLimits{}
	unknown := service.Handle(context.Background(), "POST", "/conversations/messages", body, headers)
	if unknown.Status != 503 || unknown.Body["code"] != "context_limit_unknown" {
		t.Fatalf("expected unknown limit: %#v", unknown)
	}
	loop.ContextLimits = types.ContextLimits{"local": {"mock": 500}}
	overflow := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"content": []map[string]any{{"type": "text", "text": "a very large current message that is impossible to fit with the reserved overhead"}}}, headers)
	if overflow.Status != 422 || overflow.Body["code"] != "context_overflow" || len(adapter.requests) != 0 {
		t.Fatalf("expected pre-dispatch overflow: %#v", overflow)
	}
	loop.ContextLimits = types.ContextLimits{"local": {"mock": 550}}
	first := service.Handle(context.Background(), "POST", "/conversations/messages", body, headers)
	second := service.Handle(context.Background(), "POST", "/conversations/"+first.Headers["X-Conversation-ID"]+"/messages", body, headers)
	if first.Status != 200 || second.Status != 200 || len(second.Stream) == 0 || second.Stream[0].Event != "context_notice" {
		t.Fatalf("missing context notice: %#v", second)
	}
	detail := service.Handle(context.Background(), "GET", "/conversations/"+first.Headers["X-Conversation-ID"], nil, nil)
	if len(detail.Body["messages"].([]map[string]any)) != 4 {
		t.Fatalf("full history was lost: %#v", detail.Body)
	}
}
