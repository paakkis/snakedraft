package services

import "math"

// ComputeSkillScore returns a 0–100 normalized skill score for a player.
// Returns -1 if the profile is private (no stats available).
//
// Weights:
//
//	30% Faceit ELO (or level if ELO missing)
//	25% Premier rating
//	15% Leetify aim score
//	10% Leetify positioning score
//	 8% Opening duel win % (avg CT + T side)
//	 8% Recent form (exponentially decayed last 10 matches)
//	 4% Leetify utility score
func ComputeSkillScore(p *LeetifyProfile) float64 {
	if p.PrivacyMode != "public" {
		return -1
	}

	var faceitNorm float64
	if p.Ranks.FaceitElo != nil {
		faceitNorm = normalize(float64(*p.Ranks.FaceitElo), 0, 4000)
	} else if p.Ranks.Faceit > 0 {
		// Fall back to level (1–10) if raw ELO not available
		faceitNorm = normalize(float64(p.Ranks.Faceit), 0, 10)
	}

	premierNorm := normalize(float64(p.Ranks.Premier), 0, 35000)
	aimNorm     := p.Rating.Aim / 100
	posNorm     := p.Rating.Positioning / 100
	utilNorm    := p.Rating.Utility / 100
	openingNorm := normalize(
		(p.Stats.CtOpeningDuelSuccessPercentage+p.Stats.TOpeningDuelSuccessPercentage)/2,
		0, 100,
	)
	recentNorm := normalize(computeRecentForm(p.RecentMatches), -0.2, 0.3)

	score := faceitNorm*0.30 +
		premierNorm*0.25 +
		aimNorm*0.15 +
		posNorm*0.10 +
		openingNorm*0.08 +
		recentNorm*0.08 +
		utilNorm*0.04

	return math.Round(score*10000) / 100 // 0–100 with 2 decimal places
}

// ComputeSteamScore returns a 0–100 score derived purely from Steam lifetime
// stats (GetUserStatsForGame). Used as a fallback when Leetify data is missing.
//
// Weights:
//
//	35% KD ratio        (0–2.5 range)
//	25% Win rate        (0–100%)
//	20% Headshot %      (0–60%)
//	12% Accuracy        (0–30%)
//	 8% MVP rate        (0–2 MVPs/match)
func ComputeSteamScore(stats *CS2Stats) float64 {
	kdNorm  := normalize(stats.KDRatio, 0, 2.5)
	wrNorm  := normalize(stats.WinRate, 0, 100)
	hsNorm  := normalize(stats.HSPercent, 0, 60)
	accNorm := normalize(stats.Accuracy, 0, 30)
	mvpNorm := normalize(stats.MVPRate, 0, 2)

	score := kdNorm*0.35 +
		wrNorm*0.25 +
		hsNorm*0.20 +
		accNorm*0.12 +
		mvpNorm*0.08

	return math.Round(score*10000) / 100
}

func normalize(value, min, max float64) float64 {
	if max == min {
		return 0
	}
	n := (value - min) / (max - min)
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

// computeRecentForm returns an exponentially weighted average of the
// player's last 10 match leetify_ratings. Faceit matches are weighted
// 20% higher since competition level is higher.
func computeRecentForm(matches []RecentMatch) float64 {
	if len(matches) == 0 {
		return 0
	}
	limit := 10
	if len(matches) < limit {
		limit = len(matches)
	}
	decay := 0.85
	var weightSum, valueSum float64
	for i, m := range matches[:limit] {
		multiplier := 1.0
		if m.DataSource == "faceit" {
			multiplier = 1.2
		}
		w := math.Pow(decay, float64(i)) * multiplier
		valueSum += m.LeetifyRating * w
		weightSum += w
	}
	return valueSum / weightSum
}
