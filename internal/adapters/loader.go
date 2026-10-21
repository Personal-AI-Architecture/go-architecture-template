package adapters

import (
	"fmt"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

func LoadModelAdapter(provider string, openAI OpenAICompatibleAdapter) (types.ModelAdapter, error) {
	switch provider {
	case "mock", "local":
		return MockAdapter{}, nil
	case "openai-compatible":
		return openAI, nil
	default:
		return nil, fmt.Errorf("unsupported provider adapter: %s", provider)
	}
}
