package services

import (
	"math"
	"testing"
)

func TestSnakeDraft_TwoTeams(t *testing.T) {
	players := []PlayerScore{
		{PlayerID: "p1", SkillScore: 90},
		{PlayerID: "p2", SkillScore: 80},
		{PlayerID: "p3", SkillScore: 70},
		{PlayerID: "p4", SkillScore: 60},
	}

	teams := SnakeDraft(players, 2)

	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}
	if len(teams[0]) != 2 || len(teams[1]) != 2 {
		t.Fatalf("expected 2 players per team, got %d, %d", len(teams[0]), len(teams[1]))
	}

	// Snake: T1←90, T2←80, T2←70, T1←60
	// T1=[90,60] avg=75, T2=[80,70] avg=75 → perfectly balanced
	avg0 := teamAvg(teams[0])
	avg1 := teamAvg(teams[1])
	if math.Abs(avg0-avg1) > 0.01 {
		t.Errorf("expected balanced teams, got avg %.1f vs %.1f", avg0, avg1)
	}
}

func TestSnakeDraft_FourTeams(t *testing.T) {
	players := make([]PlayerScore, 8)
	for i := range players {
		players[i] = PlayerScore{PlayerID: string(rune('a' + i)), SkillScore: float64(100 - i*10)}
	}

	teams := SnakeDraft(players, 4)

	if len(teams) != 4 {
		t.Fatalf("expected 4 teams, got %d", len(teams))
	}
	for i, t2 := range teams {
		if len(t2) != 2 {
			t.Errorf("team %d: expected 2 players, got %d", i, len(t2))
		}
	}
}

func TestSnakeDraft_SortsDescending(t *testing.T) {
	// Pass players in wrong order; SnakeDraft must sort them
	players := []PlayerScore{
		{PlayerID: "low", SkillScore: 10},
		{PlayerID: "high", SkillScore: 90},
	}

	teams := SnakeDraft(players, 2)

	// T1 should receive the highest player first (round 0 L→R → pos 0)
	if teams[0][0].PlayerID != "high" {
		t.Errorf("expected team 0 to receive highest player first, got %s", teams[0][0].PlayerID)
	}
}

func TestOptimizeTeams_ReducesVariance(t *testing.T) {
	// Deliberately unbalanced starting point
	teams := [][]PlayerScore{
		{{PlayerID: "a", SkillScore: 90}, {PlayerID: "b", SkillScore: 80}},
		{{PlayerID: "c", SkillScore: 30}, {PlayerID: "d", SkillScore: 20}},
	}
	before := TeamVariance(teams)
	after := TeamVariance(OptimizeTeams(teams, 5000))

	if after >= before {
		t.Errorf("expected optimization to reduce variance: before=%.4f after=%.4f", before, after)
	}
}

func TestTeamVariance_PerfectBalance(t *testing.T) {
	teams := [][]PlayerScore{
		{{PlayerID: "a", SkillScore: 75}},
		{{PlayerID: "b", SkillScore: 75}},
	}
	v := TeamVariance(teams)
	if v != 0 {
		t.Errorf("expected 0 variance for equal teams, got %f", v)
	}
}

func TestTeamVariance_SingleTeam(t *testing.T) {
	teams := [][]PlayerScore{
		{{PlayerID: "a", SkillScore: 50}, {PlayerID: "b", SkillScore: 60}},
	}
	v := TeamVariance(teams)
	if v != 0 {
		t.Errorf("expected 0 variance for single team, got %f", v)
	}
}

// teamAvg is a test helper.
func teamAvg(team []PlayerScore) float64 {
	if len(team) == 0 {
		return 0
	}
	sum := 0.0
	for _, p := range team {
		sum += p.SkillScore
	}
	return sum / float64(len(team))
}
