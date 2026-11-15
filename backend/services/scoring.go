package services

import "math"

// Weights of the skill score components that are referred to elsewhere.
const (
	weightRank        = 0.55
	weightAim         = 0.15
	weightPositioning = 0.10
)

// ComputeSkillScore returns a 0–100 normalized skill score for a player.
// Returns -1 if the profile is private or holds no usable stats.
//
// Weights:
//
//	55% Rank: the better of Faceit ELO (0–3000; estimated from the level
//	    if the ELO is unknown) and Premier rating (0–35000)
//	15% Leetify aim score
//	10% Leetify positioning score
//	 8% Opening duel win % (avg CT + T side)
//	 8% Recent form (exponentially decayed last 10 matches)
//	 4% Leetify utility score
//
// The rank weight is multiplied by the rank's confidence (see AssessRank), so
// a rank earned over a handful of matches, or one not played for in a long
// time, counts for less and the measured stats count for more. A Faceit ELO
// is moved towards the one the player performed like against their recent
// lobbies, and a rank that sits well below the player's aim and positioning
// is raised part of the way towards them.
//
// A player is never penalised for not being active on a platform: a missing
// source is left out instead of counted as zero, and its weight is spread
// over the components that are present.
//
// faceit is the player's own Faceit data and may be nil.
func ComputeSkillScore(p *LeetifyProfile, faceit *FaceitPlayer) float64 {
	if p.PrivacyMode != "public" {
		return -1
	}

	var weighted, totalWeight float64
	add := func(value, weight float64, present bool) {
		if present {
			weighted += value * weight
			totalWeight += weight
		}
	}

	rank := AssessRank(p, faceit)
	add(rank.Value, weightRank*rank.Confidence, rank.Value > 0)

	// Leetify reports 0 for ratings it has no data for.
	add(p.Rating.Aim/100, weightAim, p.Rating.Aim > 0)
	add(p.Rating.Positioning/100, weightPositioning, p.Rating.Positioning > 0)
	add(p.Rating.Utility/100, 0.04, p.Rating.Utility > 0)

	ct, t := p.Stats.CtOpeningDuelSuccessPercentage, p.Stats.TOpeningDuelSuccessPercentage
	add(normalize((ct+t)/2, 0, 100), 0.08, ct > 0 || t > 0)

	add(normalize(computeRecentForm(p.RecentMatches), -0.2, 0.3), 0.08, len(p.RecentMatches) > 0)

	if totalWeight == 0 {
		return -1
	}
	return math.Round(weighted/totalWeight*10000) / 100 // 0–100 with 2 decimal places
}

// ComputeSteamScore returns a 0–100 score derived purely from Steam lifetime
// stats (GetUserStatsForGame). Used as a fallback when Leetify data is missing.
//
// Weights:
//
//	30% KD ratio        (0–2.5 range)
//	22% Win rate        (0–100%)
//	18% Headshot %      (0–60%)
//	12% Hours played    (0–5000h, sqrt-scaled for diminishing returns)
//	10% Accuracy        (0–30%)
//	 8% MVP rate        (0–2 MVPs/match)
func ComputeSteamScore(stats *CS2Stats) float64 {
	kdNorm  := normalize(stats.KDRatio, 0, 2.5)
	wrNorm  := normalize(stats.WinRate, 0, 100)
	hsNorm  := normalize(stats.HSPercent, 0, 60)
	accNorm := normalize(stats.Accuracy, 0, 30)
	mvpNorm := normalize(stats.MVPRate, 0, 2)
	// Sqrt scaling: 100h ≈ 0.14, 500h ≈ 0.32, 2000h ≈ 0.63, capped at 5000h.
	// Gives credit for moderate hours without over-rewarding the 10,000h
	// player vs the 3,000h player.
	hoursNorm := normalize(math.Sqrt(stats.HoursPlayed), 0, math.Sqrt(5000))

	score := kdNorm*0.30 +
		wrNorm*0.22 +
		hsNorm*0.18 +
		hoursNorm*0.12 +
		accNorm*0.10 +
		mvpNorm*0.08

	return math.Round(score*10000) / 100
}

// faceitMaxElo is the ELO that maps to a perfect rank score. 3000 keeps Faceit
// and Premier roughly comparable (Faceit ~2100 ≈ Premier ~25k ≈ 0.7).
const faceitMaxElo = 3000

// faceitLevelElo is a typical ELO for each Faceit level (1–10), used only
// when the level is known but the ELO is not. Level 10 has no upper bound,
// so it is valued as a player who has just settled into it.
var faceitLevelElo = [...]float64{0, 300, 625, 825, 975, 1125, 1275, 1440, 1640, 1875, 2150}

func normalizeFaceitElo(elo float64) float64 {
	return normalize(elo, 0, faceitMaxElo)
}

// faceitEloOf returns the profile's Faceit ELO, estimating it from the
// level when only that is known. 0 means the player has no Faceit rank.
func faceitEloOf(p *LeetifyProfile) float64 {
	if p.Ranks.FaceitElo != nil && *p.Ranks.FaceitElo > 0 {
		return float64(*p.Ranks.FaceitElo)
	}
	if lvl := p.Ranks.Faceit; lvl > 0 {
		if lvl >= len(faceitLevelElo) {
			lvl = len(faceitLevelElo) - 1
		}
		return faceitLevelElo[lvl]
	}
	return 0
}

// ComputeFaceitScore returns a 0–100 score from Faceit data alone, for
// players who are on Faceit but have no public Leetify profile.
//
//	75% current ELO
//	25% recent form over the last FaceitRecentMatches matches
//	    (K/D 0.6–1.4 and ADR 50–100, equally weighted)
//
// The ELO weight is multiplied by the rank's confidence, and the ELO itself
// is moved towards the one the player performed like against their recent
// lobbies (see AssessRank).
// Without recent matches the ELO stands alone.
func ComputeFaceitScore(f *FaceitPlayer) float64 {
	rank := AssessRank(nil, f)
	score := rank.Value
	if f.Recent != nil && f.Recent.Matches > 0 {
		form := faceitMatchRating(f.Recent.KDRatio, f.Recent.ADR)
		eloWeight := 0.75 * rank.Confidence
		score = (score*eloWeight + form*0.25) / (eloWeight + 0.25)
	}
	return math.Round(score*10000) / 100
}

// How much Steam lifetime stats count next to a better source. They cover a
// player's whole history, so they say little about current form.
const (
	steamWeightWithLeetify = 0.05
	steamWeightWithFaceit  = 0.10
)

// CombineScores merges every available source (each may be nil) into the
// final skill score and the privacy mode to store.
//
//	public Leetify   → Leetify score, using Faceit's own ELO when available,
//	                   blended 95/5 with Steam lifetime stats
//	Faceit only      → Faceit score blended 90/10 with Steam, mode "faceit_only"
//	Steam only       → Steam lifetime stats, mode "steam_only"
//	nothing          → nil score
func CombineScores(profile *LeetifyProfile, steamStats *CS2Stats, faceit *FaceitPlayer) (score *float64, privacyMode string) {
	privacyMode = "unknown"
	leetifyScore := -1.0
	if profile != nil {
		privacyMode = profile.PrivacyMode
		leetifyScore = ComputeSkillScore(profile, faceit)
	}

	blend := func(primary, steamWeight float64) *float64 {
		if steamStats != nil {
			primary = primary*(1-steamWeight) + ComputeSteamScore(steamStats)*steamWeight
		}
		return &primary
	}
	switch {
	case leetifyScore >= 0:
		return blend(leetifyScore, steamWeightWithLeetify), privacyMode
	case faceit != nil:
		return blend(ComputeFaceitScore(faceit), steamWeightWithFaceit), "faceit_only"
	case steamStats != nil:
		steamScore := ComputeSteamScore(steamStats)
		return &steamScore, "steam_only"
	}
	return nil, privacyMode
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
