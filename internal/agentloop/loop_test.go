package agentloop

import (
	"context"
	"strings"
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

type recordingModelAdapter struct {
	requests []types.CompletionRequest
	chunks   []types.ModelStreamChunk
}

func (a *recordingModelAdapter) Name() string { return "recording" }
func (a *recordingModelAdapter) Stream(_ context.Context, request types.CompletionRequest) ([]types.ModelStreamChunk, error) {
	a.requests = append(a.requests, request)
	if len(a.chunks) > 0 {
		return a.chunks, nil
	}
	return []types.ModelStreamChunk{{Type: "done", Delta: map[string]any{"finish_reason": "stop"}}}, nil
}

func TestModelErrorCategoriesAreSafeAndStopTheTurn(t *testing.T) {
	for _, scenario := range []struct{ code, capability string }{{"model_unavailable", ""}, {"capability_unsupported", "image_input"}} {
		adapter := &recordingModelAdapter{chunks: []types.ModelStreamChunk{{Type: "error", Delta: map[string]any{"code": scenario.code, "capability": scenario.capability, "message": "secret=/tmp/key"}}}}
		loop := NewAgentLoop(adapter, nil)
		events := loop.Run(context.Background(), types.AgentLoopRequest{Messages: []types.Message{{Role: "user", Content: "hello"}}, Metadata: types.AgentLoopRequestMetadata{CorrelationID: "corr"}, Model: "mock", Provider: "local", AuthContext: map[string]any{"actor": map[string]any{"id": "owner"}, "correlation_id": "corr"}}, nil)
		if len(events) != 1 || events[0].Event != "error" || events[0].Data["code"] != scenario.code || len(adapter.requests) != 1 || adapter.requests[0].Model != "mock" || adapter.requests[0].Provider != "local" || strings.Contains(events[0].Data["message"].(string), "secret") {
			t.Fatalf("model failure was lost or leaked: %#v", events)
		}
		if scenario.capability != "" && events[0].Data["capability"] != scenario.capability {
			t.Fatalf("missing capability: %#v", events)
		}
	}
}

func TestAuthContextRejectsUnknownTopLevelField(t *testing.T) {
	loop := NewAgentLoop(adapters.MockAdapter{}, nil)
	events := loop.Run(context.Background(), types.AgentLoopRequest{Metadata: types.AgentLoopRequestMetadata{CorrelationID: "corr"}, Model: "mock", Provider: "local", AuthContext: map[string]any{"actor": map[string]any{"id": "owner"}, "correlation_id": "corr", "unexpected": "spoof"}}, nil)
	if len(events) != 1 || events[0].Data["code"] != "auth_error" {
		t.Fatalf("unknown auth field accepted: %#v", events)
	}
}

func TestAgentLoopEmitsCanonicalEvents(t *testing.T) {
	loop := NewAgentLoop(adapters.MockAdapter{Response: "hello"}, nil)
	events := loop.Run(context.Background(), types.AgentLoopRequest{
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
		Metadata: types.AgentLoopRequestMetadata{CorrelationID: "corr-1"},
		Model:    "mock", Provider: "local", AuthContext: map[string]any{"actor": map[string]any{"id": "actor-1"}, "correlation_id": "corr-1"},
	}, map[string]string{"X-Actor-ID": "actor-1", "X-Actor-Permissions": "memory:read"})
	if len(events) != 2 || events[0].Event != "text-delta" || events[1].Event != "done" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestAgentLoopRequiresActorHeaders(t *testing.T) {
	loop := NewAgentLoop(adapters.MockAdapter{}, nil)
	events := loop.Run(context.Background(), types.AgentLoopRequest{Metadata: types.AgentLoopRequestMetadata{CorrelationID: "corr-1"}}, nil)
	if len(events) != 1 || events[0].Event != "error" {
		t.Fatalf("expected actor header error: %#v", events)
	}
}

func TestDerivedSummaryIsLabelledAndEphemeral(t *testing.T) {
	adapter := &recordingModelAdapter{}
	loop := NewAgentLoop(adapter, nil)
	original := []types.Message{{Role: "user", Content: []map[string]any{{"type": "text", "text": "current"}}}}
	request := types.AgentLoopRequest{Messages: original, Metadata: types.AgentLoopRequestMetadata{CorrelationID: "corr"}, Model: "mock", Provider: "local", AuthContext: map[string]any{"actor": map[string]any{"id": "owner"}, "correlation_id": "corr"}, ContextSummary: map[string]any{"source": "gateway", "method": "deterministic", "omitted_turn_count": 1, "text": "older fact"}}
	loop.Run(context.Background(), request, nil)
	if len(original) != 1 || len(adapter.requests) != 1 || len(adapter.requests[0].Messages) != 2 {
		t.Fatalf("summary mutated saved messages or missed model: %#v", adapter.requests)
	}
	content := adapter.requests[0].Messages[0].Content.([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "Gateway-derived context") {
		t.Fatalf("derived summary was not labelled: %#v", content)
	}
}
