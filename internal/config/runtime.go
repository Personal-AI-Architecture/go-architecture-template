package config

import "fmt"

type RuntimeConfiguration struct {
	MemoryRoot      string
	ProviderAdapter string
	AuthMode        string
	ToolSources     []string
}

func LoadRuntimeConfiguration(values map[string]any) (RuntimeConfiguration, error) {
	if values == nil {
		values = map[string]any{}
	}
	read := values
	if nested, ok := values["default"].(map[string]any); ok {
		read = nested
	}
	result := RuntimeConfiguration{
		MemoryRoot:      stringValue(read["memory_root"], ".paa-memory"),
		ProviderAdapter: stringValue(read["provider_adapter"], "mock"),
		AuthMode:        stringValue(read["auth_mode"], "enforced"),
		ToolSources:     []string{"memory"},
	}
	if raw, ok := read["tool_sources"]; ok {
		list, ok := raw.([]string)
		if !ok {
			return RuntimeConfiguration{}, fmt.Errorf("tool_sources must be a list of strings")
		}
		result.ToolSources = append([]string(nil), list...)
	}
	return result, nil
}

func stringValue(value any, fallback string) string {
	if text, ok := value.(string); ok && text != "" {
		return text
	}
	return fallback
}
