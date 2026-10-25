package gateway

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"github.com/Personal-AI-Architecture/go-architecture-template/internal/memory"
)

const maxRequestBytes = 1024 * 1024

func Handler(gateway *Gateway, apiKey string) http.Handler {
	if apiKey == "" {
		panic("a demo API key is required")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writeJSON(writer, http.StatusMethodNotAllowed, map[string]any{"error": map[string]any{"code": "method_not_allowed", "message": "Method not allowed."}})
			return
		}
		writeJSON(writer, http.StatusOK, map[string]any{"status": "ok", "architecture": "PAA"})
	})
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		supplied := request.Header.Get("X-API-Key")
		if bearer := request.Header.Get("Authorization"); len(bearer) > 7 && bearer[:7] == "Bearer " {
			supplied = bearer[7:]
		}
		if subtle.ConstantTimeCompare([]byte(supplied), []byte(apiKey)) != 1 {
			writeJSON(writer, http.StatusUnauthorized, map[string]any{"code": "unauthenticated", "message": "Authentication required."})
			return
		}
		var body map[string]any
		if request.Method == http.MethodPost || request.Method == http.MethodPut {
			if request.ContentLength > maxRequestBytes {
				writeJSON(writer, http.StatusRequestEntityTooLarge, map[string]any{"code": "request_too_large", "message": "Request body too large."})
				return
			}
			request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				if _, tooLarge := err.(*http.MaxBytesError); tooLarge {
					writeJSON(writer, http.StatusRequestEntityTooLarge, map[string]any{"code": "request_too_large", "message": "Request body too large."})
					return
				}
				body = nil
			}
		}
		if request.URL.Path == "/owner/preferences" {
			if request.Method == http.MethodGet {
				preference, found, err := gateway.Memory.ReadPreferences()
				if err != nil || !found {
					writeJSON(writer, http.StatusServiceUnavailable, map[string]any{"code": "selection_unavailable", "message": "Preferences unavailable."})
					return
				}
				writeJSON(writer, http.StatusOK, preference)
				return
			}
			if request.Method == http.MethodPut {
				preference := memory.Preferences{}
				preference.Model, _ = body["model"].(string)
				preference.ActiveProviderProfile, _ = body["active_provider_profile"].(string)
				if preference.Model == "" || preference.ActiveProviderProfile == "" {
					writeJSON(writer, http.StatusBadRequest, map[string]any{"code": "invalid_request", "message": "Model and active_provider_profile are required."})
					return
				}
				if err := gateway.Memory.WritePreferences(preference); err != nil {
					writeJSON(writer, http.StatusInternalServerError, map[string]any{"code": "storage_error", "message": "Preferences could not be saved."})
					return
				}
				writeJSON(writer, http.StatusOK, preference)
				return
			}
		}
		headers := map[string]string{"X-Actor-ID": "local-owner", "X-Actor-Permissions": "memory:read,memory:write"}
		response := gateway.Handle(request.Context(), request.Method, request.URL.RequestURI(), body, headers)
		if len(response.Stream) > 0 {
			writeStream(writer, response)
			return
		}
		writeResponse(writer, response)
	})
	return mux
}

func writeResponse(writer http.ResponseWriter, response Response) {
	for key, value := range response.Headers {
		writer.Header().Set(key, value)
	}
	if response.Body == nil {
		response.Body = map[string]any{}
	}
	writeJSON(writer, response.Status, response.Body)
}

func writeJSON(writer http.ResponseWriter, status int, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		http.Error(writer, "unable to encode response", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(payload)
}

func writeStream(writer http.ResponseWriter, response Response) {
	for key, value := range response.Headers {
		writer.Header().Set(key, value)
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.WriteHeader(response.Status)
	flusher, _ := writer.(http.Flusher)
	for _, event := range response.Stream {
		payload, _ := json.Marshal(event.Data)
		_, _ = writer.Write([]byte("event: " + event.Event + "\ndata: " + string(payload) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
}
