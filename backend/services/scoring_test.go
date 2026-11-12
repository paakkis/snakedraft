package services

import (
	"math"
	"testing"
)

func TestComputeSkillScore_PrivateProfile(t *testing.T) {
	p := &LeetifyProfile{PrivacyMode: "private"}
	score := ComputeSkillScore(p)
	if score != -1 {
		t.Errorf("expected -1 for private profile, got %f", score)
	}
}

func TestComputeSkillScore_EmptyPublicProfile(t *testing.T) {
	// Public but without any stats: there is nothing to score.
	p := &LeetifyProfile{PrivacyMode: "public"}
	if score := ComputeSkillScore(p); score != -1 {
		t.Errorf("expected -1 for a profile without stats, got %f", score)
	}
}

func TestComputeSkillScore_MaxProfile(t *testing.T) {
	elo := 4000
	p := &LeetifyProfile{
		PrivacyMode: "public",
		Ranks: struct {
			Leetify   float64 `json:"leetify"`
			Premier   int     `json:"premier"`
			Faceit    int     `json:"faceit"`
			FaceitElo *int    `json:"faceit_elo"`
		}{
			Premier:   35000,
			Faceit:    10,
			FaceitElo: &elo,
		},
		Rating: struct {
			Aim         float64 `json:"aim"`
			Positioning float64 `json:"positioning"`
			Utility     float64 `json:"utility"`
			Clutch      float64 `json:"clutch"`
			Opening     float64 `json:"opening"`
			CtLeetify   float64 `json:"ct_leetify"`
			TLeetify    float64 `json:"t_leetify"`
		}{Aim: 100, Positioning: 100, Utility: 100},
		Stats: struct {
			AccuracyEnemySpotted           float64 `json:"accuracy_enemy_spotted"`
			AccuracyHead                   float64 `json:"accuracy_head"`
			CtOpeningDuelSuccessPercentage float64 `json:"ct_opening_duel_success_percentage"`
			TOpeningDuelSuccessPercentage  float64 `json:"t_opening_duel_success_percentage"`
			ReactionTimeMs                 float64 `json:"reaction_time_ms"`
			TradeKillsSuccessPercentage    float64 `json:"trade_kills_success_percentage"`
		}{CtOpeningDuelSuccessPercentage: 100, TOpeningDuelSuccessPercentage: 100},
		RecentMatches: []RecentMatch{
			{LeetifyRating: 0.3, DataSource: "valve"},
		},
	}
	score := ComputeSkillScore(p)
	if score < 90 {
		t.Errorf("expected near-max score for max-stat profile, got %f", score)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct{ v, min, max, want float64 }{
		{50, 0, 100, 0.5},
		{0, 0, 100, 0},
		{100, 0, 100, 1},
		{-10, 0, 100, 0},  // clamp low
		{200, 0, 100, 1},  // clamp high
		{5, 5, 5, 0},      // degenerate: min==max
	}
	for _, c := range cases {
		got := normalize(c.v, c.min, c.max)
		if got != c.want {
			t.Errorf("normalize(%v,%v,%v) = %v, want %v", c.v, c.min, c.max, got, c.want)
		}
	}
}

func TestComputeSkillScore_FaceitLevelFallback(t *testing.T) {
	// Without raw ELO, a typical ELO for the level is used: level 5 ≈ 1125 → 37.5.
	p := &LeetifyProfile{PrivacyMode: "public"}
	p.Ranks.Faceit = 5
	if got := ComputeSkillScore(p); math.Abs(got-37.5) > 0.01 {
		t.Errorf("level-5 fallback: got %v, want 37.5", got)
	}

	// Level 10 without an ELO must not be treated as the best player alive.
	p.Ranks.Faceit = 10
	if got := ComputeSkillScore(p); got < 65 || got > 80 {
		t.Errorf("level-10 fallback: got %v, want a just-reached-level-10 score (65–80)", got)
	}

	// Raw ELO wins over level when both are present: 2250/3000 → 75.
	elo := 2250
	p.Ranks.FaceitElo = &elo
	if got := ComputeSkillScore(p); math.Abs(got-75) > 0.01 {
		t.Errorf("elo should take precedence: got %v, want 75", got)
	}
}

// fullProfile is a player active everywhere, with every component at 60%.
func fullProfile() *LeetifyProfile {
	elo := 1800 // 60% of 3000
	p := &LeetifyProfile{PrivacyMode: "public"}
	p.Ranks.FaceitElo = &elo
	p.Ranks.Faceit = 10
	p.Ranks.Premier = 21000 // 60% of 35000
	p.Rating.Aim, p.Rating.Positioning, p.Rating.Utility = 60, 60, 60
	p.Stats.CtOpeningDuelSuccessPercentage, p.Stats.TOpeningDuelSuccessPercentage = 60, 60
	p.RecentMatches = []RecentMatch{{LeetifyRating: 0.1}} // 60% of the -0.2..0.3 range
	return p
}

func TestComputeSkillScore_MissingSourceIsNotPenalised(t *testing.T) {
	want := ComputeSkillScore(fullProfile())
	if math.Abs(want-60) > 0.01 {
		t.Fatalf("baseline profile: got %v, want 60", want)
	}

	cases := map[string]func(p *LeetifyProfile){
		"no Premier":        func(p *LeetifyProfile) { p.Ranks.Premier = 0 },
		"no Faceit":         func(p *LeetifyProfile) { p.Ranks.FaceitElo = nil; p.Ranks.Faceit = 0 },
		"no rank at all":    func(p *LeetifyProfile) { p.Ranks.FaceitElo = nil; p.Ranks.Faceit = 0; p.Ranks.Premier = 0 },
		"no recent matches": func(p *LeetifyProfile) { p.RecentMatches = nil },
		"no utility rating": func(p *LeetifyProfile) { p.Rating.Utility = 0 },
		"no opening duels": func(p *LeetifyProfile) {
			p.Stats.CtOpeningDuelSuccessPercentage, p.Stats.TOpeningDuelSuccessPercentage = 0, 0
		},
	}
	for name, strip := range cases {
		p := fullProfile()
		strip(p)
		if got := ComputeSkillScore(p); math.Abs(got-want) > 0.01 {
			t.Errorf("%s: score changed from %v to %v", name, want, got)
		}
	}
}

func TestComputeSkillScore_BestRankCounts(t *testing.T) {
	// Faceit at 100%, no Premier, everything else at 60%:
	// rank 1.0*55% + the remaining 45% at 0.6 = 82.
	elo := 3000
	p := fullProfile()
	p.Ranks.FaceitElo = &elo
	p.Ranks.Premier = 0
	if got := ComputeSkillScore(p); math.Abs(got-82) > 0.01 {
		t.Errorf("Faceit only: got %v, want 82", got)
	}

	// A weaker Premier rating does not pull the score down…
	p.Ranks.Premier = 21000
	if got := ComputeSkillScore(p); math.Abs(got-82) > 0.01 {
		t.Errorf("Faceit + weaker Premier: got %v, want 82", got)
	}

	// …and neither does a barely-used Faceit account next to a good Premier rating.
	low := 900
	p.Ranks.FaceitElo = &low
	p.Ranks.Premier = 35000
	if got := ComputeSkillScore(p); math.Abs(got-82) > 0.01 {
		t.Errorf("Premier + weaker Faceit: got %v, want 82", got)
	}
}

func TestComputeSkillScore_BetterStatsScoreHigher(t *testing.T) {
	weak := &LeetifyProfile{PrivacyMode: "public"}
	weak.Ranks.Premier = 5000
	weak.Rating.Aim = 30

	strong := &LeetifyProfile{PrivacyMode: "public"}
	strong.Ranks.Premier = 20000
	strong.Rating.Aim = 80

	if ComputeSkillScore(strong) <= ComputeSkillScore(weak) {
		t.Errorf("stronger profile must score higher: strong=%v weak=%v",
			ComputeSkillScore(strong), ComputeSkillScore(weak))
	}
}

func TestComputeSteamScore(t *testing.T) {
	if got := ComputeSteamScore(&CS2Stats{}); got != 0 {
		t.Errorf("empty stats: got %v, want 0", got)
	}

	// Everything at or beyond the cap → 100.
	max := &CS2Stats{KDRatio: 3, WinRate: 100, HSPercent: 70, Accuracy: 40, MVPRate: 5, HoursPlayed: 9000}
	if got := ComputeSteamScore(max); got != 100 {
		t.Errorf("capped stats: got %v, want 100", got)
	}

	// Hand-computed: KD 1.25 → .5*30, WR 50 → .5*22, HS 30 → .5*18,
	// 1250h → sqrt(1250/5000)=.5*12, acc 15 → .5*10, MVP 1 → .5*8  = 50
	mid := &CS2Stats{KDRatio: 1.25, WinRate: 50, HSPercent: 30, Accuracy: 15, MVPRate: 1, HoursPlayed: 1250}
	if got := ComputeSteamScore(mid); math.Abs(got-50) > 0.01 {
		t.Errorf("mid stats: got %v, want 50", got)
	}
}

func TestCombineScores(t *testing.T) {
	public := &LeetifyProfile{PrivacyMode: "public"}
	public.Rating.Aim = 100
	leetify := ComputeSkillScore(public) // 18.2
	private := &LeetifyProfile{PrivacyMode: "private"}
	steam := &CS2Stats{KDRatio: 1.25, WinRate: 50, HSPercent: 30, Accuracy: 15, MVPRate: 1, HoursPlayed: 1250} // 50
	faceit := &FaceitPlayer{Elo: 2400, Level: 10}                                                               // 80

	cases := []struct {
		name      string
		profile   *LeetifyProfile
		steam     *CS2Stats
		faceit    *FaceitPlayer
		wantScore float64 // -1 = no score
		wantMode  string
	}{
		{"public Leetify + Steam blends 95/5", public, steam, nil, leetify*0.95 + 50*0.05, "public"},
		{"public Leetify only", public, nil, nil, leetify, "public"},
		{"private Leetify falls back to Steam", private, steam, nil, 50, "steam_only"},
		{"no Leetify falls back to Steam", nil, steam, nil, 50, "steam_only"},
		{"Faceit beats Steam when Leetify is missing", nil, steam, faceit, 80*0.9 + 50*0.1, "faceit_only"},
		{"Faceit beats Steam when Leetify is private", private, steam, faceit, 80*0.9 + 50*0.1, "faceit_only"},
		{"Faceit alone", nil, nil, faceit, 80, "faceit_only"},
		{"private Leetify without Steam has no score", private, nil, nil, -1, "private"},
		{"no data at all", nil, nil, nil, -1, "unknown"},
	}
	for _, c := range cases {
		score, mode := CombineScores(c.profile, c.steam, c.faceit)
		if mode != c.wantMode {
			t.Errorf("%s: privacy mode %q, want %q", c.name, mode, c.wantMode)
		}
		if c.wantScore < 0 {
			if score != nil {
				t.Errorf("%s: expected no score, got %v", c.name, *score)
			}
			continue
		}
		if score == nil || math.Abs(*score-c.wantScore) > 0.01 {
			t.Errorf("%s: wrong score (want %v, got nil=%v)", c.name, c.wantScore, score == nil)
		}
	}
}

func TestComputeRecentForm(t *testing.T) {
	if got := computeRecentForm(nil); got != 0 {
		t.Errorf("no matches: got %v, want 0", got)
	}

	// Constant rating → the weighted average is that rating.
	same := make([]RecentMatch, 15)
	for i := range same {
		same[i] = RecentMatch{LeetifyRating: 0.05, DataSource: "matchmaking"}
	}
	if got := computeRecentForm(same); math.Abs(got-0.05) > 1e-9 {
		t.Errorf("constant ratings: got %v, want 0.05", got)
	}

	// Newer matches weigh more than older ones.
	improving := []RecentMatch{{LeetifyRating: 0.2}, {LeetifyRating: -0.2}}
	if got := computeRecentForm(improving); got <= 0 {
		t.Errorf("recent good match should dominate, got %v", got)
	}

	// Only the 10 most recent matches count.
	withOldOutlier := append(append([]RecentMatch{}, same[:10]...), RecentMatch{LeetifyRating: 5})
	if got := computeRecentForm(withOldOutlier); math.Abs(got-0.05) > 1e-9 {
		t.Errorf("11th match must be ignored, got %v", got)
	}

	// Faceit matches carry 20% more weight.
	mixed := []RecentMatch{{LeetifyRating: 0.1, DataSource: "faceit"}, {LeetifyRating: 0, DataSource: "matchmaking"}}
	want := (0.1 * 1.2) / (1.2 + 0.85)
	if got := computeRecentForm(mixed); math.Abs(got-want) > 1e-9 {
		t.Errorf("faceit weighting: got %v, want %v", got, want)
	}
}

func TestCombineScores_FaceitEloOverridesLeetifyCopy(t *testing.T) {
	// Leetify reports a stale 1500; Faceit itself says 3000.
	stale := 1500
	p := &LeetifyProfile{PrivacyMode: "public"}
	p.Ranks.FaceitElo = &stale
	p.Ranks.Faceit = 8

	score, _ := CombineScores(p, nil, &FaceitPlayer{Elo: 3000, Level: 10})
	if score == nil || math.Abs(*score-100) > 0.01 {
		t.Errorf("expected Faceit's own ELO to be used (score 100), got %v", *score)
	}
	if *p.Ranks.FaceitElo != 1500 || p.Ranks.Faceit != 8 {
		t.Error("the Leetify profile passed in must not be modified")
	}
}

func TestComputeFaceitScore(t *testing.T) {
	// ELO alone: 1500/3000 → 50.
	if got := ComputeFaceitScore(&FaceitPlayer{Elo: 1500}); math.Abs(got-50) > 0.01 {
		t.Errorf("elo only: got %v, want 50", got)
	}
	// Average recent form (K/D 1.0, ADR 75 → 0.5) leaves a 50 at 50.
	avg := &FaceitPlayer{Elo: 1500, Recent: &FaceitRecent{Matches: 20, KDRatio: 1.0, ADR: 75}}
	if got := ComputeFaceitScore(avg); math.Abs(got-50) > 0.01 {
		t.Errorf("average form: got %v, want 50", got)
	}
	// Strong recent form lifts the score: 0.5*75% + 1.0*25% = 62.5.
	hot := &FaceitPlayer{Elo: 1500, Recent: &FaceitRecent{Matches: 20, KDRatio: 1.4, ADR: 100}}
	if got := ComputeFaceitScore(hot); math.Abs(got-62.5) > 0.01 {
		t.Errorf("strong form: got %v, want 62.5", got)
	}
}
