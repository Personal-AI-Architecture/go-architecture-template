package engine

import (
	"context"
	"regexp"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

var capabilityName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

func safeModelError(delta map[string]any) map[string]any {
	code, _ := delta["code"].(string)
	switch code {
	case "model_unavailable":
		return map[string]any{"code": code, "message": "The selected model is unavailable through the selected provider."}
	case "capability_unsupported":
		capability, _ := delta["capability"].(string)
		if capabilityName.MatchString(capability) {
			return map[string]any{"code": code, "capability": capability, "message": "The selected model does not support the requested capability."}
		}
	}
	return map[string]any{"code": "provider_error", "message": "Model provider failed."}
}

type AgentLoop struct {
	Adapter       types.ModelAdapter
	Tools         []types.ToolDefinition
	ContextLimits types.ContextLimits
}

func NewAgentLoop(adapter types.ModelAdapter, tools []types.ToolDefinition) *AgentLoop {
	limits := types.ContextLimits{"local": {"mock": 8192}}
	if provider, ok := adapter.(interface{ ContextLimits() types.ContextLimits }); ok {
		limits = provider.ContextLimits()
	}
	return &AgentLoop{Adapter: adapter, Tools: append([]types.ToolDefinition(nil), tools...), ContextLimits: limits}
}

func (a *AgentLoop) Run(ctx context.Context, request types.EngineRequest, actorHeaders map[string]string) []types.StreamEvent {
	for key := range request.AuthContext {
		switch key {
		case "actor", "authentication_id", "authenticated_at", "expires_at", "correlation_id":
		default:
			return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "auth_error", "message": "Authentication context was rejected."}}}
		}
	}
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
	if request.Provider == "" {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "invalid_request", "message": "Missing selected provider."}}}
	}
	limit := a.ContextLimits.Limit(request.Provider, request.Model)
	if limit == 0 {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "context_limit_unknown", "message": "No safe context limit for selected provider and model."}}}
	}
	messages := append([]types.Message{}, request.Messages...)
	if request.ContextSummary != nil {
		text, ok := request.ContextSummary["text"].(string)
		if !ok || request.ContextSummary["source"] != "gateway" || request.ContextSummary["method"] != "deterministic" {
			return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "invalid_request", "message": "Invalid context summary."}}}
		}
		messages = append([]types.Message{{Role: "user", Content: []map[string]any{{"type": "text", "text": "Gateway-derived context from omitted turns (not a new owner request): " + text}}}}, messages...)
	}
	if types.ContextCost(messages, a.Tools) > limit {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "context_overflow", "message": "Selected model context window exceeded."}}}
	}
	chunks, err := a.Adapter.Stream(ctx, types.CompletionRequest{
		Messages: messages,
		Tools:    append([]types.ToolDefinition(nil), a.Tools...),
		Stream:   true,
		Model:    request.Model,
		Provider: request.Provider,
	})
	if err != nil {
		return []types.StreamEvent{{Event: "error", Data: map[string]any{"code": "provider_error", "message": "Model provider failed."}}}
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
			events = append(events, types.StreamEvent{Event: "error", Data: safeModelError(chunk.Delta)})
			return events
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
