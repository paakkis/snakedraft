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

	// The better rank is the one that counts, with its own confidence.
	a = AssessRank(p, &FaceitPlayer{Elo: 2400, LifetimeMatches: 180})
	if a.Source != RankSourceFaceit || !near(a.Value, 0.8) || !near(a.Confidence, 0.9) {
		t.Errorf("better Faceit rank: got %+v", a)
	}
	a = AssessRank(p, &FaceitPlayer{Elo: 900, LifetimeMatches: 180})
	if a.Source != RankSourcePremier {
		t.Errorf("better Premier rank: got %+v", a)
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
	if fresh <= settled+10 {
		t.Errorf("a 5-match rank must weigh much less than a settled one: fresh=%v settled=%v", fresh, settled)
	}

	// Hand-computed for the fresh account: rank 1/3 at weight 0.55·0.2 = 0.11,
	// aim and positioning 0.8 at weight 0.25 → (0.11/3 + 0.2) / 0.36.
	want := (0.11/3 + 0.2) / 0.36 * 100
	if math.Abs(fresh-want) > 0.01 {
		t.Errorf("fresh account: got %v, want %v", fresh, want)
	}
}

func TestComputeFaceitScore_UnsettledElo(t *testing.T) {
	fixClock(t, testNow)

	// Strong recent form on a 5-match account: ELO 0.5 at weight 0.75·0.2,
	// form 1.0 at weight 0.25 → (0.075 + 0.25) / 0.4 = 81.25.
	f := &FaceitPlayer{Elo: 1500, LifetimeMatches: 5, Recent: &FaceitRecent{Matches: 5, KDRatio: 1.4, ADR: 100}}
	if got := ComputeFaceitScore(f); math.Abs(got-81.25) > 0.01 {
		t.Errorf("got %v, want 81.25", got)
	}
}
