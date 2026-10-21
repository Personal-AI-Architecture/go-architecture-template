package memory

import "sync"

type Entry struct {
	Kind           string         `json:"kind"`
	ID             string         `json:"id,omitempty"`
	ConversationID string         `json:"conversation_id,omitempty"`
	Role           string         `json:"role,omitempty"`
	Content        string         `json:"content,omitempty"`
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

func (s *Store) Record(entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (s *Store) Export() map[string]any {
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
	return map[string]any{
		"memory_root":   s.MemoryRoot,
		"entries":       entries,
		"conversations": conversations,
		"history":       history,
	}
}
