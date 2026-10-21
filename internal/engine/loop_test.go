package engine

import (
	"context"
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

func TestAgentLoopEmitsCanonicalEvents(t *testing.T) {
	loop := NewAgentLoop(adapters.MockAdapter{Response: "hello"}, nil)
	events := loop.Run(context.Background(), types.EngineRequest{
		Messages: []types.Message{{Role: "user", Content: "Hi"}},
		Metadata: types.EngineRequestMetadata{CorrelationID: "corr-1"},
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
