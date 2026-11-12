package services

import (
	"math"
	"math/rand"
	"sort"
)

type PlayerScore struct {
	PlayerID   string
	SkillScore float64
}

// SnakeDraft distributes players across K teams in snake draft order.
// Players are sorted descending by SkillScore internally.
//
// Example with 4 teams (P1=best, P16=worst):
//
//	Round 1 (L→R): T1←P1,  T2←P2,  T3←P3,  T4←P4
//	Round 2 (R→L): T4←P5,  T3←P6,  T2←P7,  T1←P8
//	Round 3 (L→R): T1←P9,  T2←P10, ...
func SnakeDraft(players []PlayerScore, teamCount int) [][]PlayerScore {
	if teamCount <= 0 {
		return [][]PlayerScore{}
	}
	sorted := make([]PlayerScore, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].SkillScore > sorted[j].SkillScore
	})

	teams := make([][]PlayerScore, teamCount)
	for i := range teams {
		teams[i] = []PlayerScore{}
	}

	for i, p := range sorted {
		round := i / teamCount
		pos := i % teamCount
		var teamIdx int
		if round%2 == 0 {
			teamIdx = pos
		} else {
			teamIdx = teamCount - 1 - pos
		}
		teams[teamIdx] = append(teams[teamIdx], p)
	}
	return teams
}

// OptimizeTeams runs random pairwise player swaps between teams,
// accepting any swap that reduces the variance of team average scores.
// 10,000 iterations runs in < 10ms for 40 players across 8 teams.
func OptimizeTeams(teams [][]PlayerScore, iterations int) [][]PlayerScore {
	best := deepCopy(teams)
	if len(best) < 2 {
		return best
	}
	bestVar := TeamVariance(best)

	for i := 0; i < iterations; i++ {
		t1 := rand.Intn(len(best))
		t2 := rand.Intn(len(best))
		if t1 == t2 || len(best[t1]) == 0 || len(best[t2]) == 0 {
			continue
		}
		p1 := rand.Intn(len(best[t1]))
		p2 := rand.Intn(len(best[t2]))

		candidate := deepCopy(best)
		candidate[t1][p1], candidate[t2][p2] = candidate[t2][p2], candidate[t1][p1]

		if v := TeamVariance(candidate); v < bestVar {
			best = candidate
			bestVar = v
		}
	}
	return best
}

// TeamVariance returns the variance of average skill scores across teams.
// Lower is better — 0 means perfectly balanced teams.
func TeamVariance(teams [][]PlayerScore) float64 {
	if len(teams) == 0 {
		return 0
	}
	avgs := make([]float64, len(teams))
	for i, team := range teams {
		sum := 0.0
		for _, p := range team {
			sum += p.SkillScore
		}
		if len(team) > 0 {
			avgs[i] = sum / float64(len(team))
		}
	}
	mean := 0.0
	for _, a := range avgs {
		mean += a
	}
	mean /= float64(len(avgs))

	variance := 0.0
	for _, a := range avgs {
		variance += math.Pow(a-mean, 2)
	}
	return variance / float64(len(avgs))
}

func deepCopy(teams [][]PlayerScore) [][]PlayerScore {
	c := make([][]PlayerScore, len(teams))
	for i, t := range teams {
		c[i] = make([]PlayerScore, len(t))
		copy(c[i], t)
	}
	return c
}
