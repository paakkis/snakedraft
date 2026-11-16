package services

import (
	"math"
	"testing"
)

// rankOnly are settings that score on the listed rank and nothing else.
func rankOnly(faceit, premier bool) ScoreSettings {
	return ScoreSettings{UseFaceit: faceit, UsePremier: premier, WeightRank: 1}
}

func TestScoreSettings_RankOnly(t *testing.T) {
	fixClock(t, testNow)

	// Faceit 1500 (0.5) over 5 matches while performing like 2000, Premier
	// 21000 (0.6), and mechanics far above both.
	p := fullProfile()
	p.Ranks.Premier = 21000
	p.Rating.Aim, p.Rating.Positioning = 95, 95
	f := &FaceitPlayer{Elo: 1500, LifetimeMatches: 5,
		Recent:      &FaceitRecent{Matches: 5, KDRatio: 1.4, ADR: 100},
		Performance: &FaceitPerformance{Matches: 5, ImpliedElo: 2000}}
	steam := &CS2Stats{KDRatio: 2.5, WinRate: 100, HSPercent: 60, Accuracy: 30, MVPRate: 2, HoursPlayed: 5000}

	cases := []struct {
		name     string
		settings ScoreSettings
		want     float64
	}{
		{"Faceit only is the listed ELO", rankOnly(true, false), 50},
		{"Premier only is the listed rating", rankOnly(false, true), 60},
		{"both is the better of the two", rankOnly(true, true), 60},
	}
	for _, c := range cases {
		got, _ := c.settings.Combine(p, steam, f)
		if got == nil || math.Abs(*got-c.want) > 0.01 {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}

	// The same without a Leetify profile.
	if got, mode := rankOnly(true, false).Combine(nil, steam, f); got == nil || math.Abs(*got-50) > 0.01 || mode != "faceit_only" {
		t.Errorf("Faceit only without Leetify: got %v (%s), want 50", got, mode)
	}
}

func TestScoreSettings_PlayerWithoutTheRankHasNoScore(t *testing.T) {
	fixClock(t, testNow)
	p := fullProfile()
	p.Ranks.Faceit, p.Ranks.FaceitElo = 0, nil // Premier only
	steam := &CS2Stats{KDRatio: 1, WinRate: 50}

	if got, _ := rankOnly(true, false).Combine(p, steam, nil); got != nil {
		t.Errorf("Faceit only, player without Faceit: got %v, want no score", *got)
	}
	f := &FaceitPlayer{Elo: 1500, Recent: &FaceitRecent{Matches: 5, KDRatio: 1.4, ADR: 100}}
	if got, _ := rankOnly(false, true).Combine(nil, steam, f); got != nil {
		t.Errorf("Premier only, player with only Faceit: got %v, want no score", *got)
	}
	// Nothing switched on at all.
	if got, _ := (ScoreSettings{}).Combine(p, steam, f); got != nil {
		t.Errorf("empty settings: got %v, want no score", *got)
	}
}

func TestScoreSettings_Toggles(t *testing.T) {
	fixClock(t, testNow)
	base := DefaultScoreSettings()

	// Steam off: a Steam-only player has no score, and others lose its share.
	steam := &CS2Stats{KDRatio: 2.5, WinRate: 100, HSPercent: 60, Accuracy: 30, MVPRate: 2, HoursPlayed: 5000}
	noSteam := base
	noSteam.UseSteam = false
	if got, _ := noSteam.Combine(nil, steam, nil); got != nil {
		t.Errorf("Steam off: got %v, want no score", *got)
	}
	f := &FaceitPlayer{Elo: 1500}
	if got, _ := noSteam.Combine(nil, steam, f); got == nil || math.Abs(*got-50) > 0.01 {
		t.Errorf("Steam off, Faceit player: got %v, want 50", got)
	}

	// Confidence off: a 5-match rank counts in full.
	p := &LeetifyProfile{PrivacyMode: "public"}
	p.Rating.Aim, p.Rating.Positioning = 80, 80
	thin := &FaceitPlayer{Elo: 1000, LifetimeMatches: 5}
	s := base
	s.ScaleByConfidence, s.AdjustForMechanics = false, false
	want := (0.55/3 + 0.25*0.8) / 0.8 * 100
	if got := s.SkillScore(p, thin); math.Abs(got-want) > 0.01 {
		t.Errorf("confidence off: got %v, want %v", got, want)
	}

	// Performance off: the listed ELO stands and nobody is flagged.
	s = base
	s.AdjustForPerformance = false
	perf := &FaceitPlayer{Elo: 1000, LifetimeMatches: 5, Performance: &FaceitPerformance{Matches: 5, ImpliedElo: 2000}}
	if a := s.AssessRank(nil, perf); !near(a.Value, a.Listed) || a.Outperforming {
		t.Errorf("performance off: got %+v", a)
	}

	// A weight of 0 leaves the component out; only ratios matter otherwise.
	s = ScoreSettings{UsePremier: true, WeightRank: 3, WeightAim: 1}
	q := fullProfile() // Premier 21000 (0.6)
	q.Rating.Aim = 80
	want = (3*0.6 + 0.8) / 4 * 100
	if got := s.SkillScore(q, nil); math.Abs(got-want) > 0.01 {
		t.Errorf("custom weights: got %v, want %v", got, want)
	}
}

func TestSettings_Sanitized(t *testing.T) {
	s := DraftSettings{ScoreSettings: ScoreSettings{WeightRank: -1, WeightAim: math.NaN(), WeightForm: 1e9, WeightUtility: 0.5}, MaxSpread: -3}
	s = s.Sanitized()
	if s.WeightRank != 0 || s.WeightAim != 0 || s.WeightForm != maxWeight || s.WeightUtility != 0.5 || s.MaxSpread != DefaultMaxSpread {
		t.Errorf("got %+v", s)
	}
	if d := DefaultDraftSettings(); d.Sanitized() != d {
		t.Errorf("the defaults must already be valid")
	}
}
