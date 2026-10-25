package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Preferences struct {
	Model                 string `json:"model"`
	ActiveProviderProfile string `json:"active_provider_profile"`
}

type Entry struct {
	Kind           string         `json:"kind"`
	ID             string         `json:"id,omitempty"`
	ConversationID string         `json:"conversation_id,omitempty"`
	Role           string         `json:"role,omitempty"`
	Content        any            `json:"content,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type Store struct {
	MemoryRoot string
	mu         sync.Mutex
	entries    []Entry
}

func NewStore(memoryRoot string) *Store {
	return &Store{MemoryRoot: memoryRoot}
}

func (s *Store) ReadPreferences() (Preferences, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	content, err := os.ReadFile(filepath.Join(s.MemoryRoot, "preferences.json"))
	if os.IsNotExist(err) {
		return Preferences{}, false, nil
	}
	if err != nil {
		return Preferences{}, false, err
	}
	var preference Preferences
	if err := json.Unmarshal(content, &preference); err != nil {
		return Preferences{}, false, err
	}
	if preference.Model == "" || preference.ActiveProviderProfile == "" {
		return Preferences{}, false, os.ErrInvalid
	}
	return preference, true, nil
}

func (s *Store) WritePreferences(preference Preferences) error {
	if preference.Model == "" || preference.ActiveProviderProfile == "" {
		return os.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.MemoryRoot, 0700); err != nil {
		return err
	}
	content, err := json.Marshal(preference)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(s.MemoryRoot, "preferences-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), filepath.Join(s.MemoryRoot, "preferences.json"))
}

func (s *Store) Record(entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (s *Store) Export() map[string]any {
	preferences, _, preferenceError := s.ReadPreferences()
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := append([]Entry(nil), s.entries...)
	conversations := make([]Entry, 0)
	history := make([]Entry, 0)
	for _, entry := range entries {
		if entry.Kind == "conversation" {
			conversations = append(conversations, entry)
		}
		if entry.Kind == "message" {
			history = append(history, entry)
		}
	}
	result := map[string]any{
		"memory_root":   s.MemoryRoot,
		"entries":       entries,
		"conversations": conversations,
		"history":       history,
	}
	if preferenceError == nil && preferences.Model != "" {
		result["preferences"] = preferences
	}
	return result
}
