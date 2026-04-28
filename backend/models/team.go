package models

import "time"

type Team struct {
	ID            string    `json:"id"`
	TournamentID  string    `json:"tournament_id"`
	Name          string    `json:"name"`
	AvgSkillScore float64   `json:"avg_skill_score"`
	CreatedAt     time.Time `json:"created_at"`
	Players       []Player  `json:"players"`
}
