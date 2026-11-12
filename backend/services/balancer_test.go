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

func TestSnakeDraft_PickOrder(t *testing.T) {
	// 3 teams, 9 players scored 9..1. Expected snake:
	//   round 1 (L→R): T1←9 T2←8 T3←7
	//   round 2 (R→L): T3←6 T2←5 T1←4
	//   round 3 (L→R): T1←3 T2←2 T3←1
	players := make([]PlayerScore, 9)
	for i := range players {
		players[i] = PlayerScore{PlayerID: string(rune('a' + i)), SkillScore: float64(i + 1)}
	}
	teams := SnakeDraft(players, 3)

	want := [][]float64{{9, 4, 3}, {8, 5, 2}, {7, 6, 1}}
	for ti, team := range teams {
		if len(team) != len(want[ti]) {
			t.Fatalf("team %d: got %d players, want %d", ti, len(team), len(want[ti]))
		}
		for pi, p := range team {
			if p.SkillScore != want[ti][pi] {
				t.Errorf("team %d pick %d: got %v, want %v", ti, pi, p.SkillScore, want[ti][pi])
			}
		}
	}
}

func TestSnakeDraft_DoesNotMutateInput(t *testing.T) {
	players := []PlayerScore{{PlayerID: "low", SkillScore: 10}, {PlayerID: "high", SkillScore: 90}}
	SnakeDraft(players, 2)
	if players[0].PlayerID != "low" || players[1].PlayerID != "high" {
		t.Errorf("input slice was reordered: %+v", players)
	}
}

func TestSnakeDraft_UnevenPlayerCount(t *testing.T) {
	players := make([]PlayerScore, 7)
	for i := range players {
		players[i] = PlayerScore{PlayerID: string(rune('a' + i)), SkillScore: float64(70 - i*10)}
	}
	teams := SnakeDraft(players, 3)
	total := 0
	for _, team := range teams {
		total += len(team)
		if len(team) < 2 || len(team) > 3 {
			t.Errorf("team sizes should differ by at most one, got %d", len(team))
		}
	}
	if total != 7 {
		t.Errorf("drafted %d players, want 7", total)
	}
}

func TestSnakeDraft_DegenerateInputs(t *testing.T) {
	if teams := SnakeDraft(nil, 3); len(teams) != 3 {
		t.Errorf("no players: expected 3 empty teams, got %d", len(teams))
	}
	if teams := SnakeDraft([]PlayerScore{{PlayerID: "a", SkillScore: 1}}, 0); len(teams) != 0 {
		t.Errorf("zero teams: expected no teams, got %d", len(teams))
	}
}

func TestOptimizeTeams_KeepsPlayersAndTeamSizes(t *testing.T) {
	players := make([]PlayerScore, 20)
	for i := range players {
		players[i] = PlayerScore{PlayerID: string(rune('a' + i)), SkillScore: float64((i * 37) % 71)}
	}
	drafted := SnakeDraft(players, 4)
	before := TeamVariance(drafted)
	optimized := OptimizeTeams(drafted, 5000)

	if after := TeamVariance(optimized); after > before {
		t.Errorf("optimization made variance worse: before=%.4f after=%.4f", before, after)
	}
	seen := map[string]bool{}
	for i, team := range optimized {
		if len(team) != 5 {
			t.Errorf("team %d: size changed to %d", i, len(team))
		}
		for _, p := range team {
			if seen[p.PlayerID] {
				t.Errorf("player %s appears twice", p.PlayerID)
			}
			seen[p.PlayerID] = true
		}
	}
	if len(seen) != 20 {
		t.Errorf("expected 20 distinct players, got %d", len(seen))
	}
}

func TestOptimizeTeams_DoesNotMutateInput(t *testing.T) {
	teams := [][]PlayerScore{
		{{PlayerID: "a", SkillScore: 90}, {PlayerID: "b", SkillScore: 80}},
		{{PlayerID: "c", SkillScore: 30}, {PlayerID: "d", SkillScore: 20}},
	}
	OptimizeTeams(teams, 1000)
	if teams[0][0].PlayerID != "a" || teams[0][1].PlayerID != "b" || teams[1][0].PlayerID != "c" || teams[1][1].PlayerID != "d" {
		t.Errorf("input teams were modified: %+v", teams)
	}
}

func TestOptimizeTeams_SingleTeam(t *testing.T) {
	teams := [][]PlayerScore{{{PlayerID: "a", SkillScore: 90}}}
	if got := OptimizeTeams(teams, 100); len(got) != 1 || len(got[0]) != 1 {
		t.Errorf("single team should be returned unchanged, got %+v", got)
	}
}

func TestTeamVariance_KnownValue(t *testing.T) {
	teams := [][]PlayerScore{
		{{SkillScore: 80}, {SkillScore: 60}}, // avg 70
		{{SkillScore: 40}, {SkillScore: 60}}, // avg 50
	}
	if v := TeamVariance(teams); v != 100 { // ((70-60)² + (50-60)²) / 2
		t.Errorf("TeamVariance = %v, want 100", v)
	}
	if v := TeamVariance(nil); v != 0 {
		t.Errorf("TeamVariance(nil) = %v, want 0", v)
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
