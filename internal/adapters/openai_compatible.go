package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

type OpenAICompatibleAdapter struct {
	APIBaseURL       string
	APIKey           string
	Client           *http.Client
	ProviderProfiles map[string]ProviderProfile
}

type ProviderProfile struct {
	BaseURL            string
	APIKeyEnv          string
	ModelContextLimits map[string]int
}

var providerCapabilityName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

func normalizedProviderFailure(body []byte) map[string]any {
	var response struct {
		Error struct {
			Code       string `json:"code"`
			Type       string `json:"type"`
			Capability string `json:"capability"`
			Param      string `json:"param"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &response) != nil {
		return map[string]any{"code": "provider_error"}
	}
	code := response.Error.Code
	if code == "" {
		code = response.Error.Type
	}
	if code == "model_not_found" || code == "model_unavailable" {
		return map[string]any{"code": "model_unavailable"}
	}
	if code == "unsupported_capability" || code == "capability_unsupported" {
		capability := response.Error.Capability
		if capability == "" {
			capability = response.Error.Param
		}
		if providerCapabilityName.MatchString(capability) {
			return map[string]any{"code": "capability_unsupported", "capability": capability}
		}
	}
	return map[string]any{"code": "provider_error"}
}

func (a OpenAICompatibleAdapter) ContextLimits() types.ContextLimits {
	limits := types.ContextLimits{}
	for name, profile := range a.ProviderProfiles {
		limits[name] = profile.ModelContextLimits
	}
	return limits
}

func (a OpenAICompatibleAdapter) Name() string {
	return "openai-compatible"
}

func (a OpenAICompatibleAdapter) Stream(ctx context.Context, request types.CompletionRequest) ([]types.ModelStreamChunk, error) {
	if strings.TrimSpace(request.Model) == "" || strings.TrimSpace(request.Provider) == "" {
		return nil, fmt.Errorf("model API request requires model and provider")
	}
	baseURL, apiKey := a.APIBaseURL, a.APIKey
	if len(a.ProviderProfiles) > 0 {
		profile, ok := a.ProviderProfiles[request.Provider]
		if !ok {
			return nil, fmt.Errorf("unknown selected provider profile")
		}
		baseURL, apiKey = profile.BaseURL, ""
		if profile.APIKeyEnv != "" {
			apiKey = os.Getenv(profile.APIKeyEnv)
			if apiKey == "" {
				return nil, fmt.Errorf("selected provider profile secret is unavailable")
			}
		}
	} else if request.Provider != "local" {
		return nil, fmt.Errorf("unknown selected provider profile")
	}
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("selected provider profile has no base URL")
	}
	client := a.Client
	if client == nil {
		client = http.DefaultClient
	}
	payload := map[string]any{
		"model":    request.Model,
		"messages": providerMessages(request.Messages),
		"stream":   true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		return []types.ModelStreamChunk{{Type: "error", Delta: normalizedProviderFailure(body)}}, nil
	}

	var chunks []types.ModelStreamChunk
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var providerChunk struct {
			Choices []struct {
				Delta        map[string]any `json:"delta"`
				FinishReason string         `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &providerChunk); err != nil {
			return nil, err
		}
		if len(providerChunk.Choices) == 0 {
			continue
		}
		choice := providerChunk.Choices[0]
		if len(choice.Delta) > 0 {
			chunks = append(chunks, types.ModelStreamChunk{Type: "text-delta", Delta: choice.Delta})
		}
		if choice.FinishReason != "" {
			chunks = append(chunks, types.ModelStreamChunk{Type: "done", Delta: map[string]any{"finish_reason": choice.FinishReason}})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func providerMessages(messages []types.Message) []map[string]any {
	result := make([]map[string]any, 0, len(messages))
	for _, message := range messages {
		item := map[string]any{"role": message.Role, "content": message.Content}
		if len(message.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   call.ID,
					"type": "function",
					"function": map[string]any{
						"name":      call.Name,
						"arguments": call.Arguments,
					},
				})
			}
			item["tool_calls"] = calls
		}
		if message.ToolCallID != "" {
			item["tool_call_id"] = message.ToolCallID
		}
		result = append(result, item)
	}
	return result
}
