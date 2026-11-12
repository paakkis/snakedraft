package services

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// --- helpers ---------------------------------------------------------------

func mkPlayers(scores ...float64) []PlayerScore {
	players := make([]PlayerScore, len(scores))
	for i, s := range scores {
		players[i] = PlayerScore{PlayerID: fmt.Sprintf("p%d", i+1), SkillScore: s}
	}
	return players
}

func req(a, b string) TeammateRequest {
	return TeammateRequest{ID: a + "-" + b, RequesterID: a, RequestedID: b}
}

func teamIndex(teams [][]PlayerScore) map[string]int {
	idx := map[string]int{}
	for t, team := range teams {
		for _, p := range team {
			idx[p.PlayerID] = t
		}
	}
	return idx
}

// assertValidDraft checks the invariants every draft must satisfy: full
// teams, every active player placed exactly once, bench + teams == input.
func assertValidDraft(t *testing.T, players []PlayerScore, res BalanceResult, teamSize int) {
	t.Helper()
	seen := map[string]bool{}
	for i, team := range res.Teams {
		if len(team) != teamSize {
			t.Errorf("team %d has %d players, want %d", i, len(team), teamSize)
		}
		for _, p := range team {
			if seen[p.PlayerID] {
				t.Errorf("player %s drafted twice", p.PlayerID)
			}
			seen[p.PlayerID] = true
		}
	}
	for _, p := range res.Bench {
		if seen[p.PlayerID] {
			t.Errorf("benched player %s is also on a team", p.PlayerID)
		}
		seen[p.PlayerID] = true
	}
	if len(seen) != len(players) {
		t.Errorf("draft covers %d players, want %d", len(seen), len(players))
	}
	if len(res.Teams) != len(players)/teamSize {
		t.Errorf("got %d teams, want %d", len(res.Teams), len(players)/teamSize)
	}
}

func statusOf(res BalanceResult, id string) RequestStatus {
	for _, o := range res.Requests {
		if o.ID == id {
			return o.Status
		}
	}
	return ""
}

// --- chain rule ------------------------------------------------------------

func TestWouldFormFullTeam(t *testing.T) {
	chain := []TeammateRequest{req("a", "b"), req("b", "c"), req("c", "d")} // a-b-c-d

	cases := []struct {
		name     string
		existing []TeammateRequest
		a, b     string
		teamSize int
		want     bool
	}{
		{"first request is fine", nil, "a", "b", 5, false},
		{"group of 4 is allowed in 5v5", chain[:2], "c", "d", 5, false},
		{"fifth friend completes a chain", chain, "d", "e", 5, true},
		{"chain rule ignores request direction", chain, "e", "a", 5, true},
		{"joining two groups into a full team", []TeammateRequest{req("a", "b"), req("b", "c"), req("d", "e")}, "c", "d", 5, true},
		{"two separate small groups stay legal", []TeammateRequest{req("a", "b"), req("c", "d")}, "e", "f", 5, false},
		{"link inside an existing group adds nobody", chain, "a", "d", 5, false},
		{"limit follows team size", []TeammateRequest{req("a", "b")}, "b", "c", 3, true},
		{"no pairs possible in 2v2", nil, "a", "b", 2, true},
	}
	for _, c := range cases {
		if got := WouldFormFullTeam(c.existing, c.a, c.b, c.teamSize); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBalance_RejectsRequestThatCompletesAChain(t *testing.T) {
	// Ten identical players: fairness can never be the reason to split.
	players := mkPlayers(50, 50, 50, 50, 50, 50, 50, 50, 50, 50)
	requests := []TeammateRequest{
		req("p1", "p2"), req("p2", "p3"), req("p3", "p4"),
		req("p4", "p5"), // would make five friends
	}

	res, err := BalanceWithRequests(players, requests, 5, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 5)

	for _, id := range []string{"p1-p2", "p2-p3", "p3-p4"} {
		if s := statusOf(res, id); s != RequestHonored {
			t.Errorf("request %s: got %q, want honored", id, s)
		}
	}
	if s := statusOf(res, "p4-p5"); s != RequestIllegalChain {
		t.Errorf("chain-completing request: got %q, want illegal_chain", s)
	}
	idx := teamIndex(res.Teams)
	if idx["p5"] == idx["p1"] {
		t.Error("five chained friends ended up on one team")
	}
}

// --- honouring requests ----------------------------------------------------

func TestBalance_NoRequestsMatchesPlainDraft(t *testing.T) {
	players := mkPlayers(90, 85, 80, 72, 66, 61, 55, 50, 44, 40, 33, 20)
	res, err := BalanceWithRequests(players, nil, 4, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 4)
	if res.Spread != res.BaselineSpread {
		t.Errorf("without requests the baseline must be used: spread %.3f vs baseline %.3f", res.Spread, res.BaselineSpread)
	}
	if len(res.Requests) != 0 {
		t.Errorf("expected no request outcomes, got %d", len(res.Requests))
	}
}

func TestBalance_HonorsRequestsWhenFair(t *testing.T) {
	players := mkPlayers(90, 85, 80, 72, 66, 61, 55, 50, 44, 40, 33, 20, 18, 15, 12)
	requests := []TeammateRequest{
		req("p1", "p15"),                 // best + worst
		req("p4", "p8"), req("p8", "p9"), // trio
		req("p6", "p7"),
	}

	res, err := BalanceWithRequests(players, requests, 5, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 5)

	idx := teamIndex(res.Teams)
	for _, r := range requests {
		if statusOf(res, r.ID) != RequestHonored {
			t.Errorf("request %s: got %q, want honored", r.ID, statusOf(res, r.ID))
		}
		if idx[r.RequesterID] != idx[r.RequestedID] {
			t.Errorf("request %s reported honored but players are on different teams", r.ID)
		}
	}
	if res.Spread > math.Max(DefaultMaxSpread, res.BaselineSpread)+1e-9 {
		t.Errorf("spread %.2f exceeds fairness limit", res.Spread)
	}
}

func TestBalance_DropsRequestThatBreaksFairness(t *testing.T) {
	// Two stars and eight average players: the stars together cannot be
	// balanced (best case 45+45+10+10 vs 4x10).
	players := mkPlayers(45, 45, 10, 10, 10, 10, 10, 10)
	requests := []TeammateRequest{req("p1", "p2")}

	res, err := BalanceWithRequests(players, requests, 4, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 4)

	if s := statusOf(res, "p1-p2"); s != RequestDroppedForBalance {
		t.Errorf("got %q, want dropped_for_balance", s)
	}
	idx := teamIndex(res.Teams)
	if idx["p1"] == idx["p2"] {
		t.Error("stars were kept together despite the unfair result")
	}
	if res.Spread > 1e-9 {
		t.Errorf("expected perfectly balanced teams once the request is dropped, spread=%.2f", res.Spread)
	}
}

func TestBalance_DropsOnlyTheHarmfulRequest(t *testing.T) {
	players := mkPlayers(45, 45, 10, 10, 10, 10, 10, 10)
	requests := []TeammateRequest{
		req("p3", "p4"), // harmless
		req("p1", "p2"), // unbalanceable
		req("p5", "p6"), // harmless
	}

	res, err := BalanceWithRequests(players, requests, 4, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 4)

	if s := statusOf(res, "p1-p2"); s != RequestDroppedForBalance {
		t.Errorf("harmful request: got %q, want dropped_for_balance", s)
	}
	for _, id := range []string{"p3-p4", "p5-p6"} {
		if s := statusOf(res, id); s != RequestHonored {
			t.Errorf("harmless request %s: got %q, want honored", id, s)
		}
	}
	if res.Spread > 1e-9 {
		t.Errorf("expected balanced teams, spread=%.2f", res.Spread)
	}
}

func TestBalance_MaxSpreadControlsStrictness(t *testing.T) {
	players := mkPlayers(45, 45, 10, 10, 10, 10, 10, 10) // stars together → spread 17.5
	requests := []TeammateRequest{req("p1", "p2")}

	lenient, err := BalanceWithRequests(players, requests, 4, BalanceOptions{MaxSpread: 20})
	if err != nil {
		t.Fatal(err)
	}
	if s := statusOf(lenient, "p1-p2"); s != RequestHonored {
		t.Errorf("with a generous limit the request should be honored, got %q", s)
	}
	if math.Abs(lenient.Spread-17.5) > 1e-9 {
		t.Errorf("spread = %.2f, want 17.5", lenient.Spread)
	}
}

func TestBalance_RequestsNeverMakeAnUnfairBaselineWorse(t *testing.T) {
	// One superstar: even the request-free draft is far beyond MaxSpread.
	// A request must not be used as an excuse to make it worse still.
	players := mkPlayers(100, 20, 10, 10, 10, 10)
	requests := []TeammateRequest{req("p1", "p2")}

	res, err := BalanceWithRequests(players, requests, 3, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Spread > res.BaselineSpread+1e-9 {
		t.Errorf("spread %.2f is worse than the baseline %.2f", res.Spread, res.BaselineSpread)
	}
	if s := statusOf(res, "p1-p2"); s != RequestDroppedForBalance {
		t.Errorf("got %q, want dropped_for_balance", s)
	}
}

func TestBalance_GroupsThatCannotBePackedAreSplit(t *testing.T) {
	// Three trios cannot fit into two teams of five (3+3 > 5), so at least
	// one trio has to give way even though all players are equal.
	players := mkPlayers(50, 50, 50, 50, 50, 50, 50, 50, 50, 50)
	requests := []TeammateRequest{
		req("p1", "p2"), req("p2", "p3"),
		req("p4", "p5"), req("p5", "p6"),
		req("p7", "p8"), req("p8", "p9"),
	}

	res, err := BalanceWithRequests(players, requests, 5, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 5)

	honored := 0
	for _, o := range res.Requests {
		if o.Status == RequestHonored {
			honored++
		}
	}
	if honored < 4 {
		t.Errorf("expected at least 4 of 6 requests honored, got %d", honored)
	}
}

func TestBalance_RequestsInvolvingUnavailablePlayers(t *testing.T) {
	// 11 players, 5v5 → the lowest player (p11) is benched.
	players := mkPlayers(60, 58, 55, 52, 50, 48, 45, 42, 40, 38, 5)
	requests := []TeammateRequest{
		req("p1", "p11"),   // benched
		req("p2", "ghost"), // not in the draft (e.g. no skill score)
		req("p3", "p3"),    // self
		req("p4", "p5"),    // fine
	}

	res, err := BalanceWithRequests(players, requests, 5, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 5)

	if len(res.Bench) != 1 || res.Bench[0].PlayerID != "p11" {
		t.Fatalf("expected p11 benched, got %+v", res.Bench)
	}
	for _, id := range []string{"p1-p11", "p2-ghost", "p3-p3"} {
		if s := statusOf(res, id); s != RequestPlayerUnavailable {
			t.Errorf("request %s: got %q, want player_unavailable", id, s)
		}
	}
	if s := statusOf(res, "p4-p5"); s != RequestHonored {
		t.Errorf("request p4-p5: got %q, want honored", s)
	}
}

func TestBalance_OutcomesKeepRequestOrder(t *testing.T) {
	players := mkPlayers(60, 58, 55, 52, 50, 48, 45, 42)
	requests := []TeammateRequest{req("p8", "p1"), req("p2", "nobody"), req("p3", "p4")}

	res, err := BalanceWithRequests(players, requests, 4, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Requests) != len(requests) {
		t.Fatalf("got %d outcomes, want %d", len(res.Requests), len(requests))
	}
	for i, o := range res.Requests {
		if o.ID != requests[i].ID {
			t.Errorf("outcome %d is for %s, want %s", i, o.ID, requests[i].ID)
		}
		if o.Status == "" {
			t.Errorf("outcome %d has no status", i)
		}
	}
}

func TestBalance_BenchesLowestPlayers(t *testing.T) {
	players := mkPlayers(10, 90, 30, 70, 50, 20, 80) // unsorted on purpose
	res, err := BalanceWithRequests(players, nil, 3, BalanceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertValidDraft(t, players, res, 3)
	if len(res.Bench) != 1 || res.Bench[0].SkillScore != 10 {
		t.Errorf("expected the 10-score player benched, got %+v", res.Bench)
	}
}

func TestBalance_NotEnoughPlayers(t *testing.T) {
	if _, err := BalanceWithRequests(mkPlayers(50, 50, 50, 50, 50, 50, 50, 50, 50), nil, 5, BalanceOptions{}); err != ErrNotEnoughPlayers {
		t.Errorf("9 players in 5v5: got err=%v, want ErrNotEnoughPlayers", err)
	}
	if _, err := BalanceWithRequests(mkPlayers(50, 50), nil, 0, BalanceOptions{}); err == nil {
		t.Error("team size 0 must be rejected")
	}
}

// Randomised tournaments: whatever the requests, the draft must be valid,
// never contain a full team of chained friends, be honest about which
// requests were honoured, and stay within the fairness limit.
func TestBalance_RandomTournamentsKeepInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for round := 0; round < 150; round++ {
		teamSize := 2 + rng.Intn(4) // 2..5
		n := teamSize*(2+rng.Intn(5)) + rng.Intn(teamSize)
		scores := make([]float64, n)
		for i := range scores {
			scores[i] = math.Round(rng.Float64()*7000) / 100
		}
		players := mkPlayers(scores...)

		var requests []TeammateRequest
		for i := 0; i < rng.Intn(n); i++ {
			a, b := rng.Intn(n)+1, rng.Intn(n)+1
			requests = append(requests, TeammateRequest{
				ID:          fmt.Sprintf("r%d", i),
				RequesterID: fmt.Sprintf("p%d", a),
				RequestedID: fmt.Sprintf("p%d", b),
			})
		}

		res, err := BalanceWithRequests(players, requests, teamSize, BalanceOptions{Iterations: 2000})
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		assertValidDraft(t, players, res, teamSize)

		limit := math.Max(DefaultMaxSpread, res.BaselineSpread) + 1e-6
		if res.Spread > limit {
			t.Errorf("round %d: spread %.3f exceeds limit %.3f", round, res.Spread, limit)
		}

		idx := teamIndex(res.Teams)
		accepted := newFriendGroups()
		for i, o := range res.Requests {
			_, okA := idx[o.RequesterID]
			_, okB := idx[o.RequestedID]
			together := okA && okB && o.RequesterID != o.RequestedID && idx[o.RequesterID] == idx[o.RequestedID]
			switch o.Status {
			case RequestHonored:
				if !together {
					t.Errorf("round %d: request %d reported honored but players are apart", round, i)
				}
				accepted.union(o.RequesterID, o.RequestedID)
			case RequestDroppedForBalance:
				if together {
					t.Errorf("round %d: request %d reported dropped but players are together", round, i)
				}
			case RequestPlayerUnavailable:
				if okA && okB && o.RequesterID != o.RequestedID {
					t.Errorf("round %d: request %d reported unavailable but both players were drafted", round, i)
				}
			case RequestIllegalChain:
			default:
				t.Errorf("round %d: request %d has unexpected status %q", round, i, o.Status)
			}
		}
		for id, size := range accepted.size {
			if accepted.find(id) == id && size >= teamSize {
				t.Errorf("round %d: %d chained friends fill a %d-player team", round, size, teamSize)
			}
		}
	}
}

func TestTeamSpread(t *testing.T) {
	teams := [][]PlayerScore{
		{{SkillScore: 80}, {SkillScore: 60}}, // avg 70
		{{SkillScore: 50}, {SkillScore: 40}}, // avg 45
		{{SkillScore: 60}, {SkillScore: 60}}, // avg 60
	}
	if got := TeamSpread(teams); got != 25 {
		t.Errorf("TeamSpread = %v, want 25", got)
	}
	if got := TeamSpread(nil); got != 0 {
		t.Errorf("TeamSpread(nil) = %v, want 0", got)
	}
}
