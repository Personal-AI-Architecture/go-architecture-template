package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/memory"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

type StoredMessage struct {
	ID        string
	Message   types.Message
	CreatedAt string
}

type Conversation struct {
	ID        string
	CreatedAt string
	UpdatedAt string
	Messages  []StoredMessage
}

type Response struct {
	Status  int
	Headers map[string]string
	Body    map[string]any
	Stream  []types.StreamEvent
}

type Gateway struct {
	Engine        *engine.AgentLoop
	Memory        *memory.Store
	CurrentModel  func() string
	mu            sync.Mutex
	conversations map[string]*Conversation
}

func New(engineLoop *engine.AgentLoop, memoryStore *memory.Store) *Gateway {
	if memoryStore == nil {
		memoryStore = memory.NewStore(".paa-memory")
	}
	return &Gateway{Engine: engineLoop, Memory: memoryStore, CurrentModel: func() string { return "mock" }, conversations: map[string]*Conversation{}}
}

func (g *Gateway) Handle(ctx context.Context, method, path string, body map[string]any, headers map[string]string) Response {
	parsed, err := url.Parse(path)
	if err != nil {
		return errorResponse(400, "invalid_request", "Invalid route.")
	}
	method = strings.ToUpper(method)
	route := parsed.Path
	if method == "GET" && route == "/conversations" {
		return g.listConversations()
	}
	if method == "GET" && strings.HasPrefix(route, "/conversations/") {
		return g.getConversation(strings.TrimPrefix(route, "/conversations/"))
	}
	if method == "POST" && (route == "/conversations/messages" || route == "/chat") {
		return g.message(ctx, body, headers, "")
	}
	if method == "POST" && strings.HasPrefix(route, "/conversations/") && strings.HasSuffix(route, "/messages") {
		id := strings.TrimSuffix(strings.TrimPrefix(route, "/conversations/"), "/messages")
		return g.message(ctx, body, headers, id)
	}
	return errorResponse(404, "not_found", "Route not found.")
}

func (g *Gateway) message(ctx context.Context, body map[string]any, headers map[string]string, requestedID string) Response {
	content, metadata, ok := parsePublicRequest(body)
	if !ok {
		return errorResponse(400, "invalid_request", "Request must contain non-empty content and optional metadata.")
	}
	actorID := header(headers, "X-Actor-ID")
	actorPermissions := header(headers, "X-Actor-Permissions")
	if actorID == "" || actorPermissions == "" {
		return errorResponse(401, "missing_actor_headers", "Missing required actor headers.")
	}

	g.mu.Lock()
	conversation := g.conversations[requestedID]
	if requestedID != "" && conversation == nil {
		g.mu.Unlock()
		return errorResponse(404, "conversation_not_found", fmt.Sprintf("Conversation %s was not found.", requestedID))
	}
	if conversation == nil {
		now := timestamp()
		conversation = &Conversation{ID: newID("conv"), CreatedAt: now, UpdatedAt: now}
		g.conversations[conversation.ID] = conversation
		g.Memory.Record(memory.Entry{Kind: "conversation", ID: conversation.ID, Metadata: metadata})
	}
	user := types.Message{Role: "user", Content: content}
	g.appendMessageLocked(conversation, user)
	messages := make([]types.Message, 0, len(conversation.Messages))
	for _, stored := range conversation.Messages {
		messages = append(messages, stored.Message)
	}
	conversationID := conversation.ID
	g.mu.Unlock()

	correlationID := newID("corr")
	if value, ok := metadata["correlation_id"].(string); ok && strings.TrimSpace(value) != "" {
		correlationID = strings.TrimSpace(value)
	}
	events := g.Engine.Run(ctx, types.EngineRequest{
		Messages: messages,
		Metadata: types.EngineRequestMetadata{
			CorrelationID:  correlationID,
			ConversationID: conversationID,
			ClientContext:  metadata,
		},
		Model: g.CurrentModel(),
		AuthContext: map[string]any{
			"actor":             map[string]any{"id": actorID, "type": "owner", "display_name": actorID, "status": "active", "created": timestamp()},
			"authentication_id": newID("authn"), "authenticated_at": timestamp(), "correlation_id": correlationID,
		},
	}, headers)

	textParts := make([]string, 0)
	for index := range events {
		if events[index].Event == "text-delta" {
			if text, ok := events[index].Data["content"].(string); ok {
				textParts = append(textParts, text)
			}
		}
		events[index].Data["_event_type"] = events[index].Event
		if events[index].Event == "done" {
			if _, ok := events[index].Data["message_id"]; !ok {
				events[index].Data["message_id"] = newID("msg")
			}
			if _, ok := events[index].Data["finish_reason"]; !ok {
				events[index].Data["finish_reason"] = "stop"
			}
		}
	}
	if len(textParts) > 0 {
		g.mu.Lock()
		g.appendMessageLocked(conversation, types.Message{Role: "assistant", Content: []map[string]any{{"type": "text", "text": strings.Join(textParts, "")}}})
		g.mu.Unlock()
	}
	return Response{
		Status:  200,
		Headers: map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-cache", "Connection": "keep-alive", "X-Conversation-ID": conversationID},
		Stream:  events,
	}
}

func (g *Gateway) appendMessageLocked(conversation *Conversation, message types.Message) {
	now := timestamp()
	messageID := newID("msg")
	conversation.Messages = append(conversation.Messages, StoredMessage{ID: messageID, Message: message, CreatedAt: now})
	conversation.UpdatedAt = now
	g.Memory.Record(memory.Entry{Kind: "message", ID: messageID, ConversationID: conversation.ID, Role: message.Role, Content: message.Content})
}

func (g *Gateway) listConversations() Response {
	g.mu.Lock()
	defer g.mu.Unlock()
	items := make([]map[string]any, 0, len(g.conversations))
	for _, conversation := range g.conversations {
		items = append(items, map[string]any{"id": conversation.ID, "created_at": conversation.CreatedAt, "updated_at": conversation.UpdatedAt, "message_count": len(conversation.Messages)})
	}
	return Response{Status: 200, Headers: jsonHeaders(), Body: map[string]any{"conversations": items, "total": len(items)}}
}

func (g *Gateway) getConversation(id string) Response {
	g.mu.Lock()
	defer g.mu.Unlock()
	conversation := g.conversations[id]
	if conversation == nil {
		return errorResponse(404, "conversation_not_found", fmt.Sprintf("Conversation %s was not found.", id))
	}
	messages := make([]map[string]any, 0, len(conversation.Messages))
	for _, stored := range conversation.Messages {
		messages = append(messages, map[string]any{"id": stored.ID, "role": stored.Message.Role, "content": stored.Message.Content, "created_at": stored.CreatedAt})
	}
	return Response{Status: 200, Headers: jsonHeaders(), Body: map[string]any{"id": conversation.ID, "created_at": conversation.CreatedAt, "updated_at": conversation.UpdatedAt, "messages": messages}}
}

func parsePublicRequest(body map[string]any) ([]map[string]any, map[string]any, bool) {
	if body == nil {
		return nil, nil, false
	}
	if _, exists := body["messages"]; exists {
		return nil, nil, false
	}
	for key := range body {
		if key != "content" && key != "metadata" {
			return nil, nil, false
		}
	}
	var rawParts []any
	switch value := body["content"].(type) {
	case []any:
		rawParts = value
	case []map[string]any:
		for _, part := range value {
			rawParts = append(rawParts, part)
		}
	default:
		return nil, nil, false
	}
	if len(rawParts) == 0 {
		return nil, nil, false
	}
	content := make([]map[string]any, 0, len(rawParts))
	for _, item := range rawParts {
		part, ok := item.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		switch part["type"] {
		case "text":
			if _, ok := part["text"].(string); !ok {
				return nil, nil, false
			}
		case "image", "file":
			if _, ok := part["url"].(string); !ok {
				return nil, nil, false
			}
		default:
			return nil, nil, false
		}
		content = append(content, part)
	}
	metadata := map[string]any{}
	if raw, exists := body["metadata"]; exists {
		var ok bool
		metadata, ok = raw.(map[string]any)
		if !ok {
			return nil, nil, false
		}
	}
	return content, metadata, true
}

func errorResponse(status int, code, message string) Response {
	return Response{Status: status, Headers: jsonHeaders(), Body: map[string]any{"code": code, "message": message}}
}

func jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json"}
}

func splitPermissions(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func header(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func timestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func newID(prefix string) string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%s_fallback", prefix)
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}
