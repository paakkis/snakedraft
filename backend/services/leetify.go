package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type LeetifyProfile struct {
	PrivacyMode  string  `json:"privacy_mode"`
	Name         string  `json:"name"`
	Steam64ID    string  `json:"steam64_id"`
	Winrate      float64 `json:"winrate"`
	TotalMatches int     `json:"total_matches"`
	Bans         []struct {
		Platform string `json:"platform"`
	} `json:"bans"`
	Ranks struct {
		Leetify   float64 `json:"leetify"`
		Premier   int     `json:"premier"`
		Faceit    int     `json:"faceit"`
		FaceitElo *int    `json:"faceit_elo"` // nullable — pointer required
	} `json:"ranks"`
	Rating struct {
		Aim         float64 `json:"aim"`
		Positioning float64 `json:"positioning"`
		Utility     float64 `json:"utility"`
		Clutch      float64 `json:"clutch"`
		Opening     float64 `json:"opening"`
		CtLeetify   float64 `json:"ct_leetify"`
		TLeetify    float64 `json:"t_leetify"`
	} `json:"rating"`
	Stats struct {
		AccuracyEnemySpotted           float64 `json:"accuracy_enemy_spotted"`
		AccuracyHead                   float64 `json:"accuracy_head"`
		CtOpeningDuelSuccessPercentage float64 `json:"ct_opening_duel_success_percentage"`
		TOpeningDuelSuccessPercentage  float64 `json:"t_opening_duel_success_percentage"`
		ReactionTimeMs                 float64 `json:"reaction_time_ms"`
		TradeKillsSuccessPercentage    float64 `json:"trade_kills_success_percentage"`
	} `json:"stats"`
	RecentMatches []RecentMatch `json:"recent_matches"`
}

type RecentMatch struct {
	LeetifyRating float64 `json:"leetify_rating"`
	FinishedAt    string  `json:"finished_at"`
	Outcome       string  `json:"outcome"`
	DataSource    string  `json:"data_source"`
	// Rank is the player's rank after the match; its meaning depends on
	// RankType (Premier rating, competitive skill group, Faceit level…).
	Rank     int  `json:"rank"`
	RankType *int `json:"rank_type"`
}

// leetifyRankTypePremier marks a recent match whose Rank is a Premier rating.
const leetifyRankTypePremier = 11

// lastRatedPremierMatch returns the player's most recent Premier match that
// produced a rating, or nil if there is none among the recent matches.
func lastRatedPremierMatch(matches []RecentMatch) *RecentMatch {
	var last *RecentMatch
	for i, m := range matches {
		if m.RankType == nil || *m.RankType != leetifyRankTypePremier || m.Rank <= 0 {
			continue
		}
		if last == nil || m.FinishedAt > last.FinishedAt { // ISO timestamps sort as text
			last = &matches[i]
		}
	}
	return last
}

// lastKnownPremier returns the Premier rating after the player's most recent
// rated Premier match, or 0 if there is none among the recent matches.
func lastKnownPremier(matches []RecentMatch) int {
	if m := lastRatedPremierMatch(matches); m != nil {
		return m.Rank
	}
	return 0
}

// PremierRatedAt returns when the player last earned a Premier rating, so a
// stale rating can be flagged. ok is false when that is unknown (no rated
// Premier match among the recent matches Leetify returns).
func PremierRatedAt(p *LeetifyProfile) (at time.Time, ok bool) {
	m := lastRatedPremierMatch(p.RecentMatches)
	if m == nil {
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, m.FinishedAt)
	return at, err == nil
}

// leetifyBaseURL is a variable so tests can point it at a fake server.
var leetifyBaseURL = "https://api-public.cs-prod.leetify.com"

func FetchLeetifyProfile(steam64ID, apiKey string) (*LeetifyProfile, error) {
	// Leetify public API v3 — https://api-public-docs.cs-prod.leetify.com
	// (the old api.cs-prod.leetify.com/api/profile/id/… route now 404s for everyone).
	url := fmt.Sprintf("%s/v3/profile?steam64_id=%s", leetifyBaseURL, steam64ID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		// The key only raises rate limits; the endpoint also works without one.
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("player not found on Leetify")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("leetify API error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decode a partial struct just to log the privacy_mode value so we can
	// diagnose mismatches between expected and actual field values.
	var partial struct {
		PrivacyMode string `json:"privacy_mode"`
	}
	json.Unmarshal(body, &partial)
	log.Printf("[leetify] steam64=%s privacy_mode=%q", steam64ID, partial.PrivacyMode)

	var profile LeetifyProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, err
	}
	// Leetify reports no current Premier rating while a player is unranked in
	// the running season (e.g. placements not finished). Like Leetify's own
	// site, fall back to the rating from their last rated Premier match.
	if profile.Ranks.Premier <= 0 {
		profile.Ranks.Premier = lastKnownPremier(profile.RecentMatches)
	}
	return &profile, nil
}
