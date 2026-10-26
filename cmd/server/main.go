package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/adapters"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/agentloop"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/gateway"
	"github.com/Personal-AI-Architecture/go-architecture-template/internal/memory"
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
	providerID := "local"
	openAI := adapters.OpenAICompatibleAdapter{}
	if providerAdapter == "openai-compatible" {
		profiles := map[string]adapters.ProviderProfile{}
		if path := os.Getenv("PAA_ADAPTER_CONFIG_FILE"); path != "" {
			content, err := os.ReadFile(path)
			if err != nil {
				log.Fatal(err)
			}
			var document struct {
				ProviderProfiles map[string]adapters.ProviderProfile `json:"provider_profiles"`
			}
			if err := json.Unmarshal(content, &document); err != nil {
				log.Fatal(err)
			}
			profiles = document.ProviderProfiles
		} else if raw := os.Getenv("PAA_MODEL_PROFILES_JSON"); raw != "" {
			if err := json.Unmarshal([]byte(raw), &profiles); err != nil {
				log.Fatal(err)
			}
		} else {
			modelID = os.Getenv("PAA_MODEL_ID")
			baseURL := os.Getenv("PAA_MODEL_API_BASE_URL")
			limit, err := strconv.Atoi(os.Getenv("PAA_MODEL_CONTEXT_LIMIT"))
			if modelID == "" || baseURL == "" || err != nil || limit <= 0 {
				log.Fatal("OpenAI-compatible mode requires provider profiles or PAA_MODEL_ID, PAA_MODEL_API_BASE_URL, and PAA_MODEL_CONTEXT_LIMIT")
			}
			profiles["local"] = adapters.ProviderProfile{BaseURL: baseURL, APIKeyEnv: "PAA_MODEL_API_KEY", ModelContextLimits: map[string]int{modelID: limit}}
		}
		if len(profiles) == 0 {
			log.Fatal("OpenAI-compatible mode requires provider profiles")
		}
		openAI.ProviderProfiles = profiles
		providerID = os.Getenv("PAA_DEFAULT_PROVIDER")
		if providerID == "" {
			keys := make([]string, 0, len(profiles))
			for key := range profiles {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			providerID = keys[0]
		}
		profile, ok := profiles[providerID]
		if !ok {
			log.Fatal("Default provider profile is not configured")
		}
		if selected := os.Getenv("PAA_MODEL_ID"); selected != "" {
			modelID = selected
		}
		if modelID == "" {
			keys := make([]string, 0, len(profile.ModelContextLimits))
			for key := range profile.ModelContextLimits {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			if len(keys) == 0 {
				log.Fatal("Default provider profile has no model limits")
			}
			modelID = keys[0]
		}
	}
	adapter, err := adapters.LoadModelAdapter(providerAdapter, openAI)
	if err != nil {
		log.Fatal(err)
	}
	memoryRoot := os.Getenv("RUNTIME_MEMORY_ROOT")
	if memoryRoot == "" {
		memoryRoot = ".paa-memory"
	}
	mem := memory.NewStore(memoryRoot)
	if _, found, err := mem.ReadPreferences(); err != nil {
		log.Fatal(err)
	} else if !found {
		if err := mem.WritePreferences(memory.Preferences{Model: modelID, ActiveProviderProfile: providerID}); err != nil {
			log.Fatal(err)
		}
	}
	service := gateway.New(agentloop.NewAgentLoop(adapter, nil), mem)
	service.CurrentSelection = func() (string, string, error) {
		selection, found, err := mem.ReadPreferences()
		if err != nil {
			return "", "", err
		}
		if !found {
			return "", "", fmt.Errorf("owner preferences unavailable")
		}
		return selection.Model, selection.ActiveProviderProfile, nil
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
