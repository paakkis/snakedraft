package services

import (
	"math"
	"time"
)

// timeNow is a variable so tests can pin the clock.
var timeNow = time.Now

const (
	// rankSettleMatches is how many matches it takes for a rank to earn half
	// of its full weight. A rating needs games to converge: a Faceit account
	// starts at 1000 ELO whatever the player's skill, and moves about 25 per
	// match.
	rankSettleMatches = 20.0

	// rankHalfLife is how long a rank can go unplayed before its confidence
	// halves.
	rankHalfLife = 365 * 24 * time.Hour
)

const (
	// mechanicsTolerance is how far the measured mechanics may sit above the
	// rank before the rank is questioned. Among settled players the two stay
	// within a few points of each other, with mechanics usually the lower.
	mechanicsTolerance = 0.05

	// mechanicsPull is the share of the gap beyond the tolerance that the
	// rank is raised by. Mechanics are not the whole game, so the rank only
	// moves part of the way.
	mechanicsPull = 0.5

	// underratedGap is the gap from which a player is flagged as underrated.
	underratedGap = 0.10
)

// Rank sources.
const (
	RankSourceFaceit  = "faceit"
	RankSourcePremier = "premier"
)

// UnknownMatches marks a rank whose match count could not be determined.
const UnknownMatches = -1

// RankAssessment is the rank a player is scored on, and how far it can be
// trusted.
type RankAssessment struct {
	// Source is RankSourceFaceit, RankSourcePremier, or "" without a rank.
	Source string
	// Value is the rank the player is scored on, normalised to 0–1: the
	// listed rank plus any correction. 0 means the player has no rank.
	Value float64
	// Listed is the rank as the platform shows it, normalised to 0–1.
	Listed float64
	// Mechanics is the player's measured aim and positioning (0–1), or 0
	// when Leetify has no ratings for them.
	Mechanics float64
	// Underrated is set when the mechanics sit well above the listed rank.
	Underrated bool
	// Confidence (0–1) scales the weight the rank gets in the skill score.
	Confidence float64
	// Matches is how many matches are behind the rank, or UnknownMatches.
	Matches int
	// LastPlayed is when the rank was last played for, if known.
	LastPlayed *time.Time
}

// rankConfidence is how much a rank earned over the given number of matches
// and last played at the given time can be trusted:
//
//	confidence = n / (n + 20) · 0.5^(years since last played)
//
// The first factor is the same idea as the rating deviation in Glicko: few
// games mean an uncertain rating. Either part is left out (counted as 1)
// when it is unknown, so missing data never lowers the confidence.
func rankConfidence(matches int, lastPlayed *time.Time) float64 {
	c := 1.0
	if matches >= 0 {
		c = float64(matches) / (float64(matches) + rankSettleMatches)
	}
	if lastPlayed != nil {
		if age := timeNow().Sub(*lastPlayed); age > 0 {
			c *= math.Pow(0.5, float64(age)/float64(rankHalfLife))
		}
	}
	return c
}

// AssessRank picks the rank a player is scored on (the better of Faceit and
// Premier) and works out how much it can be trusted. Both arguments may be
// nil. Faceit's own data wins over the copy Leetify holds, which can be stale.
func AssessRank(p *LeetifyProfile, f *FaceitPlayer) RankAssessment {
	if p != nil && p.PrivacyMode != "public" {
		p = nil
	}

	var faceit, premier RankAssessment

	switch {
	case f != nil && f.Elo > 0:
		faceit = RankAssessment{Source: RankSourceFaceit, Value: normalizeFaceitElo(float64(f.Elo)), Matches: UnknownMatches}
		if f.LifetimeMatches > 0 {
			faceit.Matches = f.LifetimeMatches
		}
		if f.Recent != nil {
			faceit.LastPlayed = f.Recent.LastMatchAt
		}
	case p != nil && faceitEloOf(p) > 0:
		faceit = RankAssessment{Source: RankSourceFaceit, Value: normalizeFaceitElo(faceitEloOf(p)), Matches: UnknownMatches}
	}
	if faceit.Source != "" && p != nil {
		n, last := leetifyMatches(p, isFaceitMatch)
		if faceit.Matches == UnknownMatches {
			faceit.Matches = n
		}
		if faceit.LastPlayed == nil {
			faceit.LastPlayed = last
		}
	}

	if p != nil && p.Ranks.Premier > 0 {
		premier = RankAssessment{Source: RankSourcePremier, Value: normalize(float64(p.Ranks.Premier), 0, 35000)}
		premier.Matches, _ = leetifyMatches(p, isPremierMatch)
		if at, ok := PremierRatedAt(p); ok {
			premier.LastPlayed = &at
		}
	}

	faceit.Confidence = rankConfidence(faceit.Matches, faceit.LastPlayed)
	premier.Confidence = rankConfidence(premier.Matches, premier.LastPlayed)

	// A player is at least as good as their best platform shows; a low rank
	// on the other one usually just means they barely play there.
	best := faceit
	if premier.Value > best.Value || (premier.Value == best.Value && premier.Confidence > best.Confidence) {
		best = premier
	}
	if best.Value <= 0 {
		return RankAssessment{Matches: UnknownMatches}
	}
	best.Listed = best.Value

	// Aim and positioning are measured per match and do not depend on who
	// the player is matched against, so they show skill the rank has not
	// caught up with yet. Only ever raises the rank: a rank above the
	// mechanics is normal for players who win on game sense and teamwork.
	if p != nil {
		best.Mechanics = mechanics(p)
	}
	if gap := best.Mechanics - best.Listed; best.Mechanics > 0 && gap > mechanicsTolerance {
		best.Value = math.Min(1, best.Listed+mechanicsPull*(gap-mechanicsTolerance))
		best.Underrated = gap >= underratedGap
	}
	return best
}

// mechanics is the average of a player's Leetify aim and positioning ratings
// (0–1), weighted as in the skill score. 0 when neither is known.
func mechanics(p *LeetifyProfile) float64 {
	var sum, weight float64
	if p.Rating.Aim > 0 {
		sum += p.Rating.Aim / 100 * weightAim
		weight += weightAim
	}
	if p.Rating.Positioning > 0 {
		sum += p.Rating.Positioning / 100 * weightPositioning
		weight += weightPositioning
	}
	if weight == 0 {
		return 0
	}
	return sum / weight
}

func isFaceitMatch(m RecentMatch) bool { return m.DataSource == "faceit" }

func isPremierMatch(m RecentMatch) bool {
	return m.RankType != nil && *m.RankType == leetifyRankTypePremier
}

// leetifyMatches estimates how many matches of one kind a player has played,
// and when the latest one was. Leetify only lists the most recent matches, so
// the share of that kind among them is applied to the player's total. Returns
// UnknownMatches when none of the listed matches are of that kind: the rank
// is then older than everything listed and nothing can be said about it.
func leetifyMatches(p *LeetifyProfile, match func(RecentMatch) bool) (n int, last *time.Time) {
	count := 0
	for _, m := range p.RecentMatches {
		if !match(m) {
			continue
		}
		count++
		if at, err := time.Parse(time.RFC3339, m.FinishedAt); err == nil && (last == nil || at.After(*last)) {
			last = &at
		}
	}
	if count == 0 {
		return UnknownMatches, nil
	}
	listed := len(p.RecentMatches)
	total := max(p.TotalMatches, listed)
	return int(math.Round(float64(total) * float64(count) / float64(listed))), last
}
