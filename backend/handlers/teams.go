package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"cs2-tournament/models"
	"cs2-tournament/services"

	"github.com/gin-gonic/gin"
)

// POST /api/tournaments/:id/balance
// Body: { "team_size": 5 }
//
// Pipeline:
//  1. Load all players with a skill_score
//  2. Determine teamCount = floor(playerCount / teamSize), bench remainder
//  3. Run SnakeDraft → OptimizeTeams(10000 iterations)
//  4. Delete existing teams for this tournament
//  5. Persist new teams + team_players rows
//  6. Return full team assignments + variance metric
func (h *Handler) BalanceTeams(c *gin.Context) {
	tournamentID := c.Param("id")

	var body struct {
		TeamSize int `json:"team_size"`
	}
	c.ShouldBindJSON(&body)
	if body.TeamSize == 0 {
		// Fall back to tournament's configured team_size
		h.db.QueryRow(`SELECT team_size FROM tournaments WHERE id=$1`, tournamentID).Scan(&body.TeamSize)
	}
	if body.TeamSize == 0 {
		body.TeamSize = 5
	}

	// 1. Load players with known scores (skip private/missing)
	rows, err := h.db.Query(`
		SELECT id, skill_score FROM players
		WHERE tournament_id=$1 AND skill_score IS NOT NULL AND skill_score >= 0
		ORDER BY skill_score DESC`,
		tournamentID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var players []services.PlayerScore
	for rows.Next() {
		var p services.PlayerScore
		rows.Scan(&p.PlayerID, &p.SkillScore)
		players = append(players, p)
	}

	teamCount := len(players) / body.TeamSize
	if teamCount < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Not enough players with skill scores to form at least 2 teams"})
		return
	}

	// Bench lowest-ranked players that don't fill a full team
	activePlayers := players[:teamCount*body.TeamSize]
	benchPlayers := players[teamCount*body.TeamSize:]

	// 2+3. Snake draft + greedy optimization
	drafted := services.SnakeDraft(activePlayers, teamCount)
	optimized := services.OptimizeTeams(drafted, 10000)
	variance := services.TeamVariance(optimized)

	// 4. Delete existing teams (cascades to team_players)
	if _, err := h.db.Exec(`DELETE FROM teams WHERE tournament_id=$1`, tournamentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 5. Persist new teams
	var resultTeams []models.Team
	for i, teamPlayers := range optimized {
		avgScore := 0.0
		for _, p := range teamPlayers {
			avgScore += p.SkillScore
		}
		avgScore /= float64(len(teamPlayers))

		var teamID string
		err := h.db.QueryRow(`
			INSERT INTO teams (tournament_id, name, avg_skill_score)
			VALUES ($1,$2,$3) RETURNING id`,
			tournamentID, fmt.Sprintf("Team %d", i+1), avgScore,
		).Scan(&teamID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		for _, p := range teamPlayers {
			h.db.Exec(
				`INSERT INTO team_players (team_id, player_id) VALUES ($1,$2)`,
				teamID, p.PlayerID,
			)
		}

		// Load full player objects for the response
		playerRows, _ := h.db.Query(`
			SELECT p.id, p.steam64_id, p.steam_name, p.avatar_url, p.skill_score,
			       p.is_banned, p.ban_info, p.privacy_mode, p.faceit_elo, p.faceit_level, p.premier_rating
			FROM team_players tp
			JOIN players p ON p.id = tp.player_id
			WHERE tp.team_id=$1`, teamID)

		team := models.Team{
			ID:            teamID,
			TournamentID:  tournamentID,
			Name:          fmt.Sprintf("Team %d", i+1),
			AvgSkillScore: avgScore,
		}
		if playerRows != nil {
			defer playerRows.Close()
			for playerRows.Next() {
				var p models.Player
				var banInfoRaw []byte
				playerRows.Scan(
					&p.ID, &p.Steam64ID, &p.SteamName, &p.AvatarURL, &p.SkillScore,
					&p.IsBanned, &banInfoRaw, &p.PrivacyMode,
					&p.FaceitElo, &p.FaceitLevel, &p.PremierRating,
				)
				if banInfoRaw != nil {
					json.Unmarshal(banInfoRaw, &p.BanInfo)
				}
				team.Players = append(team.Players, p)
			}
		}
		if team.Players == nil {
			team.Players = []models.Player{}
		}
		resultTeams = append(resultTeams, team)
	}

	// Update tournament status
	h.db.Exec(`UPDATE tournaments SET status='balanced', updated_at=now() WHERE id=$1`, tournamentID)

	// Build bench player IDs for response
	benchIDs := make([]string, len(benchPlayers))
	for i, b := range benchPlayers {
		benchIDs[i] = b.PlayerID
	}

	c.JSON(http.StatusOK, gin.H{
		"variance":      variance,
		"teams":         resultTeams,
		"bench_players": benchIDs,
	})
}

// PATCH /api/tournaments/:id/teams/move
// Body: { "player_id": "...", "from_team_id": "...", "to_team_id": "..." }
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
		WHERE player_id=$2 AND team_id=$3`,
		body.ToTeamID, body.PlayerID, body.FromTeamID,
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

	// Recompute avg_skill_score for both affected teams
	for _, teamID := range []string{body.FromTeamID, body.ToTeamID} {
		h.db.Exec(`
			UPDATE teams SET avg_skill_score = COALESCE((
				SELECT AVG(p.skill_score)
				FROM team_players tp
				JOIN players p ON p.id = tp.player_id
				WHERE tp.team_id = $1
			), 0) WHERE id = $1`, teamID)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// loadTeams fetches all teams (with players) for a tournament.
func loadTeams(db *sql.DB, tournamentID string) ([]models.Team, error) {
	rows, err := db.Query(`
		SELECT id, tournament_id, name, avg_skill_score, created_at
		FROM teams WHERE tournament_id=$1 ORDER BY name`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []models.Team
	for rows.Next() {
		var t models.Team
		rows.Scan(&t.ID, &t.TournamentID, &t.Name, &t.AvgSkillScore, &t.CreatedAt)

		playerRows, _ := db.Query(`
			SELECT p.id, p.steam64_id, p.steam_name, p.avatar_url, p.skill_score,
			       p.is_banned, p.ban_info, p.privacy_mode, p.faceit_elo, p.faceit_level, p.premier_rating
			FROM team_players tp
			JOIN players p ON p.id = tp.player_id
			WHERE tp.team_id=$1
			ORDER BY p.skill_score DESC NULLS LAST`, t.ID)
		if playerRows != nil {
			defer playerRows.Close()
			for playerRows.Next() {
				var p models.Player
				var banInfoRaw []byte
				playerRows.Scan(
					&p.ID, &p.Steam64ID, &p.SteamName, &p.AvatarURL, &p.SkillScore,
					&p.IsBanned, &banInfoRaw, &p.PrivacyMode,
					&p.FaceitElo, &p.FaceitLevel, &p.PremierRating,
				)
				if banInfoRaw != nil {
					json.Unmarshal(banInfoRaw, &p.BanInfo)
				}
				t.Players = append(t.Players, p)
			}
		}
		if t.Players == nil {
			t.Players = []models.Player{}
		}
		teams = append(teams, t)
	}
	if teams == nil {
		teams = []models.Team{}
	}
	return teams, nil
}
