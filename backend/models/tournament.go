package models

import "time"

type Tournament struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	TeamSize  int               `json:"team_size"`
	Status    string            `json:"status"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Players   []Player          `json:"players"`
	Drafts    []Draft           `json:"drafts"`
	Requests  []TeammateRequest `json:"teammate_requests"`
}
