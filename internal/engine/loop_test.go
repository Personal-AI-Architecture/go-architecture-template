package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

type recordingModelAdapter struct{ requests []types.CompletionRequest }

func (a *recordingModelAdapter) Name() string { return "recording" }
func (a *recordingModelAdapter) Stream(_ context.Context, request types.CompletionRequest) ([]types.ModelStreamChunk, error) {
	a.requests = append(a.requests, request)
	return []types.ModelStreamChunk{{Type: "done", Delta: map[string]any{"finish_reason": "stop"}}}, nil
}

func TestAgentLoopEmitsCanonicalEvents(t *testing.T) {
	loop := NewAgentLoop(adapters.MockAdapter{Response: "hello"}, nil)
	events := loop.Run(context.Background(), types.EngineRequest{
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
		Metadata: types.EngineRequestMetadata{CorrelationID: "corr-1"},
		Model:    "mock", Provider: "local", AuthContext: map[string]any{"actor": map[string]any{"id": "actor-1"}, "correlation_id": "corr-1"},
	}, map[string]string{"X-Actor-ID": "actor-1", "X-Actor-Permissions": "memory:read"})
	if len(events) != 2 || events[0].Event != "text-delta" || events[1].Event != "done" {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestAgentLoopRequiresActorHeaders(t *testing.T) {
	loop := NewAgentLoop(adapters.MockAdapter{}, nil)
	events := loop.Run(context.Background(), types.EngineRequest{Metadata: types.EngineRequestMetadata{CorrelationID: "corr-1"}}, nil)
	if len(events) != 1 || events[0].Event != "error" {
		t.Fatalf("expected actor header error: %#v", events)
	}
}

func TestDerivedSummaryIsLabelledAndEphemeral(t *testing.T) {
	adapter := &recordingModelAdapter{}
	loop := NewAgentLoop(adapter, nil)
	original := []types.Message{{Role: "user", Content: []map[string]any{{"type": "text", "text": "current"}}}}
	request := types.EngineRequest{Messages: original, Metadata: types.EngineRequestMetadata{CorrelationID: "corr"}, Model: "mock", Provider: "local", AuthContext: map[string]any{"actor": map[string]any{"id": "owner"}, "correlation_id": "corr"}, ContextSummary: map[string]any{"source": "gateway", "method": "deterministic", "omitted_turn_count": 1, "text": "older fact"}}
	loop.Run(context.Background(), request, nil)
	if len(original) != 1 || len(adapter.requests) != 1 || len(adapter.requests[0].Messages) != 2 {
		t.Fatalf("summary mutated saved messages or missed model: %#v", adapter.requests)
	}
	content := adapter.requests[0].Messages[0].Content.([]map[string]any)
	if !strings.Contains(content[0]["text"].(string), "Gateway-derived context") {
		t.Fatalf("derived summary was not labelled: %#v", content)
	}
}
