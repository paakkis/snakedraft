package models

import (
	"encoding/json"
	"time"
)

type BanInfo struct {
	Platform string `json:"platform"`
}

type Rating struct {
	Aim         float64 `json:"aim"`
	Positioning float64 `json:"positioning"`
	Utility     float64 `json:"utility"`
	Clutch      float64 `json:"clutch"`
}

// LeetifyStats is the extra Leetify metadata shown on a player card.
type LeetifyStats struct {
	Rating         float64 `json:"rating"`  // overall Leetify rating
	Winrate        float64 `json:"winrate"` // 0–100
	TotalMatches   int     `json:"total_matches"`
	ReactionTimeMs float64 `json:"reaction_time_ms"`
	HeadAccuracy   float64 `json:"head_accuracy"` // 0–100
	OpeningDuels   float64 `json:"opening_duels"` // avg CT+T opening duel win %, 0–100
}

// SteamStats are lifetime CS2 stats from the Steam Web API.
type SteamStats struct {
	KDRatio         float64 `json:"kd_ratio"`
	HSPercent       float64 `json:"hs_percent"`
	Accuracy        float64 `json:"accuracy"`
	WinRate         float64 `json:"win_rate"`
	HoursLast2Weeks float64 `json:"hours_last_2_weeks"`
}

// FaceitStats is a player's Faceit identity and recent form.
type FaceitStats struct {
	Nickname      string     `json:"nickname"`
	RecentMatches int        `json:"recent_matches"`
	RecentKD      float64    `json:"recent_kd"`
	RecentADR     float64    `json:"recent_adr"`
	RecentHS      float64    `json:"recent_hs"`       // 0–100
	RecentWinRate float64    `json:"recent_win_rate"` // 0–100
	LastMatchAt   *time.Time `json:"last_match_at,omitempty"`
}

// RankCheck says how far a player's rank can be trusted, and whether there
// are signs that it is too low. Values on the 0–100 scale match the skill
// score.
type RankCheck struct {
	Source string `json:"source"` // "faceit" or "premier"
	// Confidence (0–1) is the share of its normal weight the rank gets.
	Confidence float64 `json:"confidence"`
	// Matches behind the rank; omitted when unknown.
	Matches *int `json:"matches,omitempty"`
	// Listed is the rank as the platform shows it, Adjusted the one the
	// player is scored on.
	Listed   float64 `json:"listed"`
	Adjusted float64 `json:"adjusted"`
	// Underrated: aim and positioning (Mechanics) sit well above what is
	// typical for the rank (TypicalMechanics).
	Underrated       bool    `json:"underrated"`
	Mechanics        float64 `json:"mechanics,omitempty"`
	TypicalMechanics float64 `json:"typical_mechanics,omitempty"`
	// Outperforming: the player keeps performing like a clearly higher
	// Faceit ELO than their own, given the lobbies they play in.
	Outperforming      bool    `json:"outperforming"`
	ImpliedElo         float64 `json:"implied_elo,omitempty"`
	LobbyElo           float64 `json:"lobby_elo,omitempty"`
	PerformanceMatches int     `json:"performance_matches,omitempty"`
}

type Player struct {
	ID            string `json:"id"`
	TournamentID  string `json:"tournament_id"`
	Steam64ID     string `json:"steam64_id"`
	VanityURL     string `json:"vanity_url"`
	SteamName     string `json:"steam_name"`
	AvatarURL     string `json:"avatar_url"`
	FaceitElo     *int   `json:"faceit_elo"`
	FaceitLevel   *int   `json:"faceit_level"`
	PremierRating *int   `json:"premier_rating"`
	// PremierRatedAt is when the Premier rating was last earned, if known.
	PremierRatedAt *time.Time    `json:"premier_rated_at,omitempty"`
	SkillScore     *float64      `json:"skill_score"`
	HoursPlayed    *float64      `json:"hours_played"`
	IsBanned       bool          `json:"is_banned"`
	BanInfo        []BanInfo     `json:"ban_info"`
	PrivacyMode    string        `json:"privacy_mode"`
	Rating         *Rating       `json:"rating,omitempty"`
	Leetify        *LeetifyStats `json:"leetify,omitempty"`
	Steam          *SteamStats   `json:"steam,omitempty"`
	Faceit         *FaceitStats  `json:"faceit,omitempty"`
	RankCheck      *RankCheck    `json:"rank_check,omitempty"`
	FetchedAt      *time.Time    `json:"fetched_at"`
	CreatedAt      time.Time     `json:"created_at"`

	// internal - not serialized directly, used for DB roundtrip
	BanInfoRaw  json.RawMessage `json:"-"`
	RawStatsRaw json.RawMessage `json:"-"`
}
