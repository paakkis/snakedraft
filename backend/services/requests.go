package services

import (
	"errors"
	"math"
	"sort"
)

// DefaultMaxSpread is the largest gap (in skill-score points, 0–100 scale)
// between the strongest and weakest team average that still counts as a fair
// draft when honouring teammate requests.
const DefaultMaxSpread = 2.0

// ErrNotEnoughPlayers is returned when fewer than two full teams can be formed.
var ErrNotEnoughPlayers = errors.New("not enough players with skill scores to form at least 2 teams")

// TeammateRequest is a wish from one player to be on the same team as another.
// For drafting purposes the link is undirected.
type TeammateRequest struct {
	ID          string
	RequesterID string
	RequestedID string
}

type RequestStatus string

const (
	// The two players ended up on the same team.
	RequestHonored RequestStatus = "honored"
	// Keeping the players together would have made the teams unfair.
	RequestDroppedForBalance RequestStatus = "dropped_for_balance"
	// The request would chain a full team of friends together.
	RequestIllegalChain RequestStatus = "illegal_chain"
	// One of the players is benched, has no skill score, or is unknown.
	RequestPlayerUnavailable RequestStatus = "player_unavailable"
)

type RequestOutcome struct {
	TeammateRequest
	Status RequestStatus
}

type BalanceOptions struct {
	// MaxSpread overrides DefaultMaxSpread when > 0.
	MaxSpread float64
	// Iterations for the request-free baseline optimisation (default 10000).
	Iterations int
}

type BalanceResult struct {
	Teams    [][]PlayerScore
	Bench    []PlayerScore
	Requests []RequestOutcome
	// Variance of team averages in the final teams.
	Variance float64
	// Spread is max team average − min team average in the final teams.
	Spread float64
	// BaselineSpread is the spread achieved when ignoring all requests.
	BaselineSpread float64
}

// WouldFormFullTeam reports whether linking players a and b, on top of the
// existing requests, would chain together a group of friends big enough to
// fill a whole team (teamSize or more players). Such chains are not allowed:
// a friend group may have at most teamSize-1 members.
func WouldFormFullTeam(existing []TeammateRequest, a, b string, teamSize int) bool {
	g := newFriendGroups()
	for _, r := range existing {
		g.union(r.RequesterID, r.RequestedID)
	}
	return g.mergedSize(a, b) >= teamSize
}

// BalanceWithRequests drafts balanced teams of exactly teamSize players while
// honouring as many teammate requests as fairness allows.
//
//  1. The lowest-scored players that don't fill a full team are benched.
//  2. A request-free baseline is drafted (SnakeDraft + OptimizeTeams).
//  3. Requests are accepted in order; any request that would chain a full
//     team of friends together is rejected.
//  4. Teams are drafted with every friend group kept together. The draft is
//     fair if its spread is at most max(MaxSpread, baseline spread). While it
//     is not fair, the request whose removal helps balance the most is
//     dropped. Dropped requests are then retried in case they fit after all.
//
// With no usable requests the result is exactly the baseline draft.
func BalanceWithRequests(players []PlayerScore, requests []TeammateRequest, teamSize int, opts BalanceOptions) (BalanceResult, error) {
	if teamSize < 1 {
		return BalanceResult{}, errors.New("team size must be at least 1")
	}
	teamCount := len(players) / teamSize
	if teamCount < 2 {
		return BalanceResult{}, ErrNotEnoughPlayers
	}
	maxSpread := opts.MaxSpread
	if maxSpread <= 0 {
		maxSpread = DefaultMaxSpread
	}
	iterations := opts.Iterations
	if iterations <= 0 {
		iterations = 10000
	}

	sorted := make([]PlayerScore, len(players))
	copy(sorted, players)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].SkillScore != sorted[j].SkillScore {
			return sorted[i].SkillScore > sorted[j].SkillScore
		}
		return sorted[i].PlayerID < sorted[j].PlayerID
	})
	active := sorted[:teamCount*teamSize]
	bench := sorted[teamCount*teamSize:]

	index := make(map[string]int, len(active))
	for i, p := range active {
		index[p.PlayerID] = i
	}

	baseline := OptimizeTeams(SnakeDraft(active, teamCount), iterations)
	baselineSpread := TeamSpread(baseline)

	// Classify requests: unavailable players, illegal chains, usable edges.
	outcomes := make([]RequestOutcome, len(requests))
	var edges []edge
	groups := newFriendGroups()
	for i, r := range requests {
		outcomes[i] = RequestOutcome{TeammateRequest: r}
		a, okA := index[r.RequesterID]
		b, okB := index[r.RequestedID]
		if !okA || !okB || a == b {
			outcomes[i].Status = RequestPlayerUnavailable
			continue
		}
		if groups.mergedSize(r.RequesterID, r.RequestedID) >= teamSize {
			outcomes[i].Status = RequestIllegalChain
			continue
		}
		groups.union(r.RequesterID, r.RequestedID)
		edges = append(edges, edge{a: a, b: b, req: i})
	}

	teams := baseline
	if len(edges) > 0 {
		limit := math.Max(maxSpread, baselineSpread) + 1e-9
		if kept, drafted := fitRequests(active, edges, teamCount, teamSize, limit); len(kept) > 0 {
			teams = drafted
		}
	}

	// Report what actually happened: a dropped request can still end up
	// honoured by chance.
	teamOf := make(map[string]int, len(active))
	for t, team := range teams {
		for _, p := range team {
			teamOf[p.PlayerID] = t
		}
	}
	for _, e := range edges {
		o := &outcomes[e.req]
		if teamOf[o.RequesterID] == teamOf[o.RequestedID] {
			o.Status = RequestHonored
		} else {
			o.Status = RequestDroppedForBalance
		}
	}

	return BalanceResult{
		Teams:          teams,
		Bench:          bench,
		Requests:       outcomes,
		Variance:       TeamVariance(teams),
		Spread:         TeamSpread(teams),
		BaselineSpread: baselineSpread,
	}, nil
}

// TeamSpread returns the gap between the highest and lowest team average.
func TeamSpread(teams [][]PlayerScore) float64 {
	if len(teams) == 0 {
		return 0
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, team := range teams {
		avg := 0.0
		for _, p := range team {
			avg += p.SkillScore
		}
		if len(team) > 0 {
			avg /= float64(len(team))
		}
		lo = math.Min(lo, avg)
		hi = math.Max(hi, avg)
	}
	return hi - lo
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

// edge is a usable request between two active players (indices into the
// active slice); req is the index of the originating request.
type edge struct {
	a, b, req int
}

// fitRequests finds a large set of edges that can be honoured with a spread
// within limit, and returns that set together with the drafted teams.
// Returns no edges when no request can be honoured fairly.
func fitRequests(active []PlayerScore, edges []edge, teamCount, teamSize int, limit float64) ([]edge, [][]PlayerScore) {
	kept := append([]edge(nil), edges...)
	var dropped []edge

	teams, spread := draftGrouped(active, kept, teamCount, teamSize)
	for len(kept) > 0 && spread > limit {
		// Drop the edge whose removal gives the fairest draft. Ties go to
		// the most recent request (first come, first served).
		best, bestSpread := -1, math.Inf(1)
		var bestTeams [][]PlayerScore
		for i := len(kept) - 1; i >= 0; i-- {
			t, s := draftGrouped(active, without(kept, i), teamCount, teamSize)
			if s < bestSpread {
				best, bestSpread, bestTeams = i, s, t
			}
		}
		if best < 0 {
			// Nothing packs into teams at all yet: shrink the largest group.
			best = lastEdgeOfLargestGroup(active, kept)
		}
		dropped = append(dropped, kept[best])
		kept = without(kept, best)
		teams, spread = bestTeams, bestSpread
		if teams == nil {
			teams, spread = draftGrouped(active, kept, teamCount, teamSize)
		}
	}

	// Dropping is greedy, so an earlier casualty may fit again by now.
	sort.Slice(dropped, func(i, j int) bool { return dropped[i].req < dropped[j].req })
	for _, e := range dropped {
		candidate := append(append([]edge(nil), kept...), e)
		if t, s := draftGrouped(active, candidate, teamCount, teamSize); s <= limit {
			kept, teams = candidate, t
		}
	}
	return kept, teams
}

func without(edges []edge, i int) []edge {
	out := make([]edge, 0, len(edges)-1)
	out = append(out, edges[:i]...)
	return append(out, edges[i+1:]...)
}

func lastEdgeOfLargestGroup(active []PlayerScore, edges []edge) int {
	g := newFriendGroups()
	for _, e := range edges {
		g.union(active[e.a].PlayerID, active[e.b].PlayerID)
	}
	best, bestSize := 0, 0
	for i, e := range edges {
		if s := g.size[g.find(active[e.a].PlayerID)]; s >= bestSize {
			best, bestSize = i, s
		}
	}
	return best
}

// unit is a set of players that must stay on the same team: either a friend
// group or a single player.
type unit struct {
	members []int // indices into active
	sum     float64
}

// draftGrouped drafts teams of exactly teamSize in which every group of
// players connected by edges shares a team, then balances them. It returns
// the teams and their spread, or (nil, +Inf) when the groups cannot be packed
// into teams.
func draftGrouped(active []PlayerScore, edges []edge, teamCount, teamSize int) ([][]PlayerScore, float64) {
	// Connected components → units.
	parent := make([]int, len(active))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	for _, e := range edges {
		if ra, rb := find(e.a), find(e.b); ra != rb {
			if ra > rb {
				ra, rb = rb, ra
			}
			parent[rb] = ra
		}
	}
	byRoot := map[int]*unit{}
	var units []*unit
	for i, p := range active { // active is sorted, so units keep that order
		r := find(i)
		u, ok := byRoot[r]
		if !ok {
			u = &unit{}
			byRoot[r] = u
			units = append(units, u)
		}
		u.members = append(u.members, i)
		u.sum += p.SkillScore
	}

	// Place big groups first (they are the hardest to fit), then singles
	// from strongest to weakest.
	sort.SliceStable(units, func(i, j int) bool {
		if len(units[i].members) != len(units[j].members) {
			return len(units[i].members) > len(units[j].members)
		}
		return units[i].sum > units[j].sum
	})
	teams := make([][]*unit, teamCount)
	sizes := make([]int, teamCount)
	sums := make([]float64, teamCount)
	for _, u := range units {
		n := len(u.members)
		target := -1
		for t := 0; t < teamCount; t++ {
			if sizes[t]+n > teamSize {
				continue
			}
			if target < 0 {
				target = t
				continue
			}
			if n > 1 {
				// Groups: tightest fit first, to keep packing feasible.
				if sizes[t] > sizes[target] || (sizes[t] == sizes[target] && sums[t] < sums[target]) {
					target = t
				}
			} else if sums[t] < sums[target] {
				target = t
			}
		}
		if target < 0 {
			return nil, math.Inf(1)
		}
		teams[target] = append(teams[target], u)
		sizes[target] += n
		sums[target] += u.sum
	}

	balanceUnits(teams, sums)

	out := make([][]PlayerScore, teamCount)
	lo, hi := math.Inf(1), math.Inf(-1)
	for t, team := range teams {
		out[t] = make([]PlayerScore, 0, teamSize)
		for _, u := range team {
			for _, m := range u.members {
				out[t] = append(out[t], active[m])
			}
		}
		sort.SliceStable(out[t], func(i, j int) bool { return out[t][i].SkillScore > out[t][j].SkillScore })
		lo = math.Min(lo, sums[t])
		hi = math.Max(hi, sums[t])
	}
	return out, (hi - lo) / float64(teamSize)
}

// balanceUnits repeatedly swaps one unit against an equally sized set of
// units from another team whenever that brings the two team totals closer,
// until no such swap exists. Team sizes never change.
func balanceUnits(teams [][]*unit, sums []float64) {
	const eps = 1e-9
	for pass := 0; pass < 1000; pass++ {
		improved := false
		for x := range teams {
			for y := range teams {
				if x == y {
					continue
				}
				// Best swap of one unit from x against a subset of y.
				bestGain, bestUnit, bestMask := eps, -1, 0
				subsets := len(teams[y])
				if subsets > 12 {
					subsets = 0 // too many to enumerate; fall back to 1:1 swaps
				}
				for ui, u := range teams[x] {
					try := func(mask, size int, sum float64) {
						if size != len(u.members) {
							return
						}
						d := sum - u.sum
						// Change in sums[x]² + sums[y]² is 2d(sx − sy + d).
						if gain := -2 * d * (sums[x] - sums[y] + d); gain > bestGain {
							bestGain, bestUnit, bestMask = gain, ui, mask
						}
					}
					if subsets == 0 {
						for vi, v := range teams[y] {
							try(1<<vi, len(v.members), v.sum)
						}
						continue
					}
					for mask := 1; mask < 1<<subsets; mask++ {
						size, sum := 0, 0.0
						for vi, v := range teams[y] {
							if mask&(1<<vi) != 0 {
								size += len(v.members)
								sum += v.sum
							}
						}
						try(mask, size, sum)
					}
				}
				if bestUnit < 0 {
					continue
				}
				u := teams[x][bestUnit]
				newX := append([]*unit(nil), teams[x][:bestUnit]...)
				newX = append(newX, teams[x][bestUnit+1:]...)
				newY := []*unit{u}
				moved := 0.0
				for vi, v := range teams[y] {
					if bestMask&(1<<vi) != 0 {
						newX = append(newX, v)
						moved += v.sum
					} else {
						newY = append(newY, v)
					}
				}
				teams[x], teams[y] = newX, newY
				sums[x] += moved - u.sum
				sums[y] -= moved - u.sum
				improved = true
			}
		}
		if !improved {
			return
		}
	}
}

// friendGroups is a union-find over player IDs tracking group sizes.
type friendGroups struct {
	parent map[string]string
	size   map[string]int
}

func newFriendGroups() *friendGroups {
	return &friendGroups{parent: map[string]string{}, size: map[string]int{}}
}

func (g *friendGroups) find(id string) string {
	if _, ok := g.parent[id]; !ok {
		g.parent[id] = id
		g.size[id] = 1
		return id
	}
	for g.parent[id] != id {
		g.parent[id] = g.parent[g.parent[id]]
		id = g.parent[id]
	}
	return id
}

func (g *friendGroups) union(a, b string) {
	ra, rb := g.find(a), g.find(b)
	if ra == rb {
		return
	}
	g.parent[rb] = ra
	g.size[ra] += g.size[rb]
}

// mergedSize is the size of the group that linking a and b would produce.
func (g *friendGroups) mergedSize(a, b string) int {
	ra, rb := g.find(a), g.find(b)
	if ra == rb {
		return g.size[ra]
	}
	return g.size[ra] + g.size[rb]
}
