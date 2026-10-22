package checks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const modulePath = "github.com/Personal-AI-Architecture/go-architecture-template"

type lockFile struct {
	Repository    string            `json:"repository"`
	Ref           string            `json:"ref"`
	Commit        string            `json:"commit"`
	SourceRoot    string            `json:"source_root"`
	ContractFiles []string          `json:"contract_files"`
	SHA256        map[string]string `json:"sha256"`
}

func Run(root, sourceCheckout string) []string {
	errors := append([]string{}, CheckFormat(root)...)
	errors = append(errors, CheckContracts(root, sourceCheckout)...)
	errors = append(errors, CheckImports(root)...)
	errors = append(errors, CheckLockIn(root)...)
	return errors
}

func CheckFormat(root string) []string {
	files := goFiles(root)
	if len(files) == 0 {
		return []string{"No Go source files found."}
	}
	args := append([]string{"-l"}, files...)
	output, err := exec.Command("gofmt", args...).CombinedOutput()
	if err != nil {
		return []string{fmt.Sprintf("gofmt failed: %s", strings.TrimSpace(string(output)))}
	}
	if strings.TrimSpace(string(output)) != "" {
		return []string{"Go files need formatting: " + strings.TrimSpace(string(output))}
	}
	return nil
}

func CheckContracts(root, sourceCheckout string) []string {
	lockBytes, err := os.ReadFile(filepath.Join(root, "paa.lock.json"))
	if err != nil {
		return []string{fmt.Sprintf("Unable to read paa.lock.json: %v", err)}
	}
	var lock lockFile
	if err := json.Unmarshal(lockBytes, &lock); err != nil {
		return []string{fmt.Sprintf("paa.lock.json is not valid JSON: %v", err)}
	}
	errors := make([]string, 0)
	if lock.Repository != "https://github.com/Personal-AI-Architecture/the-architecture.git" {
		errors = append(errors, "paa.lock.json repository must point to the PAA architecture repository.")
	}
	if lock.Ref != "main" {
		errors = append(errors, "paa.lock.json ref must be main.")
	}
	if len(lock.Commit) != 40 {
		errors = append(errors, "paa.lock.json commit must be a full 40-character SHA.")
	}
	if lock.SourceRoot != "vendor/paa-contracts" {
		errors = append(errors, "paa.lock.json source_root must be vendor/paa-contracts.")
	}

	vendorRoot := filepath.Join(root, lock.SourceRoot)
	actual := make([]string, 0)
	_ = filepath.WalkDir(vendorRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, _ := filepath.Rel(vendorRoot, path)
		actual = append(actual, filepath.ToSlash(relative))
		return nil
	})
	if !sameStrings(actual, lock.ContractFiles) {
		errors = append(errors, "Vendored contract file set differs from paa.lock.json.")
	}
	for _, relative := range lock.ContractFiles {
		vendorPath := filepath.Join(vendorRoot, filepath.FromSlash(relative))
		mirrorPath := filepath.Join(root, filepath.FromSlash(relative))
		vendorBytes, readErr := os.ReadFile(vendorPath)
		if readErr != nil {
			errors = append(errors, fmt.Sprintf("Missing vendored contract: %s.", filepath.ToSlash(filepath.Join(lock.SourceRoot, relative))))
			continue
		}
		if hashBytes(vendorBytes) != lock.SHA256[relative] {
			errors = append(errors, fmt.Sprintf("Hash drift in vendored contract: %s.", relative))
		}
		mirrorBytes, readErr := os.ReadFile(mirrorPath)
		if readErr != nil || string(mirrorBytes) != string(vendorBytes) {
			errors = append(errors, fmt.Sprintf("Local contract mirror differs from vendored source: %s.", relative))
		}
		if sourceCheckout != "" {
			command := exec.Command("git", "-C", sourceCheckout, "show", lock.Commit+":"+relative)
			sourceBytes, sourceErr := command.Output()
			if sourceErr != nil || string(sourceBytes) != string(vendorBytes) {
				errors = append(errors, fmt.Sprintf("Vendored contract differs from %s:%s.", lock.Commit, relative))
			}
		}
	}

	gateway := readText(root, "contracts/openapi/gateway-api.yaml")
	engine := readText(root, "contracts/openapi/gateway-engine.yaml")
	model := readText(root, "contracts/openapi/model-api.yaml")
	for _, route := range []string{"/conversations/messages", "/conversations", "/conversations/{id}", "/conversations/{id}/messages"} {
		if !strings.Contains(gateway, "  "+route+":") {
			errors = append(errors, "Gateway API path is missing: "+route+".")
		}
	}
	if strings.Contains(gateway, "  /chat:") || strings.Contains(gateway, "conversation_id}") {
		errors = append(errors, "Gateway API contains a non-PAA route or path parameter.")
	}
	for _, marker := range []string{"SendMessageRequest", "ConversationList", "Conversation", "ClientSSEStream"} {
		if !strings.Contains(gateway, "    "+marker+":") {
			errors = append(errors, "Gateway schema is missing: "+marker+".")
		}
	}
	for _, marker := range []string{"/agent-loop/chat:", "AgentLoopRequest:", "RequestMetadata:", "SSEStream:", "auth_context:"} {
		if !strings.Contains(engine, marker) {
			errors = append(errors, "Gateway-Agent Loop contract is missing: "+marker+".")
		}
	}
	for _, marker := range []string{"/completions:", "CompletionRequest:", "CompletionStream:"} {
		if !strings.Contains(model, marker) {
			errors = append(errors, "Model API contract is missing: "+marker+".")
		}
	}
	if strings.Contains(model, "  /model/chat/completions:") {
		errors = append(errors, "Model API promotes a provider-native route into the PAA contract.")
	}
	for relative, required := range map[string][]string{
		"contracts/schemas/configuration.json":         {"memory_root", "provider_adapter", "auth_mode", "tool_sources"},
		"contracts/schemas/identity.json":              {"id", "type", "display_name", "status", "created"},
		"contracts/schemas/message.json":               {"role", "content"},
		"contracts/schemas/policy.json":                {"subject", "resource", "action", "effect"},
		"contracts/schemas/tool-definition.json":       {"name", "description", "parameters", "source", "visibility"},
		"contracts/schemas/auth-context.json":          {"actor", "authentication_id", "authenticated_at", "correlation_id"},
		"contracts/schemas/authentication-result.json": {"authenticated", "reason", "correlation_id"},
		"contracts/schemas/authorization-request.json": {"actor_id", "resource", "action", "correlation_id"},
		"contracts/schemas/authorization-result.json":  {"allowed", "reason", "audit_id", "correlation_id"},
		"contracts/schemas/approval-request.json":      {"request_id", "tool_name", "summary", "status", "created_at", "expires_at"},
		"contracts/schemas/approval-decision.json":     {"request_id", "decision", "decided_at"},
		"contracts/schemas/audit-record.json":          {"id", "occurred_at", "event", "actor_id", "action", "result", "correlation_id"},
	} {
		var schema map[string]any
		bytes := []byte(readText(root, relative))
		if err := json.Unmarshal(bytes, &schema); err != nil {
			errors = append(errors, fmt.Sprintf("%s is not valid JSON: %v", relative, err))
			continue
		}
		if schema["type"] != "object" {
			errors = append(errors, relative+" must define an object schema.")
		}
		requiredValues, _ := schema["required"].([]any)
		for _, key := range required {
			found := false
			for _, value := range requiredValues {
				if value == key {
					found = true
				}
			}
			if !found {
				errors = append(errors, fmt.Sprintf("%s is missing required field: %s.", relative, key))
			}
		}
	}
	return errors
}

func CheckImports(root string) []string {
	allowed := map[string]map[string]bool{
		"adapters": {"adapters": true, "types": true},
		"config":   {"config": true, "types": true},
		"engine":   {"engine": true, "types": true, "adapters": true},
		"gateway":  {"gateway": true, "types": true, "engine": true, "memory": true},
		"memory":   {"memory": true, "types": true},
		"types":    {"types": true},
	}
	errors := make([]string, 0)
	for _, path := range goFiles(filepath.Join(root, "internal")) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		relative, _ := filepath.Rel(filepath.Join(root, "internal"), path)
		owner := strings.Split(filepath.ToSlash(relative), "/")[0]
		if _, ok := allowed[owner]; !ok {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Unable to parse imports in %s: %v", relative, err))
			continue
		}
		for _, importSpec := range file.Imports {
			importPath := strings.Trim(importSpec.Path.Value, "\"")
			if !strings.HasPrefix(importPath, modulePath+"/internal/") {
				continue
			}
			parts := strings.Split(strings.TrimPrefix(importPath, modulePath+"/internal/"), "/")
			target := parts[0]
			if !allowed[owner][target] {
				errors = append(errors, fmt.Sprintf("Forbidden component coupling in %s: %s", relative, importPath))
			}
		}
	}
	return errors
}

func CheckLockIn(root string) []string {
	errors := make([]string, 0)
	for _, relative := range []string{"internal/adapters/loader.go", "internal/adapters/mock.go", "internal/adapters/openai_compatible.go", "internal/engine/loop.go", "internal/types/contracts.go"} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			errors = append(errors, "Missing required file: "+relative)
		}
	}
	providerTokens := []string{"/chat/completions", "APIBaseURL", "APIKey", "openai-compatible"}
	for _, path := range goFiles(filepath.Join(root, "internal")) {
		relative, _ := filepath.Rel(root, path)
		if strings.HasPrefix(filepath.ToSlash(relative), "internal/checks/") {
			continue
		}
		if strings.HasPrefix(filepath.ToSlash(relative), "internal/adapters/") {
			continue
		}
		source := readFile(path)
		for _, token := range providerTokens {
			if strings.Contains(source, token) {
				errors = append(errors, fmt.Sprintf("Provider-specific token %q must stay within internal/adapters/: %s", token, filepath.ToSlash(relative)))
			}
		}
	}
	contracts := readText(root, "internal/types/contracts.go")
	metadataStart := strings.Index(contracts, "type EngineRequestMetadata struct")
	if metadataStart < 0 {
		errors = append(errors, "EngineRequestMetadata contract is missing.")
	} else {
		metadata := contracts[metadataStart:]
		if end := strings.Index(metadata, "}"); end >= 0 {
			metadata = metadata[:end]
		}
		for _, forbidden := range []string{"\n\tProviderAdapter ", "\n\tModel ", "\n\tTools ", "\n\tToolSources "} {
			if strings.Contains(metadata, forbidden) {
				errors = append(errors, "EngineRequestMetadata must not include runtime reconfiguration key: "+forbidden)
			}
		}
	}
	routes := readText(root, "internal/gateway/routes.go")
	for _, marker := range []string{"/conversations/messages", "/conversations", "/messages"} {
		if !strings.Contains(routes, marker) {
			errors = append(errors, "Gateway runtime is missing route marker: "+marker)
		}
	}
	return errors
}

func goFiles(root string) []string {
	files := make([]string, 0)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files
}

func readText(root, relative string) string {
	return readFile(filepath.Join(root, filepath.FromSlash(relative)))
}

func readFile(path string) string {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func hashBytes(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])
}

func sameStrings(left, right []string) bool {
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	for _, values := range [][]string{leftCopy, rightCopy} {
		for index := range values {
			values[index] = filepath.ToSlash(values[index])
		}
	}
	if len(leftCopy) != len(rightCopy) {
		return false
	}
	for _, value := range leftCopy {
		found := false
		for _, candidate := range rightCopy {
			if value == candidate {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
