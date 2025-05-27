package types

import (
	"time"
)

// DebateMessage represents a message in the debate
type DebateMessage struct {
	AgentID   string    `json:"agent_id"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// DebateStyle represents a debate style
type DebateStyle string

const (
	// Debate styles
	Socratic     DebateStyle = "socratic"
	Adversarial  DebateStyle = "adversarial"
	Collaborative DebateStyle = "collaborative"
	Analytical   DebateStyle = "analytical"
	Philosophical DebateStyle = "philosophical"
	Humorous     DebateStyle = "humorous"
	Devil        DebateStyle = "devil's advocate"
	Pragmatic    DebateStyle = "pragmatic"
)

// AllDebateStyles returns all available debate styles
func AllDebateStyles() []DebateStyle {
	return []DebateStyle{
		Socratic,
		Adversarial,
		Collaborative,
		Analytical,
		Philosophical,
		Humorous,
		Devil,
		Pragmatic,
	}
}
