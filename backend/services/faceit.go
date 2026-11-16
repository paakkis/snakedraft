package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

// faceitBaseURL is a variable so tests can point it at a fake server.
var faceitBaseURL = "https://open.faceit.com/data/v4"

// FaceitRecentMatches is how many of a player's latest matches are summarised.
const FaceitRecentMatches = 30

// FaceitPerformanceMatches is how many of a player's latest matches are
// looked up one by one to compare their performance with the lobby's ELO.
// Each one costs a request.
const FaceitPerformanceMatches = 10

// FaceitPlayer is a player's current CS2 standing on Faceit, straight from
// the Faceit Data API. It is more up to date than the Faceit rank Leetify
// reports, which is often stale or missing the ELO.
type FaceitPlayer struct {
	PlayerID string `json:"player_id"`
	Nickname string `json:"nickname"`
	Elo      int    `json:"elo"`
	Level    int    `json:"level"`
	// LifetimeMatches is how many CS2 matches the ELO was earned over;
	// 0 when unknown.
	LifetimeMatches int `json:"lifetime_matches,omitempty"`
	// Recent is nil when the player has no recent CS2 matches on Faceit.
	Recent *FaceitRecent `json:"recent,omitempty"`
	// Performance is nil when no lobby could be looked up.
	Performance *FaceitPerformance `json:"performance,omitempty"`
}

// FaceitPerformance compares how a player did in their latest matches with
// how strong those lobbies were.
type FaceitPerformance struct {
	Matches int `json:"matches"`
	// ImpliedElo is the ELO the player performed like (see impliedElo).
	ImpliedElo float64 `json:"implied_elo"`
	// LobbyElo is the average ELO of the lobbies, both teams included.
	LobbyElo float64 `json:"lobby_elo"`
}

// faceitMatch is one match from a player's point of view.
type faceitMatch struct {
	id         string
	finishedAt float64 // unix milliseconds
	kd, adr    float64
	lobbyElo   float64 // 0 until looked up, or when unknown
}

// FaceitRecent summarises a player's latest Faceit matches.
type FaceitRecent struct {
	Matches int     `json:"matches"`
	KDRatio float64 `json:"kd_ratio"` // total kills / total deaths
	// ADR is averaged over the matches Faceit has one for; 0 when none.
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

	// Both of these are a bonus: the ELO is still useful if a call fails.
	if n, err := s.lifetimeMatches(profile.PlayerID); err == nil {
		player.LifetimeMatches = n
	}
	if recent, matches, err := s.recentStats(profile.PlayerID); err == nil {
		player.Recent = recent
		player.Performance = s.performance(matches)
	}
	return player, nil
}

// performance looks up the lobbies of a player's latest matches and returns
// what ELO they performed like, or nil when no lobby is known.
func (s *FaceitService) performance(matches []faceitMatch) *FaceitPerformance {
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].finishedAt > matches[j].finishedAt })
	if len(matches) > FaceitPerformanceMatches {
		matches = matches[:FaceitPerformanceMatches]
	}

	var wg sync.WaitGroup
	slots := make(chan struct{}, 5) // be gentle with the API
	for i := range matches {
		if matches[i].id == "" {
			continue
		}
		wg.Add(1)
		go func(m *faceitMatch) {
			defer wg.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			// A lobby that cannot be fetched just leaves that match out.
			m.lobbyElo, _ = s.lobbyElo(m.id)
		}(&matches[i])
	}
	wg.Wait()

	return summarisePerformance(matches)
}

// lobbyElo returns the average ELO of the two teams in a match, or 0 when
// Faceit does not say.
func (s *FaceitService) lobbyElo(matchID string) (float64, error) {
	var match struct {
		Teams map[string]struct {
			Stats struct {
				Rating float64 `json:"rating"`
			} `json:"stats"`
			Roster []struct {
				SkillLevel int `json:"game_skill_level"`
			} `json:"roster"`
		} `json:"teams"`
	}
	found, err := s.get("/matches/"+url.PathEscape(matchID), &match)
	if err != nil || !found {
		return 0, err
	}

	var sum float64
	var teams int
	for _, team := range match.Teams {
		elo := team.Stats.Rating
		if elo <= 0 {
			// No team rating (e.g. hub matches): estimate from the levels.
			var levels, n float64
			for _, r := range team.Roster {
				if r.SkillLevel >= 1 && r.SkillLevel < len(faceitLevelElo) {
					levels += faceitLevelElo[r.SkillLevel]
					n++
				}
			}
			if n == 0 {
				continue
			}
			elo = levels / n
		}
		sum += elo
		teams++
	}
	if teams == 0 {
		return 0, nil
	}
	return sum / float64(teams), nil
}

// lifetimeMatches returns how many CS2 matches a player has on Faceit.
func (s *FaceitService) lifetimeMatches(playerID string) (int, error) {
	var result struct {
		Lifetime map[string]any `json:"lifetime"`
	}
	found, err := s.get("/players/"+url.PathEscape(playerID)+"/stats/cs2", &result)
	if err != nil || !found {
		return 0, err
	}
	return int(faceitNum(result.Lifetime, "Matches")), nil
}

// faceitNum reads a stat that Faceit sends either as a string ("Kills": "17")
// or as a number.
func faceitNum(stats map[string]any, key string) float64 {
	switch v := stats[key].(type) {
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	case float64:
		return v
	}
	return 0
}

// recentStats summarises a player's latest matches, and also returns them
// one by one.
func (s *FaceitService) recentStats(playerID string) (*FaceitRecent, []faceitMatch, error) {
	var result struct {
		Items []struct {
			Stats map[string]any `json:"stats"`
		} `json:"items"`
	}
	path := fmt.Sprintf("/players/%s/games/cs2/stats?limit=%d", url.PathEscape(playerID), FaceitRecentMatches)
	found, err := s.get(path, &result)
	if err != nil || !found || len(result.Items) == 0 {
		return nil, nil, err
	}

	num := faceitNum

	recent := &FaceitRecent{Matches: len(result.Items)}
	matches := make([]faceitMatch, 0, len(result.Items))
	var kills, deaths, headshots, adrSum, adrMatches, wins, latest float64
	for _, item := range result.Items {
		kills += num(item.Stats, "Kills")
		deaths += num(item.Stats, "Deaths")
		headshots += num(item.Stats, "Headshots")
		// Older matches come without an ADR; counting them as 0 would drag
		// the average down.
		if adr := num(item.Stats, "ADR"); adr > 0 {
			adrSum += adr
			adrMatches++
		}
		wins += num(item.Stats, "Result")
		at := num(item.Stats, "Match Finished At")
		if at > latest {
			latest = at
		}

		m := faceitMatch{finishedAt: at, adr: num(item.Stats, "ADR"), kd: num(item.Stats, "Kills")}
		m.id, _ = item.Stats["Match Id"].(string)
		if d := num(item.Stats, "Deaths"); d > 0 {
			m.kd /= d
		}
		matches = append(matches, m)
	}
	if deaths > 0 {
		recent.KDRatio = kills / deaths
	} else {
		recent.KDRatio = kills
	}
	if kills > 0 {
		recent.HSPercent = headshots / kills * 100
	}
	if adrMatches > 0 {
		recent.ADR = adrSum / adrMatches
	}
	recent.WinRate = wins / float64(recent.Matches) * 100
	if latest > 0 {
		t := time.UnixMilli(int64(latest)).UTC()
		recent.LastMatchAt = &t
	}
	return recent, matches, nil
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
