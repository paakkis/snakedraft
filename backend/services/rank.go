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
	// mechanicsBase and mechanicsSlope give the aim and positioning that are
	// typical for a rank (both 0–1): 0.40 + 0.45 · rank. Mechanics spread
	// over a much narrower range than ranks do. Players with a low rank
	// still aim at around 0.55, so comparing the two numbers directly makes
	// every low-ranked player look underrated.
	mechanicsBase  = 0.40
	mechanicsSlope = 0.45

	// mechanicsTolerance is how far the measured mechanics may sit above the
	// typical ones for the rank before the rank is questioned.
	mechanicsTolerance = 0.05

	// mechanicsPull is the share of the gap beyond the tolerance that the
	// rank is raised by. Mechanics are not the whole game, so the rank only
	// moves part of the way.
	mechanicsPull = 0.5

	// underratedGap is the gap from which a player is flagged as underrated.
	underratedGap = 0.10
)

const (
	// performanceEloRange is how far above or below the lobby's ELO a single
	// match can place a player. It mirrors the performance rating used in
	// chess, where a clean sweep is worth 400 points over the opposition.
	performanceEloRange = 400.0

	// performanceDecay weighs each match 15% less than the one after it.
	performanceDecay = 0.85

	// performanceSettleMatches is how many matches it takes for the implied
	// ELO to earn half of its full pull on the rank.
	performanceSettleMatches = 5.0

	// performanceMinPull is the share of the pull that remains even for a
	// fully settled rank: ELO follows team results, so it can lag behind a
	// player who keeps performing above their lobbies.
	performanceMinPull = 0.25

	// performanceHalfLife is how long ago the matches can have been played
	// before their pull on the rank halves. They show the form the player
	// was in then, which says less and less about the form they are in now.
	performanceHalfLife = 180 * 24 * time.Hour

	// outperformingElo and outperformingMatches are the gap between implied
	// and listed ELO, and the number of matches it must hold over, from
	// which a player is flagged as underrated. The matches must also be
	// newer than performanceHalfLife.
	outperformingElo     = 150.0
	outperformingMatches = 5
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
	// TypicalMechanics is what Mechanics usually is at this rank.
	TypicalMechanics float64
	// Underrated is set when the mechanics sit well above the typical ones.
	Underrated bool
	// Outperforming is set when a player keeps performing like a clearly
	// higher Faceit ELO than their own, given the lobbies they play in.
	Outperforming bool
	// Confidence (0–1) scales the weight the rank gets in the skill score.
	Confidence float64
	// Matches is how many matches are behind the rank on its own platform,
	// or UnknownMatches.
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
		faceit = RankAssessment{Source: RankSourceFaceit, Listed: normalizeFaceitElo(float64(f.Elo)), Matches: UnknownMatches}
		if f.LifetimeMatches > 0 {
			faceit.Matches = f.LifetimeMatches
		}
		if f.Recent != nil {
			faceit.LastPlayed = f.Recent.LastMatchAt
		}
	case p != nil && faceitEloOf(p) > 0:
		faceit = RankAssessment{Source: RankSourceFaceit, Listed: normalizeFaceitElo(faceitEloOf(p)), Matches: UnknownMatches}
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
		premier = RankAssessment{Source: RankSourcePremier, Listed: normalize(float64(p.Ranks.Premier), 0, 35000)}
		premier.Matches, _ = leetifyMatches(p, isPremierMatch)
		if at, ok := PremierRatedAt(p); ok {
			premier.LastPlayed = &at
		}
	}

	faceit.Confidence = rankConfidence(faceit.Matches, faceit.LastPlayed)
	premier.Confidence = rankConfidence(premier.Matches, premier.LastPlayed)
	faceit.Value, premier.Value = faceit.Listed, premier.Listed

	// Performance against the lobby moves the Faceit ELO towards the ELO the
	// player performed like. The fewer matches the rank was earned over and
	// the more matches the performance covers, the further it moves. This
	// works both ways. The matches are the player's latest ones, so they are
	// exactly as old as the rank: a rank going stale is no reason to trust
	// them more, and old matches pull less.
	if perf := performanceOf(f); perf != nil {
		m := float64(perf.Matches)
		settled := rankConfidence(faceit.Matches, nil)
		fresh := formFreshness(faceit.LastPlayed)
		pull := (1 - (1-performanceMinPull)*settled) * m / (m + performanceSettleMatches) * fresh
		elo := float64(f.Elo)
		faceit.Value = normalizeFaceitElo(elo + pull*(perf.ImpliedElo-elo))
		faceit.Outperforming = fresh >= 0.5 &&
			perf.Matches >= outperformingMatches && perf.ImpliedElo-elo >= outperformingElo
	}

	// A player is at least as good as their best platform shows; a low rank
	// on the other one usually just means they barely play there.
	best, other := faceit, premier
	if premier.Value > faceit.Value || (premier.Value == faceit.Value && premier.Confidence > faceit.Confidence) {
		best, other = premier, faceit
	}
	if best.Value <= 0 {
		return RankAssessment{Matches: UnknownMatches}
	}
	// The other rank backs the best one up, the more so the closer it is: a
	// thin rank that a settled one nearly agrees with is not in much doubt.
	if backing := other.Confidence * other.Value / best.Value; backing > best.Confidence {
		best.Confidence = backing
	}
	// Worth knowing about the player whichever rank they are scored on.
	best.Outperforming = faceit.Outperforming

	// Aim and positioning are measured per match and do not depend on who
	// the player is matched against, so they show skill the rank has not
	// caught up with yet. They are compared with what is typical for the
	// rank. Only ever raises the rank: mechanics below the typical ones are
	// normal for players who win on game sense and teamwork.
	if p != nil {
		best.Mechanics = mechanics(p)
	}
	best.TypicalMechanics = typicalMechanics(best.Value)
	if gap := best.Mechanics - best.TypicalMechanics; best.Mechanics > 0 && gap > mechanicsTolerance {
		best.Value = math.Min(1, best.Value+mechanicsPull*(gap-mechanicsTolerance))
		best.Underrated = gap >= underratedGap
	}
	return best
}

// formFreshness is how much a player's latest Faceit matches still say about
// their current form (0–1): it halves every performanceHalfLife since the
// last one. 1 when it is not known when that was.
func formFreshness(lastPlayed *time.Time) float64 {
	if lastPlayed == nil {
		return 1
	}
	age := timeNow().Sub(*lastPlayed)
	if age <= 0 {
		return 1
	}
	return math.Pow(0.5, float64(age)/float64(performanceHalfLife))
}

// typicalMechanics is the aim and positioning usually seen at a rank (0–1).
func typicalMechanics(rank float64) float64 {
	return mechanicsBase + mechanicsSlope*rank
}

// performanceOf returns a player's usable Faceit performance, or nil.
func performanceOf(f *FaceitPlayer) *FaceitPerformance {
	if f == nil || f.Elo <= 0 || f.Performance == nil || f.Performance.Matches <= 0 {
		return nil
	}
	return f.Performance
}

// faceitMatchRating rates a single Faceit match from 0 to 1 on K/D (0.6–1.4)
// and ADR (50–100), equally weighted. 0.5 is an average player in the lobby.
// Faceit has no ADR for older matches (it is then 0): those are rated on K/D
// alone.
func faceitMatchRating(kd, adr float64) float64 {
	if adr <= 0 {
		return normalize(kd, 0.6, 1.4)
	}
	return (normalize(kd, 0.6, 1.4) + normalize(adr, 50, 100)) / 2
}

// summarisePerformance works out what ELO a player performed like over the
// given matches (newest first). Each match with a known lobby implies
//
//	lobby ELO + 400 · (2 · rating − 1)
//
// so an average game means the player belongs in that lobby, and a dominant
// one that they belong 400 ELO above it. The matches are averaged with each
// one weighing 15% less than the one after it. Returns nil when no lobby is
// known.
func summarisePerformance(matches []faceitMatch) *FaceitPerformance {
	var implied, lobby, weightSum float64
	n := 0
	for _, m := range matches {
		if m.lobbyElo <= 0 {
			continue
		}
		w := math.Pow(performanceDecay, float64(n))
		implied += w * (m.lobbyElo + performanceEloRange*(2*faceitMatchRating(m.kd, m.adr)-1))
		lobby += m.lobbyElo
		weightSum += w
		n++
	}
	if n == 0 {
		return nil
	}
	return &FaceitPerformance{Matches: n, ImpliedElo: implied / weightSum, LobbyElo: lobby / float64(n)}
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
