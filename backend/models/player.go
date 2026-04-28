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

type Player struct {
	ID            string      `json:"id"`
	TournamentID  string      `json:"tournament_id"`
	Steam64ID     string      `json:"steam64_id"`
	VanityURL     string      `json:"vanity_url"`
	SteamName     string      `json:"steam_name"`
	AvatarURL     string      `json:"avatar_url"`
	FaceitElo     *int        `json:"faceit_elo"`
	FaceitLevel   *int        `json:"faceit_level"`
	PremierRating *int        `json:"premier_rating"`
	SkillScore    *float64    `json:"skill_score"`
	HoursPlayed   *float64    `json:"hours_played"`
	IsBanned      bool        `json:"is_banned"`
	BanInfo       []BanInfo   `json:"ban_info"`
	PrivacyMode   string      `json:"privacy_mode"`
	Rating        *Rating     `json:"rating,omitempty"`
	FetchedAt     *time.Time  `json:"fetched_at"`
	CreatedAt     time.Time   `json:"created_at"`

	// internal — not serialized directly, used for DB roundtrip
	BanInfoRaw  json.RawMessage `json:"-"`
	RawStatsRaw json.RawMessage `json:"-"`
}
