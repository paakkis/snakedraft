package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"cs2-tournament/models"

	"github.com/gin-gonic/gin"
)

// PATCH /api/tournaments/:id/teams/move
// Body: { "player_id": "...", "from_team_id": "...", "to_team_id": "..." }
//
// Both teams must belong to the same draft of this tournament.
func (h *Handler) MovePlayer(c *gin.Context) {
	var body struct {
		PlayerID   string `json:"player_id" binding:"required"`
		FromTeamID string `json:"from_team_id" binding:"required"`
		ToTeamID   string `json:"to_team_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.db.Exec(`
		UPDATE team_players SET team_id=$1, manually_moved=true
		WHERE player_id=$2 AND team_id=$3
		  AND EXISTS (
			SELECT 1 FROM teams src JOIN teams dst ON dst.draft_id = src.draft_id
			WHERE src.id=$3 AND dst.id=$1 AND src.tournament_id=$4
		  )`,
		body.ToTeamID, body.PlayerID, body.FromTeamID, c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "player not found in source team"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// loadTeams fetches the teams of a draft, each with its players. A player's
// skill score is the one they were drafted on, and the team average is the
// average of those.
func loadTeams(db *sql.DB, draftID string) ([]models.Team, error) {
	rows, err := db.Query(`
		SELECT id, tournament_id, draft_id, name, created_at
		FROM teams WHERE draft_id=$1 ORDER BY length(name), name`,
		draftID,
	)
	if err != nil {
		return nil, err
	}
	teams := []models.Team{}
	for rows.Next() {
		t := models.Team{Players: []models.Player{}}
		if err := rows.Scan(&t.ID, &t.TournamentID, &t.DraftID, &t.Name, &t.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		teams = append(teams, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range teams {
		t := &teams[i]
		playerRows, err := db.Query(`
			SELECT p.id, p.steam64_id, p.steam_name, p.avatar_url, ds.score, p.hours_played,
			       p.is_banned, p.ban_info, p.privacy_mode, p.faceit_elo, p.faceit_level, p.premier_rating
			FROM team_players tp
			JOIN players p ON p.id = tp.player_id
			LEFT JOIN draft_scores ds ON ds.draft_id = $2 AND ds.player_id = p.id
			WHERE tp.team_id=$1
			ORDER BY ds.score DESC NULLS LAST, p.steam_name`, t.ID, draftID)
		if err != nil {
			return nil, err
		}
		sum, scored := 0.0, 0
		for playerRows.Next() {
			var p models.Player
			var banInfoRaw []byte
			if err := playerRows.Scan(
				&p.ID, &p.Steam64ID, &p.SteamName, &p.AvatarURL, &p.SkillScore, &p.HoursPlayed,
				&p.IsBanned, &banInfoRaw, &p.PrivacyMode,
				&p.FaceitElo, &p.FaceitLevel, &p.PremierRating,
			); err != nil {
				playerRows.Close()
				return nil, err
			}
			if banInfoRaw != nil {
				json.Unmarshal(banInfoRaw, &p.BanInfo)
			}
			if p.SkillScore != nil {
				sum += *p.SkillScore
				scored++
			}
			t.Players = append(t.Players, p)
		}
		playerRows.Close()
		if err := playerRows.Err(); err != nil {
			return nil, err
		}
		if scored > 0 {
			t.AvgSkillScore = sum / float64(scored)
		}
	}
	return teams, nil
}
