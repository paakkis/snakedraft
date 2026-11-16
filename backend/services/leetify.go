package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
		FaceitElo *int    `json:"faceit_elo"` // nullable - pointer required
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

	// Partial marks a profile that the public API does not have and that
	// was put together from what the Leetify website shows instead (see
	// fetchLeetifyWebProfile). It has no positioning rating, no opening
	// duel percentages and no list of matches.
	Partial bool `json:"partial,omitempty"`
	// MatchmakingPlayedAt is when the player last played matchmaking. It is
	// only set on a partial profile, where it stands in for the date of the
	// last Premier match.
	MatchmakingPlayedAt *time.Time `json:"matchmaking_played_at,omitempty"`
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
		if p.MatchmakingPlayedAt != nil && p.Ranks.Premier > 0 {
			return *p.MatchmakingPlayedAt, true
		}
		return time.Time{}, false
	}
	at, err := time.Parse(time.RFC3339, m.FinishedAt)
	return at, err == nil
}

// leetifyBaseURL is a variable so tests can point it at a fake server.
var leetifyBaseURL = "https://api-public.cs-prod.leetify.com"

// errLeetifyNotFound is returned when Leetify has nothing on a player.
var errLeetifyNotFound = errors.New("player not found on Leetify")

// FetchLeetifyProfile fetches a player's profile from the Leetify public API.
// When that API does not know the player, it falls back to the data behind
// the Leetify website, which also covers players who never signed up there.
func FetchLeetifyProfile(steam64ID, apiKey string) (*LeetifyProfile, error) {
	profile, err := fetchLeetifyPublicProfile(steam64ID, apiKey)
	if !errors.Is(err, errLeetifyNotFound) {
		return profile, err
	}
	web, webErr := fetchLeetifyWebProfile(steam64ID)
	if webErr != nil {
		log.Printf("[leetify] steam64=%s website fallback failed: %v", steam64ID, webErr)
		return nil, err
	}
	log.Printf("[leetify] steam64=%s not in the public API, using the website's data", steam64ID)
	return web, nil
}

func fetchLeetifyPublicProfile(steam64ID, apiKey string) (*LeetifyProfile, error) {
	// Leetify public API v3 - https://api-public-docs.cs-prod.leetify.com
	// (the old api.cs-prod.leetify.com/api/profile/id/… route now 404s for everyone).
	endpoint := fmt.Sprintf("%s/v3/profile?steam64_id=%s", leetifyBaseURL, steam64ID)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
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
		return nil, errLeetifyNotFound
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

// leetifyWebBaseURL is the API behind leetify.com. It is not documented and
// not meant for outside use: it can change or start refusing requests at any
// time, so everything read from it is optional. A variable so tests can point
// it at a fake server.
var leetifyWebBaseURL = "https://api.cs-prod.leetify.com"

// fetchLeetifyWebProfile builds a partial profile from the two requests the
// Leetify profile page makes: the player's ranks, and their averages over
// the last 30 5v5 matches. Both must succeed.
func fetchLeetifyWebProfile(steam64ID string) (*LeetifyProfile, error) {
	var sidebar struct {
		Platforms map[string]struct {
			LatestGameFinishedAt *time.Time `json:"latestGameFinishedAt"`
			LatestElo            *int       `json:"latestElo"`
			BannedAt             *string    `json:"bannedAt"`
		} `json:"platforms"`
		Ranks []struct {
			MapName    *string `json:"mapName"`
			DataSource string  `json:"dataSource"`
			Latest     int     `json:"latest"`
		} `json:"ranks"`
	}
	var games struct {
		MatchesPlayed        int     `json:"matchesPlayed"`
		WinRate              float64 `json:"winRate"` // 0–1
		AimRating            float64 `json:"aimRating"`
		UtilityRating        float64 `json:"utilityRating"`
		LeetifyRating        float64 `json:"leetifyRating"`
		CtLeetifyRating      float64 `json:"ctLeetifyRating"`
		TLeetifyRating       float64 `json:"tLeetifyRating"`
		AccuracyEnemySpotted float64 `json:"accuracyEnemySpotted"`
		AccuracyHead         float64 `json:"accuracyHead"`
		ReactionTime         float64 `json:"reactionTime"`
	}
	base := leetifyWebBaseURL + "/api/profile/" + url.PathEscape(steam64ID)
	if err := getLeetifyWeb(base+"/sidebar-games", &sidebar); err != nil {
		return nil, err
	}
	if err := getLeetifyWeb(base+"/recent-games/5v5", &games); err != nil {
		return nil, err
	}
	if games.MatchesPlayed <= 0 {
		return nil, errLeetifyNotFound
	}

	p := &LeetifyProfile{
		PrivacyMode:  "public",
		Steam64ID:    steam64ID,
		Partial:      true,
		Winrate:      games.WinRate,
		TotalMatches: games.MatchesPlayed,
	}
	p.Rating.Aim = games.AimRating
	p.Rating.Utility = games.UtilityRating
	p.Rating.CtLeetify, p.Rating.TLeetify = games.CtLeetifyRating, games.TLeetifyRating
	p.Ranks.Leetify = games.LeetifyRating * 100
	p.Stats.AccuracyEnemySpotted = games.AccuracyEnemySpotted
	p.Stats.AccuracyHead = games.AccuracyHead
	p.Stats.ReactionTimeMs = games.ReactionTime
	// The matches are not listed one by one. Their average rating goes in as
	// a single match, so that the recent form has something to work with.
	// It has no source or rank type, so it does not count as a Faceit or
	// Premier match anywhere.
	p.RecentMatches = []RecentMatch{{LeetifyRating: games.LeetifyRating}}

	for _, r := range sidebar.Ranks {
		if r.MapName != nil || r.Latest <= 0 {
			continue // per-map competitive ranks
		}
		switch r.DataSource {
		case "matchmaking":
			p.Ranks.Premier = r.Latest
		case "faceit":
			p.Ranks.Faceit = r.Latest
		}
	}
	if f, ok := sidebar.Platforms["faceit"]; ok && f.LatestElo != nil && *f.LatestElo > 0 {
		p.Ranks.FaceitElo = f.LatestElo
	}
	if m, ok := sidebar.Platforms["matchmaking"]; ok {
		p.MatchmakingPlayedAt = m.LatestGameFinishedAt
	}
	for name, platform := range sidebar.Platforms {
		if platform.BannedAt != nil && *platform.BannedAt != "" {
			p.Bans = append(p.Bans, struct {
				Platform string `json:"platform"`
			}{Platform: name})
		}
	}
	return p, nil
}

func getLeetifyWeb(endpoint string, out any) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("leetify website API error: %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
