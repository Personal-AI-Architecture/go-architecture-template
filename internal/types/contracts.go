// Package types contains the canonical PAA-facing data contracts.
package types

import "context"

type ToolCall struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Arguments        map[string]any `json:"arguments"`
	InvalidArguments bool           `json:"invalid_arguments,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    any        `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolDefinition struct {
	Name                string         `json:"name"`
	Description         string         `json:"description"`
	Parameters          map[string]any `json:"parameters"`
	Source              string         `json:"source,omitempty"`
	Category            string         `json:"category,omitempty"`
	MutatesState        bool           `json:"mutates_state,omitempty"`
	RequiredPermissions []string       `json:"required_permissions,omitempty"`
}

type EngineRequestMetadata struct {
	CorrelationID      string         `json:"correlation_id"`
	ConversationID     string         `json:"conversation_id,omitempty"`
	Trigger            string         `json:"trigger,omitempty"`
	ClientContext      map[string]any `json:"client_context,omitempty"`
	ActorID            string         `json:"actor_id,omitempty"`
	ActorPermissions   []string       `json:"actor_permissions,omitempty"`
	AllowedToolSources []string       `json:"allowed_tool_sources,omitempty"`
}

type EngineRequest struct {
	Messages    []Message             `json:"messages"`
	Metadata    EngineRequestMetadata `json:"metadata"`
	Model       string                `json:"model"`
	AuthContext map[string]any        `json:"auth_context"`
}

type CompletionRequest struct {
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Stream      bool             `json:"stream"`
	Model       string           `json:"model,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

type ModelStreamChunk struct {
	Type  string         `json:"type"`
	Delta map[string]any `json:"delta,omitempty"`
}

type StreamEvent struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data,omitempty"`
}

type ModelAdapter interface {
	Name() string
	Stream(context.Context, CompletionRequest) ([]ModelStreamChunk, error)
}
