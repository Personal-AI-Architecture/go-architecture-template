package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/gateway"
)

func main() {
	service := gateway.New(engine.NewAgentLoop(adapters.MockAdapter{Response: "Hello from a PAA Agent Loop."}, nil), nil)
	response := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"content": []map[string]any{{"type": "text", "text": "Explain what PAA is."}}}, map[string]string{
		"X-Actor-ID":          "demo-user",
		"X-Actor-Permissions": "memory:read,memory:write",
	})

	conversationID := response.Headers["X-Conversation-ID"]
	fmt.Println("PAA Go template demo")
	fmt.Println("===================")
	fmt.Println("Request: POST /conversations/messages")
	fmt.Println("Conversation:", conversationID)
	fmt.Println("Stream:")
	for _, event := range response.Stream {
		payload, _ := json.Marshal(event.Data)
		fmt.Printf("  %s: %s\n", event.Event, payload)
	}
	detail := service.Handle(context.Background(), "GET", "/conversations/"+conversationID, nil, nil)
	fmt.Printf("Persisted messages: %d\n", len(detail.Body["messages"].([]map[string]any)))
	fmt.Println("\nPAA boundary demonstrated: Gateway -> Agent Loop -> Model Adapter -> stream -> memory.")
}
