package engine

import (
	"context"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

type AgentLoop struct {
	Adapter types.ModelAdapter
	Tools   []types.ToolDefinition
}

func NewAgentLoop(adapter types.ModelAdapter, tools []types.ToolDefinition) *AgentLoop {
	return &AgentLoop{Adapter: adapter, Tools: append([]types.ToolDefinition(nil), tools...)}
}

func (a *AgentLoop) Run(ctx context.Context, request types.EngineRequest, actorHeaders map[string]string) []types.StreamEvent {
	if request.Metadata.CorrelationID == "" {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "provider_error", "message": "Missing correlation_id."}}}
	}
	actor, ok := request.AuthContext["actor"].(map[string]any)
	if !ok || actor["id"] == "" || request.AuthContext["correlation_id"] != request.Metadata.CorrelationID {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "unauthorized", "message": "Missing authenticated actor context."}}}
	}
	if request.Model == "" {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "invalid_request", "message": "Missing selected model."}}}
	}
	chunks, err := a.Adapter.Stream(ctx, types.CompletionRequest{
		Messages: request.Messages,
		Tools:    append([]types.ToolDefinition(nil), a.Tools...),
		Stream:   true,
		Model:    request.Model,
	})
	if err != nil {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "provider_error", "message": err.Error()}}}
	}
	events := make([]types.StreamEvent, 0, len(chunks))
	for _, chunk := range chunks {
		switch chunk.Type {
		case "text-delta":
			if content, ok := chunk.Delta["content"].(string); ok && content != "" {
				events = append(events, types.StreamEvent{Event: "text-delta", Data: map[string]any{"content": content}})
			}
		case "tool-call":
			events = append(events, types.StreamEvent{Event: "tool-call", Data: chunk.Delta})
		case "tool-result":
			events = append(events, types.StreamEvent{Event: "tool-result", Data: chunk.Delta})
		case "error":
			events = append(events, types.StreamEvent{Event: "error", Data: chunk.Delta})
		case "done":
			finishReason := "stop"
			if value, ok := chunk.Delta["finish_reason"].(string); ok && value != "" {
				finishReason = value
			}
			events = append(events, types.StreamEvent{Event: "done", Data: map[string]any{"finish_reason": finishReason}})
		}
	}
	return events
}

func header(headers map[string]string, name string) string {
	for key, value := range headers {
		if equalFold(key, name) {
			return value
		}
	}
	return ""
}

func equalFold(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		l, r := left[index], right[index]
		if l >= 'A' && l <= 'Z' {
			l += 'a' - 'A'
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if l != r {
			return false
		}
	}
	return true
}
