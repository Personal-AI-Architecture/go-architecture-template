package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/engine"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/gateway"
)

func main() {
	host := flag.String("host", "127.0.0.1", "HTTP bind host")
	port := flag.Int("port", 8000, "HTTP bind port")
	flag.Parse()

	if mode := os.Getenv("RUNTIME_AUTH_MODE"); mode != "" && mode != "enforced" {
		log.Fatal("HTTP starter requires enforced Auth")
	}
	providerAdapter := os.Getenv("RUNTIME_PROVIDER_ADAPTER")
	if providerAdapter == "" {
		providerAdapter = "mock"
	}
	modelID := "mock"
	openAI := adapters.OpenAICompatibleAdapter{}
	if providerAdapter == "openai-compatible" {
		modelID = os.Getenv("PAA_MODEL_ID")
		baseURL := os.Getenv("PAA_MODEL_API_BASE_URL")
		limit, err := strconv.Atoi(os.Getenv("PAA_MODEL_CONTEXT_LIMIT"))
		if modelID == "" || baseURL == "" || err != nil || limit <= 0 {
			log.Fatal("OpenAI-compatible mode requires PAA_MODEL_ID, PAA_MODEL_API_BASE_URL, and PAA_MODEL_CONTEXT_LIMIT")
		}
		openAI.ProviderProfiles = map[string]adapters.ProviderProfile{"local": {BaseURL: baseURL, APIKeyEnv: "PAA_MODEL_API_KEY", ModelContextLimits: map[string]int{modelID: limit}}}
	}
	adapter, err := adapters.LoadModelAdapter(providerAdapter, openAI)
	if err != nil {
		log.Fatal(err)
	}
	service := gateway.New(engine.NewAgentLoop(adapter, nil), nil)
	service.CurrentModel = func() string { return modelID }
	service.CurrentProvider = func() string { return "local" }
	memoryRoot := os.Getenv("RUNTIME_MEMORY_ROOT")
	if memoryRoot == "" {
		memoryRoot = ".paa-memory"
	}
	if err := service.EnablePersistence(memoryRoot); err != nil {
		log.Fatal(err)
	}
	apiKey := os.Getenv("PAA_DEMO_API_KEY")
	if apiKey == "" {
		log.Fatal("set PAA_DEMO_API_KEY to a local test token before starting")
	}
	address := fmt.Sprintf("%s:%d", *host, *port)
	fmt.Printf("PAA Go template listening at http://%s\n", address)
	fmt.Printf("Try: curl http://%s/health\n", address)
	fmt.Println("Press Ctrl-C to stop.")
	log.Fatal(http.ListenAndServe(address, gateway.Handler(service, apiKey)))
}
