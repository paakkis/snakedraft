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

// marshalOrNil marshals v, or returns nil for a nil pointer so the column
// is stored as SQL NULL.
func marshalOrNil[T any](v *T) []byte {
	if v == nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}

// playerStats is everything fetched about a player from the stat sources,
// plus the values derived from it. Each source may be nil.
type playerStats struct {
	leetify *services.LeetifyProfile
	steam   *services.CS2Stats
	faceit  *services.FaceitPlayer

	skillScore    *float64
	privacyMode   string
	faceitElo     *int
	faceitLevel   *int
	premierRating *int
	hoursPlayed   *float64
	isBanned      bool
	bans          []models.BanInfo
}

// fetchPlayerStats queries Leetify, Steam and Faceit for a player. A failing
// source is logged and skipped: the score is built from whatever answered.
func (h *Handler) fetchPlayerStats(steam64ID string) playerStats {
	var s playerStats
	var err error

	if s.leetify, err = h.fetchLeetify(steam64ID); err != nil {
		log.Printf("[players] Leetify fetch failed for %s: %v", steam64ID, err)
	}
	if s.steam, err = h.steam.GetCS2Stats(steam64ID); err != nil {
		log.Printf("[players] Steam stats fetch failed for %s: %v", steam64ID, err)
	}
	// Prefer the playtime Steam shows on the profile over in-match time
	if total, recent, ok, err := h.steam.GetCS2Playtime(steam64ID); err != nil {
		log.Printf("[players] Steam playtime fetch failed for %s: %v", steam64ID, err)
	} else if ok {
		s.hoursPlayed = &total
		if s.steam != nil {
			s.steam.HoursPlayed, s.steam.HoursLast2Weeks = total, recent
		}
	}
	if s.faceit, err = h.faceit.GetPlayer(steam64ID); err != nil {
		log.Printf("[players] Faceit fetch failed for %s: %v", steam64ID, err)
	}

	s.skillScore, s.privacyMode = services.CombineScores(s.leetify, s.steam, s.faceit)

	if p := s.leetify; p != nil {
		for _, b := range p.Bans {
			s.bans = append(s.bans, models.BanInfo{Platform: b.Platform})
		}
		s.isBanned = len(p.Bans) > 0
		if p.Ranks.FaceitElo != nil {
			s.faceitElo = p.Ranks.FaceitElo
		}
		if p.Ranks.Faceit > 0 {
			lvl := p.Ranks.Faceit
			s.faceitLevel = &lvl
		}
		if p.Ranks.Premier > 0 {
			pr := p.Ranks.Premier
			s.premierRating = &pr
		}
	}
	// Faceit's own numbers win over Leetify's copy of them
	if f := s.faceit; f != nil {
		s.faceitElo, s.faceitLevel = &f.Elo, &f.Level
	}
	if s.hoursPlayed == nil && s.steam != nil && s.steam.HoursPlayed > 0 {
		s.hoursPlayed = &s.steam.HoursPlayed
	}
	return s
}

func (s *playerStats) banInfoJSON() []byte {
	if len(s.bans) == 0 {
		return nil
	}
	b, _ := json.Marshal(s.bans)
	return b
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
	stats := h.fetchPlayerStats(steam64ID)

	now := time.Now()
	var playerID string
	err = h.db.QueryRow(`
		INSERT INTO players (
			tournament_id, steam64_id, vanity_url, steam_name, avatar_url,
			faceit_elo, faceit_level, premier_rating,
			skill_score, hours_played, is_banned, ban_info, privacy_mode, raw_stats, fetched_at,
			steam_stats, faceit_stats
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id`,
		tournamentID, steam64ID, vanityURL, summary.Name, summary.AvatarURL,
		stats.faceitElo, stats.faceitLevel, stats.premierRating,
		stats.skillScore, stats.hoursPlayed, stats.isBanned, jsonParam(stats.banInfoJSON()),
		stats.privacyMode, jsonParam(marshalOrNil(stats.leetify)), now,
		jsonParam(marshalOrNil(stats.steam)), jsonParam(marshalOrNil(stats.faceit)),
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
		FaceitElo:     stats.faceitElo,
		FaceitLevel:   stats.faceitLevel,
		PremierRating: stats.premierRating,
		SkillScore:    stats.skillScore,
		HoursPlayed:   stats.hoursPlayed,
		IsBanned:      stats.isBanned,
		BanInfo:       stats.bans,
		PrivacyMode:   stats.privacyMode,
		FetchedAt:     &now,
	}
	fillDetails(&player, stats.leetify, stats.steam, stats.faceit)

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

	stats := h.fetchPlayerStats(steam64ID)

	now := time.Now()
	_, err = h.db.Exec(`
		UPDATE players SET
			steam_name=$1, avatar_url=$2,
			faceit_elo=$3, faceit_level=$4, premier_rating=$5,
			skill_score=$6, hours_played=$7, is_banned=$8, ban_info=$9,
			privacy_mode=$10, raw_stats=$11, fetched_at=$12,
			steam_stats=$14, faceit_stats=$15
		WHERE id=$13`,
		summary.Name, summary.AvatarURL,
		stats.faceitElo, stats.faceitLevel, stats.premierRating,
		stats.skillScore, stats.hoursPlayed, stats.isBanned, jsonParam(stats.banInfoJSON()),
		stats.privacyMode, jsonParam(marshalOrNil(stats.leetify)), now,
		playerID, jsonParam(marshalOrNil(stats.steam)), jsonParam(marshalOrNil(stats.faceit)),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// A changed score changes the average of the team the player is on
	h.db.Exec(`
		UPDATE teams t SET avg_skill_score = COALESCE((
			SELECT AVG(p.skill_score)
			FROM team_players tp JOIN players p ON p.id = tp.player_id
			WHERE tp.team_id = t.id
		), 0)
		WHERE t.id IN (SELECT team_id FROM team_players WHERE player_id = $1)`, playerID)

	c.JSON(http.StatusOK, gin.H{
		"id":             playerID,
		"steam_name":     summary.Name,
		"avatar_url":     summary.AvatarURL,
		"skill_score":    stats.skillScore,
		"hours_played":   stats.hoursPlayed,
		"is_banned":      stats.isBanned,
		"privacy_mode":   stats.privacyMode,
		"faceit_elo":     stats.faceitElo,
		"faceit_level":   stats.faceitLevel,
		"premier_rating": stats.premierRating,
		"fetched_at":     now,
	})
}

// loadPlayers fetches all players for a tournament from the DB.
func loadPlayers(db *sql.DB, tournamentID string) ([]models.Player, error) {
	rows, err := db.Query(`
		SELECT id, tournament_id, steam64_id, vanity_url, steam_name, avatar_url,
		       faceit_elo, faceit_level, premier_rating, skill_score, hours_played,
		       is_banned, ban_info, privacy_mode, raw_stats, fetched_at, created_at,
		       steam_stats, faceit_stats
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
		var banInfoRaw, leetifyRaw, steamRaw, faceitRaw []byte
		err := rows.Scan(
			&p.ID, &p.TournamentID, &p.Steam64ID, &p.VanityURL, &p.SteamName, &p.AvatarURL,
			&p.FaceitElo, &p.FaceitLevel, &p.PremierRating, &p.SkillScore, &p.HoursPlayed,
			&p.IsBanned, &banInfoRaw, &p.PrivacyMode, &leetifyRaw, &p.FetchedAt, &p.CreatedAt,
			&steamRaw, &faceitRaw,
		)
		if err != nil {
			return nil, err
		}
		if banInfoRaw != nil {
			json.Unmarshal(banInfoRaw, &p.BanInfo)
		}
		fillDetails(&p,
			unmarshalOrNil[services.LeetifyProfile](leetifyRaw),
			unmarshalOrNil[services.CS2Stats](steamRaw),
			unmarshalOrNil[services.FaceitPlayer](faceitRaw),
		)
		players = append(players, p)
	}
	if players == nil {
		players = []models.Player{}
	}
	return players, nil
}

func unmarshalOrNil[T any](raw []byte) *T {
	if raw == nil {
		return nil
	}
	var v T
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return &v
}

// fillDetails sets the display-only metadata on a player from whichever
// sources are available (each may be nil).
func fillDetails(p *models.Player, profile *services.LeetifyProfile, steamStats *services.CS2Stats, faceit *services.FaceitPlayer) {
	if profile != nil && profile.PrivacyMode == "public" {
		p.Rating = &models.Rating{
			Aim:         profile.Rating.Aim,
			Positioning: profile.Rating.Positioning,
			Utility:     profile.Rating.Utility,
			Clutch:      profile.Rating.Clutch,
		}
		if at, ok := services.PremierRatedAt(profile); ok && p.PremierRating != nil {
			p.PremierRatedAt = &at
		}
		p.Leetify = &models.LeetifyStats{
			Rating:         profile.Ranks.Leetify,
			Winrate:        profile.Winrate * 100,
			TotalMatches:   profile.TotalMatches,
			ReactionTimeMs: profile.Stats.ReactionTimeMs,
			HeadAccuracy:   profile.Stats.AccuracyHead,
			OpeningDuels: (profile.Stats.CtOpeningDuelSuccessPercentage +
				profile.Stats.TOpeningDuelSuccessPercentage) / 2,
		}
	}
	if steamStats != nil {
		p.Steam = &models.SteamStats{
			KDRatio:         steamStats.KDRatio,
			HSPercent:       steamStats.HSPercent,
			Accuracy:        steamStats.Accuracy,
			WinRate:         steamStats.WinRate,
			HoursLast2Weeks: steamStats.HoursLast2Weeks,
		}
	}
	if faceit != nil {
		p.Faceit = &models.FaceitStats{Nickname: faceit.Nickname}
		if r := faceit.Recent; r != nil {
			p.Faceit.RecentMatches = r.Matches
			p.Faceit.RecentKD = r.KDRatio
			p.Faceit.RecentADR = r.ADR
			p.Faceit.RecentHS = r.HSPercent
			p.Faceit.RecentWinRate = r.WinRate
			p.Faceit.LastMatchAt = r.LastMatchAt
		}
	}
}
