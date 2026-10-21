package adapters

import (
	"context"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

type MockAdapter struct {
	Response string
}

func (a MockAdapter) Name() string {
	return "mock"
}

func (a MockAdapter) Stream(_ context.Context, _ types.CompletionRequest) ([]types.ModelStreamChunk, error) {
	response := a.Response
	if response == "" {
		response = "mock response"
	}
	return []types.ModelStreamChunk{
		{Type: "text-delta", Delta: map[string]any{"content": response}},
		{Type: "done", Delta: map[string]any{"finish_reason": "stop"}},
	}, nil
}
