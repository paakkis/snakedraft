package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// faceitBaseURL is a variable so tests can point it at a fake server.
var faceitBaseURL = "https://open.faceit.com/data/v4"

// FaceitRecentMatches is how many of a player's latest matches are summarised.
const FaceitRecentMatches = 30

// FaceitPlayer is a player's current CS2 standing on Faceit, straight from
// the Faceit Data API. It is more up to date than the Faceit rank Leetify
// reports, which is often stale or missing the ELO.
type FaceitPlayer struct {
	PlayerID string `json:"player_id"`
	Nickname string `json:"nickname"`
	Elo      int    `json:"elo"`
	Level    int    `json:"level"`
	// Recent is nil when the player has no recent CS2 matches on Faceit.
	Recent *FaceitRecent `json:"recent,omitempty"`
}

// FaceitRecent summarises a player's latest Faceit matches.
type FaceitRecent struct {
	Matches     int        `json:"matches"`
	KDRatio     float64    `json:"kd_ratio"` // total kills / total deaths
	ADR         float64    `json:"adr"`
	HSPercent   float64    `json:"hs_percent"`
	WinRate     float64    `json:"win_rate"` // 0–100
	LastMatchAt *time.Time `json:"last_match_at,omitempty"`
}

type FaceitService struct {
	APIKey string
	client *http.Client
}

func NewFaceitService(apiKey string) *FaceitService {
	return &FaceitService{APIKey: apiKey, client: &http.Client{Timeout: 10 * time.Second}}
}

// Enabled reports whether a Faceit API key is configured.
func (s *FaceitService) Enabled() bool { return s != nil && s.APIKey != "" }

// GetPlayer looks a player up by Steam64 ID. It returns nil (not an error)
// when no key is configured or the player has no CS2 profile on Faceit.
func (s *FaceitService) GetPlayer(steam64ID string) (*FaceitPlayer, error) {
	if !s.Enabled() {
		return nil, nil
	}

	var profile struct {
		PlayerID string `json:"player_id"`
		Nickname string `json:"nickname"`
		Games    map[string]struct {
			SkillLevel int `json:"skill_level"`
			FaceitElo  int `json:"faceit_elo"`
		} `json:"games"`
	}
	found, err := s.get("/players?game=cs2&game_player_id="+url.QueryEscape(steam64ID), &profile)
	if err != nil || !found {
		return nil, err
	}
	cs2, ok := profile.Games["cs2"]
	if !ok || cs2.FaceitElo <= 0 {
		return nil, nil
	}
	player := &FaceitPlayer{
		PlayerID: profile.PlayerID,
		Nickname: profile.Nickname,
		Elo:      cs2.FaceitElo,
		Level:    cs2.SkillLevel,
	}

	// Recent form is a bonus: the ELO is still useful if this call fails.
	if recent, err := s.recentStats(profile.PlayerID); err == nil {
		player.Recent = recent
	}
	return player, nil
}

func (s *FaceitService) recentStats(playerID string) (*FaceitRecent, error) {
	var result struct {
		Items []struct {
			Stats map[string]any `json:"stats"`
		} `json:"items"`
	}
	path := fmt.Sprintf("/players/%s/games/cs2/stats?limit=%d", url.PathEscape(playerID), FaceitRecentMatches)
	found, err := s.get(path, &result)
	if err != nil || !found || len(result.Items) == 0 {
		return nil, err
	}

	// Faceit sends most numbers as strings ("Kills": "17") and some as numbers.
	num := func(stats map[string]any, key string) float64 {
		switch v := stats[key].(type) {
		case string:
			f, _ := strconv.ParseFloat(v, 64)
			return f
		case float64:
			return v
		}
		return 0
	}

	recent := &FaceitRecent{Matches: len(result.Items)}
	var kills, deaths, headshots, adrSum, wins, latest float64
	for _, item := range result.Items {
		kills += num(item.Stats, "Kills")
		deaths += num(item.Stats, "Deaths")
		headshots += num(item.Stats, "Headshots")
		adrSum += num(item.Stats, "ADR")
		wins += num(item.Stats, "Result")
		if at := num(item.Stats, "Match Finished At"); at > latest {
			latest = at
		}
	}
	if deaths > 0 {
		recent.KDRatio = kills / deaths
	} else {
		recent.KDRatio = kills
	}
	if kills > 0 {
		recent.HSPercent = headshots / kills * 100
	}
	recent.ADR = adrSum / float64(recent.Matches)
	recent.WinRate = wins / float64(recent.Matches) * 100
	if latest > 0 {
		t := time.UnixMilli(int64(latest)).UTC()
		recent.LastMatchAt = &t
	}
	return recent, nil
}

// get fetches a Data API path into out. found is false on 404.
func (s *FaceitService) get(path string, out any) (found bool, err error) {
	req, err := http.NewRequest(http.MethodGet, faceitBaseURL+path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("faceit API error: %d", resp.StatusCode)
	}
	return true, json.NewDecoder(resp.Body).Decode(out)
}
