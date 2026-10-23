package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/checks"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/config"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/gateway"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/memory"
)

func main() {
	if err := run(); err != nil {
		fmt.Println("Acceptance check failed:", err)
		os.Exit(1)
	}
	fmt.Println("Acceptance check passed.")
}

func run() error {
	runtime, err := config.LoadRuntimeConfiguration(map[string]any{
		"default": map[string]any{
			"memory_root":      ".paa-memory",
			"provider_adapter": "mock",
			"auth_mode":        "enforced",
			"tool_sources":     []string{"memory"},
		},
	})
	if err != nil {
		return err
	}
	adapter, err := adapters.LoadModelAdapter(runtime.ProviderAdapter, adapters.OpenAICompatibleAdapter{APIBaseURL: "http://provider.local/v1"})
	if err != nil || adapter.Name() != "mock" {
		return fmt.Errorf("mock adapter selection failed")
	}
	openAI, err := adapters.LoadModelAdapter("openai-compatible", adapters.OpenAICompatibleAdapter{APIBaseURL: "http://provider.local/v1"})
	if err != nil || openAI.Name() != "openai-compatible" {
		return fmt.Errorf("OpenAI-compatible adapter selection failed")
	}
	if _, err := adapters.LoadModelAdapter("unknown", adapters.OpenAICompatibleAdapter{}); err == nil {
		return fmt.Errorf("unknown adapter should be rejected")
	}

	service := gateway.New(engine.NewAgentLoop(adapters.MockAdapter{Response: "acceptance-ok"}, nil), memory.NewStore(runtime.MemoryRoot))
	headers := map[string]string{"x-actor-id": "actor-acceptance", "x-actor-permissions": "memory:read,memory:write"}
	response := service.Handle(context.Background(), "POST", "/conversations/messages", map[string]any{"content": []map[string]any{{"type": "text", "text": "Hello"}}}, headers)
	if response.Status != 200 || len(response.Stream) < 2 {
		return fmt.Errorf("canonical gateway route did not stream")
	}
	conversationID := response.Headers["X-Conversation-ID"]
	listing := service.Handle(context.Background(), "GET", "/conversations", nil, nil)
	if listing.Body["total"] != 1 {
		return fmt.Errorf("conversation list did not contain created conversation")
	}
	detail := service.Handle(context.Background(), "GET", "/conversations/"+conversationID, nil, nil)
	messages, ok := detail.Body["messages"].([]map[string]any)
	if !ok || len(messages) != 2 {
		return fmt.Errorf("conversation detail did not persist user and assistant messages")
	}
	if len(service.Memory.Export()["history"].([]memory.Entry)) < 2 {
		return fmt.Errorf("memory history was not exported")
	}

	if errors := checks.CheckContracts(".", ""); len(errors) > 0 {
		return fmt.Errorf("contract checks failed: %v", errors)
	}
	if errors := checks.CheckImports("."); len(errors) > 0 {
		return fmt.Errorf("import checks failed: %v", errors)
	}
	if errors := checks.CheckLockIn("."); len(errors) > 0 {
		return fmt.Errorf("lock-in checks failed: %v", errors)
	}
	return nil
}
