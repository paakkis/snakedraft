package services

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func fakeFaceit(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	old := faceitBaseURL
	faceitBaseURL = srv.URL
	t.Cleanup(func() { faceitBaseURL = old; srv.Close() })
}

func TestFaceitGetPlayer(t *testing.T) {
	fakeFaceit(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("missing bearer token, got %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.URL.Path == "/players" && r.URL.Query().Get("game_player_id") == "76561198000000001":
			if r.URL.Query().Get("game") != "cs2" {
				t.Errorf("lookup must be scoped to cs2, got %q", r.URL.RawQuery)
			}
			w.Write([]byte(`{"player_id":"abc","nickname":"Nick","games":{
				"cs2":{"skill_level":10,"faceit_elo":2314},
				"csgo":{"skill_level":10,"faceit_elo":2480}}}`))
		case r.URL.Path == "/players" && r.URL.Query().Get("game_player_id") == "csgo-only":
			w.Write([]byte(`{"player_id":"old","nickname":"Old","games":{"csgo":{"skill_level":5,"faceit_elo":1100}}}`))
		case r.URL.Path == "/players/abc/stats/cs2":
			w.Write([]byte(`{"lifetime":{"Matches":"1829","Total Matches":"356","Wins":"940"}}`))
		case strings.HasPrefix(r.URL.Path, "/players/abc/games/cs2/stats"):
			// Real responses mix string and numeric values.
			w.Write([]byte(`{"items":[
				{"stats":{"Kills":"20","Deaths":"10","Headshots":"10","ADR":"90","Result":"1","Match Finished At":1794179812000}},
				{"stats":{"Kills":"10","Deaths":"20","Headshots":"5","ADR":"70.5","Result":"0","Match Finished At":1794176721000}}]}`))
		default:
			http.Error(w, `{"errors":[{"message":"The resource was not found."}]}`, http.StatusNotFound)
		}
	})
	s := NewFaceitService("key")

	p, err := s.GetPlayer("76561198000000001")
	if err != nil || p == nil {
		t.Fatalf("GetPlayer: %v, %v", p, err)
	}
	if p.Nickname != "Nick" || p.Elo != 2314 || p.Level != 10 {
		t.Errorf("wrong player (must use the cs2 entry, not csgo): %+v", p)
	}
	if p.LifetimeMatches != 1829 {
		t.Errorf("wrong lifetime match count: %d", p.LifetimeMatches)
	}
	r := p.Recent
	if r == nil {
		t.Fatal("expected recent stats")
	}
	if r.Matches != 2 || r.KDRatio != 1.0 || r.HSPercent != 50 || r.WinRate != 50 || math.Abs(r.ADR-80.25) > 1e-9 {
		t.Errorf("wrong recent stats: %+v", r)
	}
	if r.LastMatchAt == nil || r.LastMatchAt.UnixMilli() != 1794179812000 {
		t.Errorf("wrong last match time: %v", r.LastMatchAt)
	}

	if p, err := s.GetPlayer("unknown"); p != nil || err != nil {
		t.Errorf("unknown player should be (nil, nil), got (%v, %v)", p, err)
	}
	if p, err := s.GetPlayer("csgo-only"); p != nil || err != nil {
		t.Errorf("player without a cs2 profile should be (nil, nil), got (%v, %v)", p, err)
	}
}

func TestFaceitGetPlayer_Unavailable(t *testing.T) {
	called := false
	fakeFaceit(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "nope", http.StatusForbidden)
	})

	// No key configured: no request at all.
	if p, err := NewFaceitService("").GetPlayer("76561198000000001"); p != nil || err != nil || called {
		t.Errorf("without a key Faceit must be skipped, got (%v, %v), called=%v", p, err, called)
	}
	// API failure: reported as an error so it gets logged, never a bogus player.
	if p, err := NewFaceitService("key").GetPlayer("76561198000000001"); p != nil || err == nil {
		t.Errorf("expected an error on HTTP 403, got (%v, %v)", p, err)
	}
}

func TestFaceitGetPlayer_Performance(t *testing.T) {
	var mu sync.Mutex
	requested := map[string]int{} // lobbies are looked up concurrently
	fakeFaceit(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/players":
			w.Write([]byte(`{"player_id":"abc","nickname":"Nick","games":{"cs2":{"skill_level":4,"faceit_elo":1000}}}`))
		case r.URL.Path == "/players/abc/stats/cs2":
			w.Write([]byte(`{"lifetime":{"Matches":"12"}}`))
		case strings.HasPrefix(r.URL.Path, "/players/abc/games/cs2/stats"):
			// Deliberately not newest first.
			w.Write([]byte(`{"items":[
				{"stats":{"Match Id":"1-old","Kills":"10","Deaths":"10","ADR":"75","Result":"0","Match Finished At":1000}},
				{"stats":{"Match Id":"1-new","Kills":"28","Deaths":"20","ADR":"100","Result":"1","Match Finished At":3000}},
				{"stats":{"Match Id":"1-hub","Kills":"10","Deaths":"10","ADR":"75","Result":"1","Match Finished At":2000}},
				{"stats":{"Match Id":"1-gone","Kills":"30","Deaths":"5","ADR":"140","Result":"1","Match Finished At":500}},
				{"stats":{"Kills":"30","Deaths":"5","ADR":"140","Result":"1","Match Finished At":400}}]}`))
		case strings.HasPrefix(r.URL.Path, "/matches/"):
			id := strings.TrimPrefix(r.URL.Path, "/matches/")
			mu.Lock()
			requested[id]++
			mu.Unlock()
			switch id {
			case "1-new":
				w.Write([]byte(`{"teams":{
					"faction1":{"stats":{"rating":2100},"roster":[{"game_skill_level":10}]},
					"faction2":{"stats":{"rating":1900},"roster":[{"game_skill_level":9}]}}}`))
			case "1-old":
				w.Write([]byte(`{"teams":{
					"faction1":{"stats":{"rating":1000},"roster":[]},
					"faction2":{"stats":{"rating":1000},"roster":[]}}}`))
			case "1-hub":
				// No team rating: estimated from the levels (4 → 975, 6 → 1275).
				w.Write([]byte(`{"teams":{
					"faction1":{"roster":[{"game_skill_level":4},{"game_skill_level":4}]},
					"faction2":{"roster":[{"game_skill_level":6},{"game_skill_level":6}]}}}`))
			default:
				http.Error(w, "gone", http.StatusNotFound)
			}
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	p, err := NewFaceitService("key").GetPlayer("76561198000000001")
	if err != nil || p == nil {
		t.Fatalf("GetPlayer: %v, %v", p, err)
	}
	if p.Recent == nil || p.Recent.Matches != 5 {
		t.Fatalf("recent stats must cover every match: %+v", p.Recent)
	}
	perf := p.Performance
	if perf == nil {
		t.Fatal("expected a performance summary")
	}

	// Newest first: 1-new (lobby 2000, dominant → 2400), 1-hub (lobby 1125,
	// average → 1125), 1-old (lobby 1000, average → 1000). The match that is
	// gone and the one without an ID are left out.
	wantImplied := (2400 + 0.85*1125 + 0.85*0.85*1000) / (1 + 0.85 + 0.85*0.85)
	wantLobby := (2000 + 1125 + 1000) / 3.0
	if perf.Matches != 3 || math.Abs(perf.ImpliedElo-wantImplied) > 1e-6 || math.Abs(perf.LobbyElo-wantLobby) > 1e-6 {
		t.Errorf("got %+v, want 3 matches, implied %v, lobby %v", perf, wantImplied, wantLobby)
	}
	for _, id := range []string{"1-new", "1-hub", "1-old", "1-gone"} {
		if requested[id] != 1 {
			t.Errorf("match %s requested %d times, want 1", id, requested[id])
		}
	}
}

func TestFaceitGetPlayer_PerformanceLooksUpOnlyTheLatestMatches(t *testing.T) {
	var mu sync.Mutex
	lookups := 0
	fakeFaceit(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/players":
			w.Write([]byte(`{"player_id":"abc","nickname":"Nick","games":{"cs2":{"skill_level":4,"faceit_elo":1000}}}`))
		case strings.HasPrefix(r.URL.Path, "/players/abc/games/cs2/stats"):
			var items []string
			for i := 0; i < 30; i++ {
				items = append(items, fmt.Sprintf(
					`{"stats":{"Match Id":"m%d","Kills":"10","Deaths":"10","ADR":"75","Match Finished At":%d}}`, i, 1000+i))
			}
			w.Write([]byte(`{"items":[` + strings.Join(items, ",") + `]}`))
		case strings.HasPrefix(r.URL.Path, "/matches/"):
			mu.Lock()
			lookups++
			mu.Unlock()
			w.Write([]byte(`{"teams":{"faction1":{"stats":{"rating":1500}},"faction2":{"stats":{"rating":1500}}}}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	p, err := NewFaceitService("key").GetPlayer("76561198000000001")
	if err != nil || p == nil || p.Performance == nil {
		t.Fatalf("GetPlayer: %+v, %v", p, err)
	}
	if lookups != FaceitPerformanceMatches || p.Performance.Matches != FaceitPerformanceMatches {
		t.Errorf("looked up %d lobbies for %d matches, want %d", lookups, p.Performance.Matches, FaceitPerformanceMatches)
	}
	if math.Abs(p.Performance.ImpliedElo-1500) > 1e-6 {
		t.Errorf("average games in 1500 lobbies: implied %v, want 1500", p.Performance.ImpliedElo)
	}
}
