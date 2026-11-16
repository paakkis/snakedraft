package services

import (
	"math"
	"testing"
	"time"
)

// fixClock pins timeNow for the duration of a test.
func fixClock(t *testing.T, now time.Time) {
	t.Helper()
	old := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = old })
}

var testNow = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestRankConfidence(t *testing.T) {
	fixClock(t, testNow)
	yearAgo := testNow.Add(-rankHalfLife)
	future := testNow.Add(time.Hour)

	cases := []struct {
		name    string
		matches int
		last    *time.Time
		want    float64
	}{
		{"nothing known is fully trusted", UnknownMatches, nil, 1},
		{"no matches", 0, nil, 0},
		{"5 matches", 5, nil, 0.2},
		{"20 matches", 20, nil, 0.5},
		{"180 matches", 180, nil, 0.9},
		{"a year unplayed halves it", UnknownMatches, &yearAgo, 0.5},
		{"both factors multiply", 20, &yearAgo, 0.25},
		{"just played", 20, &testNow, 0.5},
		{"clock skew is ignored", 20, &future, 0.5},
	}
	for _, c := range cases {
		if got := rankConfidence(c.matches, c.last); !near(got, c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// recentMatches builds n recent Leetify matches of one kind, the newest at `last`.
func recentMatches(n int, source string, rankType int, last time.Time) []RecentMatch {
	out := make([]RecentMatch, n)
	for i := range out {
		out[i] = RecentMatch{
			DataSource: source,
			FinishedAt: last.Add(-time.Duration(i) * 24 * time.Hour).Format(time.RFC3339),
			Rank:       15000,
		}
		if rankType != 0 {
			rt := rankType
			out[i].RankType = &rt
		}
	}
	return out
}

func TestLeetifyMatches(t *testing.T) {
	p := &LeetifyProfile{PrivacyMode: "public"}

	if n, last := leetifyMatches(p, isPremierMatch); n != UnknownMatches || last != nil {
		t.Errorf("no recent matches: got (%v, %v), want unknown", n, last)
	}

	// Everything Leetify knows is listed: the count is exact.
	p.RecentMatches = append(recentMatches(6, "matchmaking", leetifyRankTypePremier, testNow),
		recentMatches(2, "faceit", 0, testNow.Add(-48*time.Hour))...)
	p.TotalMatches = 8
	if n, last := leetifyMatches(p, isPremierMatch); n != 6 || last == nil || !last.Equal(testNow) {
		t.Errorf("premier: got (%v, %v), want (6, %v)", n, last, testNow)
	}
	if n, _ := leetifyMatches(p, isFaceitMatch); n != 2 {
		t.Errorf("faceit: got %v, want 2", n)
	}

	// Only the latest matches are listed: the share is applied to the total.
	p.TotalMatches = 400
	if n, _ := leetifyMatches(p, isPremierMatch); n != 300 {
		t.Errorf("premier estimate: got %v, want 300", n)
	}

	// A rank older than everything listed: nothing can be said.
	if n, _ := leetifyMatches(p, func(RecentMatch) bool { return false }); n != UnknownMatches {
		t.Errorf("no matching matches: got %v, want unknown", n)
	}
}

func TestAssessRank(t *testing.T) {
	fixClock(t, testNow)

	if a := AssessRank(nil, nil); a.Value != 0 || a.Source != "" {
		t.Errorf("no data: got %+v", a)
	}

	// Faceit's own match count drives the confidence.
	a := AssessRank(nil, &FaceitPlayer{Elo: 1500, LifetimeMatches: 5})
	if a.Source != RankSourceFaceit || !near(a.Value, 0.5) || a.Matches != 5 || !near(a.Confidence, 0.2) {
		t.Errorf("new Faceit account: got %+v", a)
	}

	// Data stored before match counts were fetched stays fully trusted.
	if a := AssessRank(nil, &FaceitPlayer{Elo: 1500}); a.Matches != UnknownMatches || a.Confidence != 1 {
		t.Errorf("unknown match count: got %+v", a)
	}

	// A Faceit rank last played two years ago.
	old := testNow.Add(-2 * rankHalfLife)
	a = AssessRank(nil, &FaceitPlayer{Elo: 1500, LifetimeMatches: 180, Recent: &FaceitRecent{Matches: 1, LastMatchAt: &old}})
	if !near(a.Confidence, 0.9*0.25) {
		t.Errorf("stale Faceit rank: confidence %v, want %v", a.Confidence, 0.9*0.25)
	}

	// Premier with a few placement matches.
	p := &LeetifyProfile{PrivacyMode: "public", TotalMatches: 12}
	p.Ranks.Premier = 17500
	p.RecentMatches = append(recentMatches(10, "matchmaking", leetifyRankTypePremier, testNow),
		recentMatches(2, "matchmaking", 12, testNow)...)
	a = AssessRank(p, nil)
	if a.Source != RankSourcePremier || !near(a.Value, 0.5) || a.Matches != 10 || !near(a.Confidence, 10.0/30) {
		t.Errorf("new Premier player: got %+v", a)
	}

	// A settled, better Faceit rank wins over the thin Premier one.
	a = AssessRank(p, &FaceitPlayer{Elo: 2400, LifetimeMatches: 180})
	if a.Source != RankSourceFaceit || !near(a.Value, 0.8) || !near(a.Confidence, 0.9) {
		t.Errorf("better Faceit rank: got %+v", a)
	}

	// The other rank backs the best one up in proportion to how close it is.
	// A thin Faceit rank a little above Premier: its own confidence is 0.2,
	// Premier's backing is 0.33·0.5/0.55.
	a = AssessRank(p, &FaceitPlayer{Elo: 1650, LifetimeMatches: 5})
	if a.Source != RankSourceFaceit || !near(a.Value, 0.55) || !near(a.Confidence, 10.0/30*0.5/0.55) {
		t.Errorf("thin Faceit rank backed by Premier: got %+v", a)
	}

	// A settled Premier rating right behind a thin Faceit rank leaves little doubt…
	settled := &LeetifyProfile{PrivacyMode: "public", TotalMatches: 1000}
	settled.Ranks.Premier = 17500
	settled.RecentMatches = recentMatches(100, "matchmaking", leetifyRankTypePremier, testNow)
	premierConf := 1000.0 / 1020
	a = AssessRank(settled, &FaceitPlayer{Elo: 1650, LifetimeMatches: 5})
	if a.Source != RankSourceFaceit || !near(a.Confidence, premierConf*0.5/0.55) {
		t.Errorf("thin Faceit rank backed by settled Premier: got %+v", a)
	}

	// …while one far below it adds little.
	a = AssessRank(settled, &FaceitPlayer{Elo: 3000, LifetimeMatches: 5})
	if !near(a.Confidence, premierConf*0.5) {
		t.Errorf("distant backing: got %+v", a)
	}

	// A worse rank never lowers the confidence of the best one, and a
	// barely-used Faceit account does not pull a good Premier rating down.
	a = AssessRank(settled, &FaceitPlayer{Elo: 900, LifetimeMatches: 3})
	if a.Source != RankSourcePremier || !near(a.Value, 0.5) || !near(a.Confidence, premierConf) {
		t.Errorf("settled Premier with a barely-used Faceit account: got %+v", a)
	}

	// Without Faceit's own data, Leetify's copy and its Faceit matches are used.
	elo := 1800
	l := &LeetifyProfile{PrivacyMode: "public", TotalMatches: 5}
	l.Ranks.FaceitElo = &elo
	l.RecentMatches = recentMatches(5, "faceit", 0, testNow)
	a = AssessRank(l, nil)
	if a.Source != RankSourceFaceit || !near(a.Value, 0.6) || a.Matches != 5 || !near(a.Confidence, 0.2) {
		t.Errorf("Leetify's Faceit rank: got %+v", a)
	}

	// A private Leetify profile contributes nothing.
	l.PrivacyMode = "private"
	if a := AssessRank(l, nil); a.Value != 0 {
		t.Errorf("private profile: got %+v", a)
	}
}

func TestComputeSkillScore_UnsettledRankCountsLess(t *testing.T) {
	fixClock(t, testNow)

	// Strong mechanics (80) behind a fresh Faceit account still at 1000 ELO.
	p := &LeetifyProfile{PrivacyMode: "public"}
	p.Rating.Aim, p.Rating.Positioning = 80, 80

	settled := ComputeSkillScore(p, &FaceitPlayer{Elo: 1000, LifetimeMatches: 2000})
	fresh := ComputeSkillScore(p, &FaceitPlayer{Elo: 1000, LifetimeMatches: 5})
	if fresh <= settled+5 {
		t.Errorf("a 5-match rank must weigh much less than a settled one: fresh=%v settled=%v", fresh, settled)
	}

	// Hand-computed for the fresh account. The rank (1/3) is raised towards
	// the mechanics (0.8): 1/3 + 0.5·(0.8 − 1/3 − 0.05) = 0.54167. It gets
	// weight 0.55·0.2 = 0.11, aim and positioning 0.8 at weight 0.25.
	rank := 1.0/3 + 0.5*(0.8-1.0/3-0.05)
	want := (0.11*rank + 0.2) / 0.36 * 100
	if math.Abs(fresh-want) > 0.01 {
		t.Errorf("fresh account: got %v, want %v", fresh, want)
	}
}

func TestComputeFaceitScore_FormWeightIsFixed(t *testing.T) {
	fixClock(t, testNow)

	// A thin rank must not hand its weight to the recent form, which depends
	// on the lobbies: 0.5·75% + 1.0·25% = 62.5 whatever the match count.
	for _, lifetime := range []int{5, 2000} {
		f := &FaceitPlayer{Elo: 1500, LifetimeMatches: lifetime, Recent: &FaceitRecent{Matches: 5, KDRatio: 1.4, ADR: 100}}
		if got := ComputeFaceitScore(f); math.Abs(got-62.5) > 0.01 {
			t.Errorf("%d lifetime matches: got %v, want 62.5", lifetime, got)
		}
	}
}

func TestAssessRank_MechanicsAboveRank(t *testing.T) {
	fixClock(t, testNow)
	profile := func(premier int, aim, positioning float64) *LeetifyProfile {
		p := &LeetifyProfile{PrivacyMode: "public"}
		p.Ranks.Premier = premier
		p.Rating.Aim, p.Rating.Positioning = aim, positioning
		return p
	}

	// Rank 0.4, mechanics 0.8: raised by half of the gap beyond the tolerance.
	a := AssessRank(profile(14000, 80, 80), nil)
	if !near(a.Listed, 0.4) || !near(a.Mechanics, 0.8) || !near(a.Value, 0.4+0.5*0.35) || !a.Underrated {
		t.Errorf("mechanics far above rank: got %+v", a)
	}

	// Aim counts for more than positioning (0.15 vs 0.10): (90·0.15 + 40·0.10) / 0.25 = 70.
	if a := AssessRank(profile(14000, 90, 40), nil); !near(a.Mechanics, 0.7) {
		t.Errorf("weighted mechanics: got %v, want 0.7", a.Mechanics)
	}

	// A gap between the tolerance and the flag: nudged, but not flagged.
	a = AssessRank(profile(14000, 48, 48), nil)
	if !near(a.Value, 0.4+0.5*0.03) || a.Underrated {
		t.Errorf("small gap: got %+v", a)
	}

	// Within the tolerance, and mechanics below the rank: the rank stands.
	for _, m := range []float64{44, 40, 20} {
		if a := AssessRank(profile(14000, m, m), nil); !near(a.Value, 0.4) || a.Underrated {
			t.Errorf("mechanics %v: rank must stand, got %+v", m, a)
		}
	}

	// No ratings from Leetify: nothing to compare against.
	if a := AssessRank(profile(14000, 0, 0), nil); !near(a.Value, 0.4) || a.Mechanics != 0 || a.Underrated {
		t.Errorf("no mechanics: got %+v", a)
	}

	// The correction never pushes the rank past the top of the scale.
	if a := AssessRank(profile(34000, 100, 100), nil); a.Value > 1 {
		t.Errorf("rank above 1: %+v", a)
	}

	// The comparison is against the best listed rank, whichever platform it is from.
	a = AssessRank(profile(14000, 80, 80), &FaceitPlayer{Elo: 2400})
	if a.Source != RankSourceFaceit || !near(a.Value, 0.8) || a.Underrated {
		t.Errorf("Faceit rank matching the mechanics: got %+v", a)
	}
}

func TestComputeSkillScore_UnderratedPlayerScoresHigher(t *testing.T) {
	fixClock(t, testNow)

	// Hand-computed: rank 0.4 raised to 0.575 at weight 0.55, mechanics 0.8
	// at weight 0.25 → (0.31625 + 0.2) / 0.8 = 64.53. Without the correction
	// it would be (0.22 + 0.2) / 0.8 = 52.5.
	p := &LeetifyProfile{PrivacyMode: "public"}
	p.Ranks.Premier = 14000
	p.Rating.Aim, p.Rating.Positioning = 80, 80
	if got := ComputeSkillScore(p, nil); math.Abs(got-64.53) > 0.01 {
		t.Errorf("got %v, want 64.53", got)
	}
}

func TestFaceitMatchRating(t *testing.T) {
	cases := []struct{ kd, adr, want float64 }{
		{1.0, 75, 0.5}, // an average player in the lobby
		{1.4, 100, 1},
		{0.6, 50, 0},
		{3.0, 150, 1}, // clamped
		{1.4, 50, 0.5},
	}
	for _, c := range cases {
		if got := faceitMatchRating(c.kd, c.adr); !near(got, c.want) {
			t.Errorf("faceitMatchRating(%v, %v) = %v, want %v", c.kd, c.adr, got, c.want)
		}
	}
}

func TestSummarisePerformance(t *testing.T) {
	if p := summarisePerformance(nil); p != nil {
		t.Errorf("no matches: got %+v", p)
	}
	if p := summarisePerformance([]faceitMatch{{kd: 2, adr: 120}}); p != nil {
		t.Errorf("no known lobby: got %+v", p)
	}

	// An average game: the player belongs in that lobby.
	p := summarisePerformance([]faceitMatch{{kd: 1.0, adr: 75, lobbyElo: 2000}})
	if p == nil || p.Matches != 1 || !near(p.ImpliedElo, 2000) || !near(p.LobbyElo, 2000) {
		t.Errorf("average game: got %+v", p)
	}

	// Dominating and being dominated: 400 ELO either side of the lobby.
	if p := summarisePerformance([]faceitMatch{{kd: 1.4, adr: 100, lobbyElo: 2000}}); !near(p.ImpliedElo, 2400) {
		t.Errorf("dominant game: got %v, want 2400", p.ImpliedElo)
	}
	if p := summarisePerformance([]faceitMatch{{kd: 0.6, adr: 50, lobbyElo: 2000}}); !near(p.ImpliedElo, 1600) {
		t.Errorf("dominated game: got %v, want 1600", p.ImpliedElo)
	}

	// Newer matches weigh more, and matches without a lobby are left out
	// without taking up a place in the decay.
	p = summarisePerformance([]faceitMatch{
		{kd: 1.4, adr: 100, lobbyElo: 2000}, // 2400, weight 1
		{kd: 9, adr: 200},                   // unknown lobby
		{kd: 1.0, adr: 75, lobbyElo: 1000},  // 1000, weight 0.85
	})
	want := (2400 + 0.85*1000) / 1.85
	if p.Matches != 2 || !near(p.ImpliedElo, want) || !near(p.LobbyElo, 1500) {
		t.Errorf("mixed matches: got %+v, want implied %v over 2 matches", p, want)
	}
}

func TestAssessRank_PerformanceAgainstLobby(t *testing.T) {
	fixClock(t, testNow)
	player := func(elo, lifetime, perfMatches int, implied float64) *FaceitPlayer {
		return &FaceitPlayer{Elo: elo, LifetimeMatches: lifetime,
			Performance: &FaceitPerformance{Matches: perfMatches, ImpliedElo: implied}}
	}

	// A 5-match account at 1000 ELO holding its own in 2000 ELO lobbies.
	// Confidence 0.2 → pull (1 − 0.75·0.2) · 5/10 = 0.425 → 1425 ELO.
	a := AssessRank(nil, player(1000, 5, 5, 2000))
	if !near(a.Listed, 1000.0/3000) || !near(a.Value, 1425.0/3000) || !a.Outperforming {
		t.Errorf("fresh account in strong lobbies: got %+v", a)
	}
	if !near(a.Confidence, 0.2) {
		t.Errorf("performance must not change the confidence: got %v", a.Confidence)
	}

	// The same performance on a settled rank moves it much less:
	// confidence 0.99 → pull (1 − 0.75·0.99) · 10/15 ≈ 0.17.
	settled := AssessRank(nil, player(1000, 1980, 10, 2000))
	wantElo := 1000 + (1-0.75*0.99)*(10.0/15)*1000
	if !near(settled.Value, wantElo/3000) || settled.Value >= a.Value {
		t.Errorf("settled rank: got %v, want %v", settled.Value, wantElo/3000)
	}

	// It works both ways: a player carried above their level is moved down.
	a = AssessRank(nil, player(2000, 5, 5, 1000))
	if !near(a.Value, 1575.0/3000) || a.Outperforming {
		t.Errorf("underperforming account: got %+v", a)
	}

	// Performing like one's own ELO changes nothing.
	a = AssessRank(nil, player(1500, 100, 10, 1500))
	if !near(a.Value, 0.5) || a.Outperforming {
		t.Errorf("performance in line with the rank: got %+v", a)
	}

	// A big gap over too few matches moves the rank a little but is not flagged.
	a = AssessRank(nil, player(1000, 300, 2, 2000))
	if a.Value <= a.Listed || a.Outperforming {
		t.Errorf("two good matches: got %+v", a)
	}

	// The adjusted Faceit rank is what competes with Premier.
	p := &LeetifyProfile{PrivacyMode: "public"}
	p.Ranks.Premier = 1750 // 0.05
	if a := AssessRank(p, player(1000, 5, 5, 2000)); a.Source != RankSourceFaceit || !near(a.Value, 1425.0/3000) {
		t.Errorf("adjusted Faceit rank should beat a very low Premier: got %+v", a)
	}

	// The mechanics are compared with the adjusted rank, so the same
	// evidence is not counted twice: 0.475 vs mechanics 0.5 is within tolerance.
	p.Rating.Aim, p.Rating.Positioning = 50, 50
	if a := AssessRank(p, player(1000, 5, 5, 2000)); !near(a.Value, 1425.0/3000) || a.Underrated {
		t.Errorf("mechanics within tolerance of the adjusted rank: got %+v", a)
	}

	// Scored on a settled Premier rank, the player is still flagged.
	p.Ranks.Premier = 21000
	if a := AssessRank(p, player(1000, 5, 5, 2000)); a.Source != RankSourcePremier || !near(a.Value, 0.6) || !a.Outperforming {
		t.Errorf("flag must survive being scored on Premier: got %+v", a)
	}
}

func TestAssessRank_OldPerformanceCountsLess(t *testing.T) {
	fixClock(t, testNow)
	player := func(age time.Duration) *FaceitPlayer {
		last := testNow.Add(-age)
		return &FaceitPlayer{Elo: 1800, LifetimeMatches: 480,
			Recent:      &FaceitRecent{Matches: 10, KDRatio: 1.4, ADR: 100, LastMatchAt: &last},
			Performance: &FaceitPerformance{Matches: 10, ImpliedElo: 2300}}
	}
	// 480 matches → settled 0.96 → pull (1 − 0.75·0.96) · 10/15 = 0.18667.
	const pull = (1 - 0.75*0.96) * 10.0 / 15

	// Played yesterday: the full pull, and flagged.
	a := AssessRank(nil, player(24*time.Hour))
	if math.Abs(a.Value-(1800+pull*500)/3000) > 1e-3 || !a.Outperforming {
		t.Errorf("fresh matches: got %+v", a)
	}

	// One half-life ago: half the pull.
	a = AssessRank(nil, player(performanceHalfLife))
	if !near(a.Value, (1800+pull*0.5*500)/3000) {
		t.Errorf("matches one half-life old: got %v", a.Value)
	}

	// 20 months ago: the rank barely moves and the player is not flagged,
	// even though the stale rank itself has lost most of its confidence.
	a = AssessRank(nil, player(600*24*time.Hour))
	if a.Value-a.Listed > 0.004 || a.Outperforming {
		t.Errorf("20-month-old matches must not matter: got %+v", a)
	}

	// The same goes for the recent form of a Faceit-only player.
	fresh, old := ComputeFaceitScore(player(24*time.Hour)), ComputeFaceitScore(player(600*24*time.Hour))
	if old > 62 || fresh < 70 {
		t.Errorf("old form must count for less: fresh=%v old=%v", fresh, old)
	}
}

func TestComputeFaceitScore_PerformanceAgainstLobby(t *testing.T) {
	fixClock(t, testNow)

	// ELO alone, moved from 1000 to 1425 by the performance → 47.5.
	f := &FaceitPlayer{Elo: 1000, LifetimeMatches: 5, Performance: &FaceitPerformance{Matches: 5, ImpliedElo: 2000}}
	if got := ComputeFaceitScore(f); math.Abs(got-47.5) > 0.01 {
		t.Errorf("got %v, want 47.5", got)
	}
}
