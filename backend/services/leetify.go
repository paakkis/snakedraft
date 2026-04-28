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
}

func FetchLeetifyProfile(steam64ID, apiKey string) (*LeetifyProfile, error) {
	url := fmt.Sprintf("https://api.cs-prod.leetify.com/api/profile/id/%s", steam64ID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

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
	return &profile, nil
}
