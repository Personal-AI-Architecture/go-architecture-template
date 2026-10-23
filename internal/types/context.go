package types

import "encoding/json"

// ContextLimits are conservative input token ceilings by provider profile and model.
type ContextLimits map[string]map[string]int

func (limits ContextLimits) Limit(provider, model string) int {
	if value := limits[provider][model]; value > 0 {
		return value
	}
	return 0
}

// ContextCost uses UTF-8 bytes as a conservative token upper bound and reserves
// space for protocol, bootstrap instructions, and the model response.
func ContextCost(messages []Message, tools []ToolDefinition) int {
	payload, _ := json.Marshal(map[string]any{"messages": messages, "tools": tools})
	return len(payload) + 384
}

func SelectCompleteTurns(messages []Message, tools []ToolDefinition, limit int) ([]Message, int, bool) {
	turns := make([][]Message, 0)
	for _, message := range messages {
		if message.Role == "user" || len(turns) == 0 {
			turns = append(turns, []Message{})
		}
		turns[len(turns)-1] = append(turns[len(turns)-1], message)
	}
	selected := make([]Message, 0)
	kept := 0
	for index := len(turns) - 1; index >= 0; index-- {
		candidate := append(append([]Message{}, turns[index]...), selected...)
		if ContextCost(candidate, tools) > limit {
			if kept == 0 {
				return nil, len(turns), false
			}
			break
		}
		selected = candidate
		kept++
	}
	return selected, len(turns) - kept, true
}
