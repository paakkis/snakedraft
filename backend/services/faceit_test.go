package services

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
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
