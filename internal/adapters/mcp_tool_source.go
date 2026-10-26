package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

// MCPToolSource translates one configured external source into canonical tools.
type MCPToolSource struct {
	ID, URL     string
	Client      *http.Client
	mu          sync.Mutex
	session     string
	initialized bool
	nextID      int
	known       map[string]bool
}

func NewMCPToolSource(id, url string, client *http.Client) (*MCPToolSource, error) {
	if strings.TrimSpace(id) == "" || !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		return nil, errors.New("invalid MCP source configuration")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &MCPToolSource{ID: id, URL: url, Client: client, nextID: 1, known: map[string]bool{}}, nil
}

func (s *MCPToolSource) send(method string, params map[string]any, notification bool) (map[string]any, error) {
	payload := map[string]any{"jsonrpc": "2.0", "method": method}
	if !notification {
		payload["id"] = s.nextID
		s.nextID++
		payload["params"] = params
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", s.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-03-26")
	if s.session != "" {
		req.Header.Set("Mcp-Session-Id", s.session)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("MCP request failed (%d)", resp.StatusCode)
	}
	if value := resp.Header.Get("Mcp-Session-Id"); value != "" {
		s.session = value
	}
	if notification {
		return nil, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "data:") {
				raw = []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				break
			}
		}
	}
	var result struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.Error != nil || result.Result == nil {
		return nil, errors.New("MCP response was invalid or returned an error")
	}
	return result.Result, nil
}

func (s *MCPToolSource) initialize() error {
	if s.initialized {
		return nil
	}
	if _, err := s.send("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "paa-template", "version": "0.1.0"}}, false); err != nil {
		return err
	}
	if _, err := s.send("notifications/initialized", nil, true); err != nil {
		return err
	}
	s.initialized = true
	return nil
}

func (s *MCPToolSource) Discover() ([]types.ToolDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.initialize(); err != nil {
		return nil, err
	}
	s.known = map[string]bool{}
	var tools []types.ToolDefinition
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := s.send("tools/list", params, false)
		if err != nil {
			return nil, err
		}
		entries, _ := result["tools"].([]any)
		for _, entry := range entries {
			item, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			name, _ := item["name"].(string)
			if strings.TrimSpace(name) == "" {
				continue
			}
			description, _ := item["description"].(string)
			parameters, _ := item["inputSchema"].(map[string]any)
			if parameters == nil {
				parameters = map[string]any{}
			}
			s.known[name] = true
			tools = append(tools, types.ToolDefinition{Name: name, Description: description, Parameters: parameters, Source: types.ToolSource{ID: s.ID, Kind: "mcp"}, Category: "discoverable"})
		}
		cursor, _ = result["nextCursor"].(string)
		if cursor == "" {
			return tools, nil
		}
	}
}

func (s *MCPToolSource) Call(name string, args map[string]any) (map[string]any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.known[name] {
		return nil, errors.New("tool was not discovered from this source")
	}
	return s.send("tools/call", map[string]any{"name": name, "arguments": args}, false)
}
