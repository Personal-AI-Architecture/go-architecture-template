package adapters

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/types"
)

func TestLoadModelAdapterUsesRuntimeProvider(t *testing.T) {
	adapter, err := LoadModelAdapter("mock", OpenAICompatibleAdapter{})
	if err != nil || adapter.Name() != "mock" {
		t.Fatalf("mock adapter selection failed: %v", err)
	}
	local, err := LoadModelAdapter("local", OpenAICompatibleAdapter{})
	if err != nil || local.Name() != "mock" {
		t.Fatalf("local adapter selection failed: %v", err)
	}
	if _, err := LoadModelAdapter("unknown", OpenAICompatibleAdapter{}); err == nil {
		t.Fatal("unknown adapter should fail")
	}
}

func TestOpenAICompatibleAdapterNormalizesProviderStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected provider route: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = writer.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	adapter := OpenAICompatibleAdapter{APIBaseURL: server.URL, Model: "test", Client: server.Client()}
	chunks, err := adapter.Stream(context.Background(), types.CompletionRequest{Messages: []types.Message{{Role: "user", Content: "hello"}}, Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0].Delta["content"] != "ok" || !strings.EqualFold(chunks[1].Delta["finish_reason"].(string), "stop") {
		t.Fatalf("unexpected normalized provider chunks: %#v", chunks)
	}
}
