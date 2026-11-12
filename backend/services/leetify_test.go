package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A trimmed real response from the Leetify public API (v3/profile).
const leetifyFixture = `{
  "privacy_mode": "public", "winrate": 0.4333, "total_matches": 954, "name": "someone",
  "bans": [], "steam64_id": "76561198000000001",
  "ranks": {"leetify": 1.05, "premier": null, "faceit": 10, "faceit_elo": 2118, "wingman": 15},
  "rating": {"aim": 80.5, "positioning": 76.4, "utility": 62.4, "clutch": 0.08, "opening": 0.04},
  "stats": {"accuracy_head": 21.6, "reaction_time_ms": 532.6,
            "ct_opening_duel_success_percentage": 60.4, "t_opening_duel_success_percentage": 56.0},
  "recent_matches": [{"finished_at": "2026-11-08T00:33:21.000Z", "data_source": "faceit",
                      "outcome": "loss", "leetify_rating": -0.0302}]
}`

func TestFetchLeetifyProfile(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAuth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		if r.URL.Query().Get("steam64_id") == "404" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(leetifyFixture))
	}))
	defer srv.Close()
	old := leetifyBaseURL
	leetifyBaseURL = srv.URL
	defer func() { leetifyBaseURL = old }()

	p, err := FetchLeetifyProfile("76561198000000001", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v3/profile" || gotQuery != "steam64_id=76561198000000001" {
		t.Errorf("wrong endpoint: %s?%s", gotPath, gotQuery)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if p.PrivacyMode != "public" || p.Ranks.Faceit != 10 || p.Ranks.FaceitElo == nil || *p.Ranks.FaceitElo != 2118 {
		t.Errorf("ranks not decoded: %+v", p.Ranks)
	}
	if p.Ranks.Premier != 0 {
		t.Errorf("null premier should decode to 0, got %d", p.Ranks.Premier)
	}
	if len(p.RecentMatches) != 1 || p.RecentMatches[0].DataSource != "faceit" {
		t.Errorf("recent matches not decoded: %+v", p.RecentMatches)
	}
	if score := ComputeSkillScore(p); score < 30 || score > 70 {
		t.Errorf("unexpected score for a Faceit level 10 profile: %v", score)
	}

	if _, err := FetchLeetifyProfile("76561198000000001", ""); err != nil || gotAuth != "" {
		t.Errorf("without a key no Authorization header should be sent (got %q, err %v)", gotAuth, err)
	}
	if _, err := FetchLeetifyProfile("404", ""); err == nil {
		t.Error("expected an error for an unknown player")
	}
}

func TestLastKnownPremier(t *testing.T) {
	premier, competitive := 11, 12
	matches := []RecentMatch{
		{FinishedAt: "2026-10-22T10:00:00.000Z", RankType: &premier, Rank: 0},      // unranked this season
		{FinishedAt: "2026-10-19T10:00:00.000Z", RankType: &competitive, Rank: 18}, // skill group, not a rating
		{FinishedAt: "2026-07-29T10:00:00.000Z", RankType: &premier, Rank: 26477},
		{FinishedAt: "2026-07-30T10:00:00.000Z", RankType: &premier, Rank: 26484}, // latest rated
		{FinishedAt: "2026-07-31T10:00:00.000Z", DataSource: "faceit", Rank: 10},  // no rank type
	}
	if got := lastKnownPremier(matches); got != 26484 {
		t.Errorf("got %d, want 26484", got)
	}
	if got := lastKnownPremier(matches[:2]); got != 0 {
		t.Errorf("no rated Premier match: got %d, want 0", got)
	}

	at, ok := PremierRatedAt(&LeetifyProfile{RecentMatches: matches})
	if !ok || at.Format("2006-01-02") != "2026-07-30" {
		t.Errorf("PremierRatedAt = %v, %v; want 2026-07-30", at, ok)
	}
	if _, ok := PremierRatedAt(&LeetifyProfile{RecentMatches: matches[:2]}); ok {
		t.Error("PremierRatedAt must be unknown without a rated Premier match")
	}
}

func TestFetchLeetifyProfile_PremierFallback(t *testing.T) {
	body := `{"privacy_mode":"public","ranks":{"premier":%s},"recent_matches":[
		{"finished_at":"2026-07-30T10:00:00.000Z","data_source":"matchmaking","rank_type":11,"rank":26484}]}`
	current := "null"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Replace(body, "%s", current, 1)))
	}))
	defer srv.Close()
	old := leetifyBaseURL
	leetifyBaseURL = srv.URL
	defer func() { leetifyBaseURL = old }()

	if p, err := FetchLeetifyProfile("1", ""); err != nil || p.Ranks.Premier != 26484 {
		t.Errorf("null premier should fall back to the last rated match, got %+v (err %v)", p, err)
	}
	current = "21000"
	if p, err := FetchLeetifyProfile("1", ""); err != nil || p.Ranks.Premier != 21000 {
		t.Errorf("a current rating must win over match history, got %+v (err %v)", p, err)
	}
}
