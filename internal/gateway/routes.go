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
	Model     string
	Provider  string
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
	Engine          *engine.AgentLoop
	Memory          *memory.Store
	CurrentModel    func() string
	CurrentProvider func() string
	mu              sync.Mutex
	conversations   map[string]*Conversation
}

func New(engineLoop *engine.AgentLoop, memoryStore *memory.Store) *Gateway {
	if memoryStore == nil {
		memoryStore = memory.NewStore(".paa-memory")
	}
	return &Gateway{Engine: engineLoop, Memory: memoryStore, CurrentModel: func() string { return "mock" }, CurrentProvider: func() string { return "local" }, conversations: map[string]*Conversation{}}
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
	model, provider := g.CurrentModel(), g.CurrentProvider()
	limit := g.Engine.ContextLimits.Limit(provider, model)
	if limit == 0 {
		return errorResponse(503, "context_limit_unknown", "No safe context limit for selected provider and model.")
	}
	selected, omitted, fits := types.SelectCompleteTurns(messages, g.Engine.Tools, limit)
	if !fits {
		return errorResponse(422, "context_overflow", "Current message exceeds the selected model context window.")
	}

	correlationID := newID("corr")
	if value, ok := metadata["correlation_id"].(string); ok && strings.TrimSpace(value) != "" {
		correlationID = strings.TrimSpace(value)
	}
	events := g.Engine.Run(ctx, types.EngineRequest{
		Messages: selected,
		Metadata: types.EngineRequestMetadata{
			CorrelationID:  correlationID,
			ConversationID: conversationID,
			ClientContext:  metadata,
		},
		Model:    model,
		Provider: provider,
		AuthContext: map[string]any{
			"actor":             map[string]any{"id": actorID, "type": "owner", "display_name": actorID, "status": "active", "created": timestamp()},
			"authentication_id": newID("authn"), "authenticated_at": timestamp(), "correlation_id": correlationID,
		},
	}, headers)
	if omitted > 0 {
		events = append([]types.StreamEvent{{Event: "context_notice", Data: map[string]any{"_event_type": "context_notice", "action": "omitted", "omitted_turn_count": omitted, "message": "Older conversation turns were omitted from this reply's working context."}}}, events...)
	}

	textParts := make([]string, 0)
	completed := false
	for index := range events {
		if events[index].Event == "text-delta" {
			if text, ok := events[index].Data["content"].(string); ok {
				textParts = append(textParts, text)
			}
		}
		events[index].Data["_event_type"] = events[index].Event
		if events[index].Event == "done" {
			completed = true
			if _, ok := events[index].Data["message_id"]; !ok {
				events[index].Data["message_id"] = newID("msg")
			}
			if _, ok := events[index].Data["finish_reason"]; !ok {
				events[index].Data["finish_reason"] = "stop"
			}
		}
	}
	if completed && len(textParts) > 0 {
		g.mu.Lock()
		g.appendMessageLocked(conversation, types.Message{Role: "assistant", Content: []map[string]any{{"type": "text", "text": strings.Join(textParts, "")}}}, model, provider)
		g.mu.Unlock()
	}
	return Response{
		Status:  200,
		Headers: map[string]string{"Content-Type": "text/event-stream", "Cache-Control": "no-cache", "Connection": "keep-alive", "X-Conversation-ID": conversationID},
		Stream:  events,
	}
}

func (g *Gateway) appendMessageLocked(conversation *Conversation, message types.Message, provenance ...string) {
	now := timestamp()
	messageID := newID("msg")
	stored := StoredMessage{ID: messageID, Message: message, CreatedAt: now}
	if message.Role == "assistant" {
		if len(provenance) != 2 || provenance[0] == "" || provenance[1] == "" {
			panic("assistant reply requires model and provider")
		}
		stored.Model, stored.Provider = provenance[0], provenance[1]
	}
	conversation.Messages = append(conversation.Messages, stored)
	conversation.UpdatedAt = now
	entry := memory.Entry{Kind: "message", ID: messageID, ConversationID: conversation.ID, Role: message.Role, Content: message.Content}
	if stored.Model != "" {
		entry.Metadata = map[string]any{"model": stored.Model, "provider": stored.Provider}
	}
	g.Memory.Record(entry)
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
		item := map[string]any{"id": stored.ID, "role": stored.Message.Role, "content": stored.Message.Content, "created_at": stored.CreatedAt}
		if stored.Message.Role == "assistant" {
			item["model"], item["provider"] = stored.Model, stored.Provider
		}
		messages = append(messages, item)
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
		allowed := map[string]bool{"type": true, "extensions": true}
		switch part["type"] {
		case "text":
			allowed["text"] = true
			if _, ok := part["text"].(string); !ok {
				return nil, nil, false
			}
		case "image", "file":
			allowed["url"], allowed["name"], allowed["mime_type"] = true, true, true
			if _, ok := part["url"].(string); !ok {
				return nil, nil, false
			}
		default:
			return nil, nil, false
		}
		for key, value := range part {
			if !allowed[key] {
				return nil, nil, false
			}
			if key == "extensions" {
				if _, ok := value.(map[string]any); !ok {
					return nil, nil, false
				}
			}
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
