package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"cs2-tournament/models"
	"cs2-tournament/services"

	"github.com/gin-gonic/gin"
)

// maxDraftNameLength keeps a draft name short enough for a tab.
const maxDraftNameLength = 60

// POST /api/tournaments/:id/drafts
// Body: { "name": "Faceit only", "settings": { "use_premier": false, ... } }
//
// Both are optional. Settings that are left out keep their defaults (see
// services.DefaultDraftSettings). The tournament's other drafts are left
// alone, so the same players can be drafted with different settings and the
// results compared.
//
// Pipeline:
//  1. Score every player from their stored stats with the draft's settings
//  2. services.BalanceWithRequests: bench the remainder, snake draft +
//     optimise, honour teammate requests as far as fairness allows
//  3. Store the draft, its scores and its teams in one transaction
func (h *Handler) CreateDraft(c *gin.Context) {
	tournamentID := c.Param("id")

	var body struct {
		Name     string          `json:"name"`
		Settings json.RawMessage `json:"settings"`
	}
	// No body at all is a valid way to ask for the default settings.
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settings := services.DefaultDraftSettings()
	if len(body.Settings) > 0 {
		if err := json.Unmarshal(body.Settings, &settings); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settings: " + err.Error()})
			return
		}
	}
	settings = settings.Sanitized()

	var teamSize int
	err := h.db.QueryRow(`SELECT team_size FROM tournaments WHERE id=$1`, tournamentID).Scan(&teamSize)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "tournament not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if teamSize <= 0 {
		teamSize = 5
	}

	// 1. Score
	scores, _, err := scorePlayers(h.db, tournamentID, settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	players := make([]services.PlayerScore, 0, len(scores))
	for id, s := range scores {
		players = append(players, services.PlayerScore{PlayerID: id, SkillScore: s.Score})
	}

	requests, err := loadRequests(h.db, tournamentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var links []services.TeammateRequest
	if settings.HonorRequests {
		for _, r := range requests {
			links = append(links, services.TeammateRequest{ID: r.ID, RequesterID: r.RequesterID, RequestedID: r.RequestedID})
		}
	}

	// 2. Draft
	result, err := services.BalanceWithRequests(players, links, teamSize, services.BalanceOptions{
		MaxSpread: settings.MaxSpread,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf(
			"These settings can score %d players, and two teams need %d", len(players), teamSize*2)})
		return
	}
	statuses := make(map[string]string, len(result.Requests))
	for _, o := range result.Requests {
		statuses[o.ID] = string(o.Status)
	}

	// 3. Store, so a failure can't leave half a draft behind
	tx, err := h.db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	name := strings.TrimSpace(body.Name)
	if len([]rune(name)) > maxDraftNameLength {
		name = string([]rune(name)[:maxDraftNameLength])
	}
	if name == "" {
		var made int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM drafts WHERE tournament_id=$1`, tournamentID).Scan(&made); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		name = fmt.Sprintf("Draft %d", made+1)
	}

	settingsJSON, _ := json.Marshal(settings)
	statusesJSON, _ := json.Marshal(statuses)
	var draftID string
	err = tx.QueryRow(`
		INSERT INTO drafts (tournament_id, name, settings, request_statuses)
		VALUES ($1,$2,$3,$4) RETURNING id`,
		tournamentID, name, string(settingsJSON), string(statusesJSON),
	).Scan(&draftID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for id, s := range scores {
		if _, err := tx.Exec(
			`INSERT INTO draft_scores (draft_id, player_id, score, estimated) VALUES ($1,$2,$3,$4)`,
			draftID, id, s.Score, s.Estimated,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	for i, teamPlayers := range result.Teams {
		avgScore := 0.0
		for _, p := range teamPlayers {
			avgScore += p.SkillScore
		}
		avgScore /= float64(len(teamPlayers))

		var teamID string
		err := tx.QueryRow(`
			INSERT INTO teams (tournament_id, draft_id, name, avg_skill_score)
			VALUES ($1,$2,$3,$4) RETURNING id`,
			tournamentID, draftID, fmt.Sprintf("Team %d", i+1), avgScore,
		).Scan(&teamID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, p := range teamPlayers {
			if _, err := tx.Exec(
				`INSERT INTO team_players (team_id, player_id) VALUES ($1,$2)`,
				teamID, p.PlayerID,
			); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}
	if _, err := tx.Exec(`UPDATE tournaments SET status='balanced', updated_at=now() WHERE id=$1`, tournamentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	draft, err := loadDraft(h.db, tournamentID, draftID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, draft)
}

// PATCH /api/tournaments/:id/drafts/:did
// Body: { "name": "..." }
func (h *Handler) RenameDraft(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A draft needs a name"})
		return
	}
	if len([]rune(name)) > maxDraftNameLength {
		name = string([]rune(name)[:maxDraftNameLength])
	}
	result, err := h.db.Exec(
		`UPDATE drafts SET name=$1 WHERE id=$2 AND tournament_id=$3`,
		name, c.Param("did"), c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "name": name})
}

// DELETE /api/tournaments/:id/drafts/:did
func (h *Handler) DeleteDraft(c *gin.Context) {
	tournamentID := c.Param("id")
	// Deleting a draft cascades to its teams, team_players and scores
	result, err := h.db.Exec(
		`DELETE FROM drafts WHERE id=$1 AND tournament_id=$2`,
		c.Param("did"), tournamentID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "draft not found"})
		return
	}
	h.db.Exec(`
		UPDATE tournaments SET status='signup', updated_at=now()
		WHERE id=$1 AND status='balanced'
		  AND NOT EXISTS (SELECT 1 FROM drafts WHERE tournament_id=$1)`, tournamentID)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/tournaments/:id/scores
// Body: { "settings": { ... } }
//
// Scores every player with the given settings without drafting anything, so
// the player list can show what a draft with those settings would be made on.
// Nothing is fetched: the scores come from the stored stats.
func (h *Handler) PreviewScores(c *gin.Context) {
	var body struct {
		Settings json.RawMessage `json:"settings"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settings := services.DefaultDraftSettings()
	if len(body.Settings) > 0 {
		if err := json.Unmarshal(body.Settings, &settings); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid settings: " + err.Error()})
			return
		}
	}
	scores, checks, err := scorePlayers(h.db, c.Param("id"), settings.Sanitized())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"scores": scores, "rank_checks": checks})
}

// scorePlayers scores every player of a tournament from their stored stats
// with the given settings. Players who cannot be scored are left out. It
// also returns how the rank of each scored player was assessed, for those
// scored on a rank.
func scorePlayers(db *sql.DB, tournamentID string, settings services.DraftSettings) (map[string]models.DraftScore, map[string]*models.RankCheck, error) {
	rows, err := db.Query(`
		SELECT id, skill_score, raw_stats, steam_stats, faceit_stats
		FROM players WHERE tournament_id=$1`,
		tournamentID,
	)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	checks := map[string]*models.RankCheck{}
	usesFullModel := settings.ScoreSettings == services.DefaultScoreSettings()
	scores := map[string]models.DraftScore{}
	for rows.Next() {
		var id string
		var stored *float64
		var leetifyRaw, steamRaw, faceitRaw []byte
		if err := rows.Scan(&id, &stored, &leetifyRaw, &steamRaw, &faceitRaw); err != nil {
			return nil, nil, err
		}
		leetify := unmarshalOrNil[services.LeetifyProfile](leetifyRaw)
		steam := unmarshalOrNil[services.CS2Stats](steamRaw)
		faceit := unmarshalOrNil[services.FaceitPlayer](faceitRaw)

		// The full model's score, which is also all there is for a player
		// whose stats were never stored.
		full := stored
		if leetify != nil || steam != nil || faceit != nil {
			full, _ = services.CombineScores(leetify, steam, faceit)
		}
		score := full
		if !usesFullModel {
			score, _ = settings.Combine(leetify, steam, faceit)
		}

		// The rank is assessed by whichever model the score came from.
		used := settings.ScoreSettings
		switch {
		case score != nil && *score >= 0:
			scores[id] = models.DraftScore{Score: *score}
		case settings.FallbackToFullModel && full != nil && *full >= 0:
			scores[id] = models.DraftScore{Score: *full, Estimated: true}
			used = services.DefaultScoreSettings()
		default:
			continue
		}
		if check := rankCheck(used, leetify, faceit); check != nil && used.WeightRank > 0 {
			checks[id] = check
		}
	}
	return scores, checks, rows.Err()
}

// loadDrafts fetches every draft of a tournament, oldest first.
func loadDrafts(db *sql.DB, tournamentID string) ([]models.Draft, error) {
	rows, err := db.Query(
		`SELECT id FROM drafts WHERE tournament_id=$1 ORDER BY created_at, id`, tournamentID,
	)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	drafts := make([]models.Draft, 0, len(ids))
	for _, id := range ids {
		d, err := loadDraft(db, tournamentID, id)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, d)
	}
	return drafts, nil
}

// loadDraft fetches one draft with its teams and scores.
func loadDraft(db *sql.DB, tournamentID, draftID string) (models.Draft, error) {
	d := models.Draft{
		Settings:        services.DefaultDraftSettings(),
		Scores:          map[string]models.DraftScore{},
		RequestStatuses: map[string]string{},
	}
	var settingsRaw, statusesRaw []byte
	err := db.QueryRow(`
		SELECT id, tournament_id, name, settings, request_statuses, created_at
		FROM drafts WHERE id=$1 AND tournament_id=$2`,
		draftID, tournamentID,
	).Scan(&d.ID, &d.TournamentID, &d.Name, &settingsRaw, &statusesRaw, &d.CreatedAt)
	if err != nil {
		return d, err
	}
	// Settings that are missing keep their defaults.
	json.Unmarshal(settingsRaw, &d.Settings)
	if statusesRaw != nil {
		json.Unmarshal(statusesRaw, &d.RequestStatuses)
	}

	rows, err := db.Query(`SELECT player_id, score, estimated FROM draft_scores WHERE draft_id=$1`, draftID)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var id string
		var s models.DraftScore
		if err := rows.Scan(&id, &s.Score, &s.Estimated); err != nil {
			rows.Close()
			return d, err
		}
		d.Scores[id] = s
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return d, err
	}

	d.Teams, err = loadTeams(db, draftID)
	return d, err
}
