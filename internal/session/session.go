// Package session persists interactive chat sessions to disk so a conversation
// can be resumed after the terminal (or the machine) is closed. A session is a
// named, structured transcript that belongs to a workspace — the canonical
// record of a conversation, kept under .ai/sessions/.
//
// This is deliberately separate from the memory engine: the memory store holds
// chunked, embedded turns for *semantic recall*; a session holds the full,
// verbatim, replayable transcript. They are complementary, not redundant.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Message is one turn in a session.
type Message struct {
	Role     string `json:"role"`               // "user" | "assistant"
	Provider string `json:"provider,omitempty"` // backend that produced an assistant turn
	Text     string `json:"text"`
	Ts       int64  `json:"ts"` // unix seconds
}

// Record is a persisted session: its identity plus the running transcript.
type Record struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Workspace string         `json:"workspace"`
	Agent     string         `json:"agent"`
	Provider  string         `json:"provider"` // active provider at last save
	CreatedAt int64          `json:"created_at"`
	UpdatedAt int64          `json:"updated_at"`
	Messages  []Message      `json:"messages"`
	Meta      map[string]any `json:"meta,omitempty"`

	// ProviderSession is the backend's own session id (e.g. Claude Code's), from
	// the most recent turn. It is recorded so a future resume can pass it to the
	// CLI via --resume; the verbatim transcript above is replayed regardless.
	ProviderSession string `json:"provider_session,omitempty"`
}

// New mints a fresh session record. When name is empty it defaults to
// "<agent>-<id-prefix>" so every session is addressable by a human-ish name.
func New(name, workspace, agent, provider string) Record {
	now := time.Now().Unix()
	id := NewID()
	if name == "" {
		name = agent + "-" + id[:8]
	}
	return Record{
		ID:        id,
		Name:      name,
		Workspace: workspace,
		Agent:     agent,
		Provider:  provider,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// Append adds a message and advances UpdatedAt.
func (r *Record) Append(m Message) {
	if m.Ts == 0 {
		m.Ts = time.Now().Unix()
	}
	r.Messages = append(r.Messages, m)
	r.UpdatedAt = m.Ts
}

// NewID returns a random 32-char hex identifier.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
