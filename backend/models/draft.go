package models

import (
	"time"

	"cs2-tournament/services"
)

// DraftScore is the score a player was drafted on.
type DraftScore struct {
	Score float64 `json:"score"`
	// Estimated: the draft's settings could not score the player, so the
	// full model stood in.
	Estimated bool `json:"estimated,omitempty"`
}

// Draft is one set of teams, and the settings and scores it was made with.
// The scores are kept as they were at the time, so a draft stays what it was
// when players are refreshed later.
type Draft struct {
	ID           string                 `json:"id"`
	TournamentID string                 `json:"tournament_id"`
	Name         string                 `json:"name"`
	Settings     services.DraftSettings `json:"settings"`
	Teams        []Team                 `json:"teams"`
	// Scores by player id. Players who are missing could not be scored.
	Scores map[string]DraftScore `json:"scores"`
	// RequestStatuses by teammate request id, as the draft left them.
	RequestStatuses map[string]string `json:"request_statuses"`
	CreatedAt       time.Time         `json:"created_at"`
}
