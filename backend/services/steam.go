package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SteamService struct {
	APIKey string
	client *http.Client
}

func NewSteamService(apiKey string) *SteamService {
	return &SteamService{
		APIKey: apiKey,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

type PlayerSummary struct {
	Steam64ID string `json:"steamid"`
	Name      string `json:"personaname"`
	AvatarURL string `json:"avatarfull"`
}

func (s *SteamService) ResolveVanityURL(vanityURL string) (string, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/ISteamUser/ResolveVanityURL/v1/?key=%s&vanityurl=%s",
		s.APIKey, vanityURL,
	)
	resp, err := s.client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Response struct {
			SteamID string `json:"steamid"`
			Success int    `json:"success"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Response.Success != 1 {
		return "", fmt.Errorf("vanity URL not found")
	}
	return result.Response.SteamID, nil
}

func (s *SteamService) GetPlayerSummary(steam64ID string) (*PlayerSummary, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s",
		s.APIKey, steam64ID,
	)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Response struct {
			Players []PlayerSummary `json:"players"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Response.Players) == 0 {
		return nil, fmt.Errorf("player not found")
	}
	return &result.Response.Players[0], nil
}

// CS2Stats holds derived metrics from GetUserStatsForGame for CS2 (appid=730).
type CS2Stats struct {
	KDRatio    float64
	HSPercent  float64
	Accuracy   float64
	WinRate    float64
	MVPRate    float64
	TotalKills int64
}

// GetCS2Stats fetches lifetime stats from the Steam API for CS2 and derives
// KD ratio, headshot %, accuracy, win rate, and MVP rate.
// Returns nil (not an error) when the profile is private or has no CS2 stats.
func (s *SteamService) GetCS2Stats(steam64ID string) (*CS2Stats, error) {
	url := fmt.Sprintf(
		"https://api.steampowered.com/ISteamUserStats/GetUserStatsForGame/v2/?appid=730&key=%s&steamid=%s",
		s.APIKey, steam64ID,
	)
	resp, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 403 = private profile, 400/404 = no CS2 stats — not an error we surface
	if resp.StatusCode != 200 {
		return nil, nil
	}

	var result struct {
		PlayerStats struct {
			Stats []struct {
				Name  string `json:"name"`
				Value int64  `json:"value"`
			} `json:"stats"`
		} `json:"playerstats"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	m := make(map[string]int64, len(result.PlayerStats.Stats))
	for _, s := range result.PlayerStats.Stats {
		m[s.Name] = s.Value
	}

	totalKills := m["total_kills"]
	totalDeaths := m["total_deaths"]
	totalHSKills := m["total_kills_headshot"]
	totalShotsHit := m["total_shots_hit"]
	totalShotsFired := m["total_shots_fired"]
	totalWins := m["total_matches_won"]
	totalMatches := m["total_matches_played"]
	totalMVPs := m["total_mvps"]

	// Require at least some matches to have meaningful stats
	if totalMatches < 10 {
		return nil, nil
	}

	var kd, hs, acc, wr, mvpr float64
	if totalDeaths > 0 {
		kd = float64(totalKills) / float64(totalDeaths)
	}
	if totalKills > 0 {
		hs = float64(totalHSKills) / float64(totalKills) * 100
	}
	if totalShotsFired > 0 {
		acc = float64(totalShotsHit) / float64(totalShotsFired) * 100
	}
	if totalMatches > 0 {
		wr = float64(totalWins) / float64(totalMatches) * 100
		mvpr = float64(totalMVPs) / float64(totalMatches)
	}

	return &CS2Stats{
		KDRatio:    kd,
		HSPercent:  hs,
		Accuracy:   acc,
		WinRate:    wr,
		MVPRate:    mvpr,
		TotalKills: totalKills,
	}, nil
}

// ParseSteamInput handles three input formats:
//
//	https://steamcommunity.com/id/vanityname     → resolve vanity URL
//	https://steamcommunity.com/profiles/76561... → extract ID directly
//	76561...                                     → use directly
func (s *SteamService) ParseSteamInput(input string) (steam64ID string, vanityURL string, err error) {
	input = strings.TrimSpace(input)

	if strings.Contains(input, "/profiles/") {
		parts := strings.Split(input, "/profiles/")
		steam64ID = strings.Trim(parts[len(parts)-1], "/")
		return steam64ID, "", nil
	}

	if strings.Contains(input, "/id/") {
		parts := strings.Split(input, "/id/")
		vanityURL = strings.Trim(parts[len(parts)-1], "/")
		steam64ID, err = s.ResolveVanityURL(vanityURL)
		return steam64ID, vanityURL, err
	}

	// Assume raw Steam64 ID (17-digit number)
	if len(input) > 10 {
		return input, "", nil
	}

	return "", "", fmt.Errorf("could not parse steam input: %s", input)
}
