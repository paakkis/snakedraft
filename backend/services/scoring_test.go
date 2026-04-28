package services

import (
	"testing"
)

func TestComputeSkillScore_PrivateProfile(t *testing.T) {
	p := &LeetifyProfile{PrivacyMode: "private"}
	score := ComputeSkillScore(p)
	if score != -1 {
		t.Errorf("expected -1 for private profile, got %f", score)
	}
}

func TestComputeSkillScore_ZeroProfile(t *testing.T) {
	p := &LeetifyProfile{PrivacyMode: "public"}
	score := ComputeSkillScore(p)
	if score < 0 || score > 100 {
		t.Errorf("score out of range [0,100]: %f", score)
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
