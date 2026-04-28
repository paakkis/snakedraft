package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"cs2-tournament/models"
	"cs2-tournament/services"

	"github.com/gin-gonic/gin"
)

// jsonParam converts marshalled bytes to a string for JSONB columns,
// or nil so the driver sends SQL NULL. pq sends []byte as bytea, which
// PostgreSQL rejects for JSONB; a string is sent as text and accepted.
func jsonParam(b []byte) interface{} {
	if b == nil {
		return nil
	}
	return string(b)
}

// POST /api/tournaments/:id/players
// Body: { "steam_url": "https://steamcommunity.com/id/username" }
func (h *Handler) AddPlayer(c *gin.Context) {
	tournamentID := c.Param("id")

	var body struct {
		SteamURL string `json:"steam_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	steam64ID, vanityURL, err := h.steam.ParseSteamInput(body.SteamURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not resolve Steam URL: " + err.Error()})
		return
	}
	log.Printf("[players] input=%q → steam64=%s vanity=%s", body.SteamURL, steam64ID, vanityURL)

	var exists bool
	h.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM players WHERE tournament_id=$1 AND steam64_id=$2)`,
		tournamentID, steam64ID,
	).Scan(&exists)
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "Player already in tournament"})
		return
	}

	summary, err := h.steam.GetPlayerSummary(steam64ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not fetch Steam profile: " + err.Error()})
		return
	}

	time.Sleep(100 * time.Millisecond)
	profile, leetifyErr := h.fetchLeetify(steam64ID)
	if leetifyErr != nil {
		log.Printf("[players] Leetify fetch failed for %s: %v", steam64ID, leetifyErr)
	}

	steamStats, steamErr := h.steam.GetCS2Stats(steam64ID)
	if steamErr != nil {
		log.Printf("[players] Steam stats fetch failed for %s: %v", steam64ID, steamErr)
	}

	var (
		skillScore    *float64
		isBanned      bool
		privacyMode   = "unknown"
		faceitElo     *int
		faceitLevel   *int
		premierRating *int
		banInfoJSON   []byte
		rawStatsJSON  []byte
		rating        *models.Rating
	)

	if profile != nil {
		leetifyScore := services.ComputeSkillScore(profile)
		isBanned = len(profile.Bans) > 0
		privacyMode = profile.PrivacyMode

		if profile.Ranks.FaceitElo != nil {
			faceitElo = profile.Ranks.FaceitElo
		}
		if profile.Ranks.Faceit > 0 {
			lvl := profile.Ranks.Faceit
			faceitLevel = &lvl
		}
		if profile.Ranks.Premier > 0 {
			pr := profile.Ranks.Premier
			premierRating = &pr
		}
		if isBanned {
			banInfoJSON, _ = json.Marshal(profile.Bans)
		}
		rawStatsJSON, _ = json.Marshal(profile)

		rating = &models.Rating{
			Aim:         profile.Rating.Aim,
			Positioning: profile.Rating.Positioning,
			Utility:     profile.Rating.Utility,
			Clutch:      profile.Rating.Clutch,
		}

		// Blend: if we have a valid Leetify score and Steam stats, supplement
		// Leetify (90%) with Steam-derived component (10%) for a richer signal.
		// If Leetify returned private/unknown (-1), fall back to Steam only.
		var finalScore float64
		if leetifyScore >= 0 && steamStats != nil {
			steamScore := services.ComputeSteamScore(steamStats)
			finalScore = leetifyScore*0.90 + steamScore*0.10
		} else {
			finalScore = leetifyScore
		}
		skillScore = &finalScore
	} else if steamStats != nil {
		// No Leetify data at all — use Steam stats as sole score source.
		steamScore := services.ComputeSteamScore(steamStats)
		skillScore = &steamScore
		privacyMode = "steam_only"
	}

	var hoursPlayed *float64
	if steamStats != nil && steamStats.HoursPlayed > 0 {
		h := steamStats.HoursPlayed
		hoursPlayed = &h
	}

	now := time.Now()
	var playerID string
	err = h.db.QueryRow(`
		INSERT INTO players (
			tournament_id, steam64_id, vanity_url, steam_name, avatar_url,
			faceit_elo, faceit_level, premier_rating,
			skill_score, hours_played, is_banned, ban_info, privacy_mode, raw_stats, fetched_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		RETURNING id`,
		tournamentID, steam64ID, vanityURL, summary.Name, summary.AvatarURL,
		faceitElo, faceitLevel, premierRating,
		skillScore, hoursPlayed, isBanned, jsonParam(banInfoJSON), privacyMode, jsonParam(rawStatsJSON), now,
	).Scan(&playerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	player := models.Player{
		ID:            playerID,
		TournamentID:  tournamentID,
		Steam64ID:     steam64ID,
		VanityURL:     vanityURL,
		SteamName:     summary.Name,
		AvatarURL:     summary.AvatarURL,
		FaceitElo:     faceitElo,
		FaceitLevel:   faceitLevel,
		PremierRating: premierRating,
		SkillScore:    skillScore,
		HoursPlayed:   hoursPlayed,
		IsBanned:      isBanned,
		PrivacyMode:   privacyMode,
		Rating:        rating,
		FetchedAt:     &now,
	}
	if isBanned && profile != nil {
		for _, b := range profile.Bans {
			player.BanInfo = append(player.BanInfo, models.BanInfo{Platform: b.Platform})
		}
	}

	c.JSON(http.StatusCreated, player)
}

// DELETE /api/tournaments/:id/players/:pid
func (h *Handler) RemovePlayer(c *gin.Context) {
	result, err := h.db.Exec(
		`DELETE FROM players WHERE id=$1 AND tournament_id=$2`,
		c.Param("pid"), c.Param("id"),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "player not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /api/tournaments/:id/players/:pid/refresh
func (h *Handler) RefreshPlayer(c *gin.Context) {
	tournamentID := c.Param("id")
	playerID := c.Param("pid")

	var steam64ID string
	err := h.db.QueryRow(
		`SELECT steam64_id FROM players WHERE id=$1 AND tournament_id=$2`,
		playerID, tournamentID,
	).Scan(&steam64ID)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "player not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	summary, err := h.steam.GetPlayerSummary(steam64ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Could not fetch Steam profile: " + err.Error()})
		return
	}

	profile, leetifyRefreshErr := h.fetchLeetify(steam64ID)
	if leetifyRefreshErr != nil {
		log.Printf("[players] Leetify refresh failed for %s: %v", steam64ID, leetifyRefreshErr)
	}

	steamStats, steamRefreshErr := h.steam.GetCS2Stats(steam64ID)
	if steamRefreshErr != nil {
		log.Printf("[players] Steam stats refresh failed for %s: %v", steam64ID, steamRefreshErr)
	}

	var (
		skillScore    *float64
		isBanned      bool
		privacyMode   = "unknown"
		faceitElo     *int
		faceitLevel   *int
		premierRating *int
		banInfoJSON   []byte
		rawStatsJSON  []byte
	)

	if profile != nil {
		leetifyScore := services.ComputeSkillScore(profile)
		isBanned = len(profile.Bans) > 0
		privacyMode = profile.PrivacyMode

		if profile.Ranks.FaceitElo != nil {
			faceitElo = profile.Ranks.FaceitElo
		}
		if profile.Ranks.Faceit > 0 {
			lvl := profile.Ranks.Faceit
			faceitLevel = &lvl
		}
		if profile.Ranks.Premier > 0 {
			pr := profile.Ranks.Premier
			premierRating = &pr
		}
		if isBanned {
			banInfoJSON, _ = json.Marshal(profile.Bans)
		}
		rawStatsJSON, _ = json.Marshal(profile)

		var finalScore float64
		if leetifyScore >= 0 && steamStats != nil {
			steamScore := services.ComputeSteamScore(steamStats)
			finalScore = leetifyScore*0.90 + steamScore*0.10
		} else {
			finalScore = leetifyScore
		}
		skillScore = &finalScore
	} else if steamStats != nil {
		steamScore := services.ComputeSteamScore(steamStats)
		skillScore = &steamScore
		privacyMode = "steam_only"
	}

	var hoursPlayed *float64
	if steamStats != nil && steamStats.HoursPlayed > 0 {
		hp := steamStats.HoursPlayed
		hoursPlayed = &hp
	}

	now := time.Now()
	_, err = h.db.Exec(`
		UPDATE players SET
			steam_name=$1, avatar_url=$2,
			faceit_elo=$3, faceit_level=$4, premier_rating=$5,
			skill_score=$6, hours_played=$7, is_banned=$8, ban_info=$9,
			privacy_mode=$10, raw_stats=$11, fetched_at=$12
		WHERE id=$13`,
		summary.Name, summary.AvatarURL,
		faceitElo, faceitLevel, premierRating,
		skillScore, hoursPlayed, isBanned, jsonParam(banInfoJSON),
		privacyMode, jsonParam(rawStatsJSON), now,
		playerID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":             playerID,
		"steam_name":     summary.Name,
		"avatar_url":     summary.AvatarURL,
		"skill_score":    skillScore,
		"hours_played":   hoursPlayed,
		"is_banned":      isBanned,
		"privacy_mode":   privacyMode,
		"faceit_elo":     faceitElo,
		"faceit_level":   faceitLevel,
		"premier_rating": premierRating,
		"fetched_at":     now,
	})
}

// loadPlayers fetches all players for a tournament from the DB.
func loadPlayers(db *sql.DB, tournamentID string) ([]models.Player, error) {
	rows, err := db.Query(`
		SELECT id, tournament_id, steam64_id, vanity_url, steam_name, avatar_url,
		       faceit_elo, faceit_level, premier_rating, skill_score, hours_played,
		       is_banned, ban_info, privacy_mode, raw_stats, fetched_at, created_at
		FROM players
		WHERE tournament_id=$1
		ORDER BY skill_score DESC NULLS LAST, created_at ASC`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []models.Player
	for rows.Next() {
		var p models.Player
		var banInfoRaw, rawStatsRaw []byte
		err := rows.Scan(
			&p.ID, &p.TournamentID, &p.Steam64ID, &p.VanityURL, &p.SteamName, &p.AvatarURL,
			&p.FaceitElo, &p.FaceitLevel, &p.PremierRating, &p.SkillScore, &p.HoursPlayed,
			&p.IsBanned, &banInfoRaw, &p.PrivacyMode, &rawStatsRaw, &p.FetchedAt, &p.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if banInfoRaw != nil {
			json.Unmarshal(banInfoRaw, &p.BanInfo)
		}
		if rawStatsRaw != nil {
			var lp services.LeetifyProfile
			if json.Unmarshal(rawStatsRaw, &lp) == nil && lp.PrivacyMode == "public" {
				p.Rating = &models.Rating{
					Aim:         lp.Rating.Aim,
					Positioning: lp.Rating.Positioning,
					Utility:     lp.Rating.Utility,
					Clutch:      lp.Rating.Clutch,
				}
			}
		}
		players = append(players, p)
	}
	if players == nil {
		players = []models.Player{}
	}
	return players, nil
}
