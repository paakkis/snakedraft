package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// steamBaseURL is a variable so tests can point it at a fake server.
var steamBaseURL = "https://api.steampowered.com"

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
		steamBaseURL+"/ISteamUser/ResolveVanityURL/v1/?key=%s&vanityurl=%s",
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
		steamBaseURL+"/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s",
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
	KDRatio    float64 `json:"kd_ratio"`
	HSPercent  float64 `json:"hs_percent"`
	Accuracy   float64 `json:"accuracy"`
	WinRate    float64 `json:"win_rate"`
	MVPRate    float64 `json:"mvp_rate"`
	TotalKills int64   `json:"total_kills"`
	// HoursPlayed is the playtime Steam shows on the profile when available
	// (see GetCS2Playtime); otherwise the time spent inside matches, which is
	// roughly half of that.
	HoursPlayed float64 `json:"hours_played"`
	// HoursLast2Weeks is only known when the profile playtime is.
	HoursLast2Weeks float64 `json:"hours_last_2_weeks"`
}

// GetCS2Stats fetches lifetime stats from the Steam API for CS2 and derives
// KD ratio, headshot %, accuracy, win rate, and MVP rate.
// Returns nil (not an error) when the profile is private or has no CS2 stats.
func (s *SteamService) GetCS2Stats(steam64ID string) (*CS2Stats, error) {
	url := fmt.Sprintf(
		steamBaseURL+"/ISteamUserStats/GetUserStatsForGame/v2/?appid=730&key=%s&steamid=%s",
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
	totalTimePlayed := m["total_time_played"] // in seconds

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
	hoursPlayed := float64(totalTimePlayed) / 3600.0

	return &CS2Stats{
		KDRatio:     kd,
		HSPercent:   hs,
		Accuracy:    acc,
		WinRate:     wr,
		MVPRate:     mvpr,
		TotalKills:  totalKills,
		HoursPlayed: hoursPlayed,
	}, nil
}

// GetCS2Playtime returns the CS2 playtime exactly as shown on the Steam
// profile (total and last two weeks, in hours). ok is false when the
// player's game details are private.
//
// This is not the same number as total_time_played in GetUserStatsForGame,
// which only counts time spent inside matches.
func (s *SteamService) GetCS2Playtime(steam64ID string) (total, last2Weeks float64, ok bool, err error) {
	url := fmt.Sprintf(
		steamBaseURL+"/IPlayerService/GetOwnedGames/v1/?key=%s&steamid=%s&include_played_free_games=1&appids_filter[0]=730",
		s.APIKey, steam64ID,
	)
	resp, err := s.client.Get(url)
	if err != nil {
		return 0, 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, 0, false, nil
	}

	var result struct {
		Response struct {
			Games []struct {
				AppID           int     `json:"appid"`
				PlaytimeForever float64 `json:"playtime_forever"` // minutes
				Playtime2Weeks  float64 `json:"playtime_2weeks"`  // minutes
			} `json:"games"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, false, err
	}
	for _, g := range result.Response.Games {
		if g.AppID == 730 && g.PlaytimeForever > 0 {
			return g.PlaytimeForever / 60, g.Playtime2Weeks / 60, true, nil
		}
	}
	return 0, 0, false, nil
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
		if !isSteam64ID(steam64ID) {
			return "", "", fmt.Errorf("%q is not a Steam64 ID — custom profile names use /id/ instead of /profiles/", steam64ID)
		}
		return steam64ID, "", nil
	}

	if strings.Contains(input, "/id/") {
		parts := strings.Split(input, "/id/")
		vanityURL = strings.Trim(parts[len(parts)-1], "/")
		steam64ID, err = s.ResolveVanityURL(vanityURL)
		return steam64ID, vanityURL, err
	}

	if isSteam64ID(input) {
		return input, "", nil
	}

	return "", "", fmt.Errorf("could not parse steam input: %s", input)
}

// isSteam64ID reports whether s looks like a 17-digit Steam64 ID.
func isSteam64ID(s string) bool {
	if len(s) != 17 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
