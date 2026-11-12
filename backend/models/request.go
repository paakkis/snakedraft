package models

import "time"

// TeammateRequest is a player's wish to be on the same team as another player.
// Status is only set in the response of a balance run.
type TeammateRequest struct {
	ID           string    `json:"id"`
	TournamentID string    `json:"tournament_id"`
	RequesterID  string    `json:"requester_id"`
	RequestedID  string    `json:"requested_id"`
	Status       string    `json:"status,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
