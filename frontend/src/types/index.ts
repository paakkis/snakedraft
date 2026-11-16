export interface Rating {
  aim: number
  positioning: number
  utility: number
  clutch: number
}

export interface BanInfo {
  platform: string
}

export interface LeetifyStats {
  rating: number            // overall Leetify rating
  winrate: number           // 0–100
  total_matches: number
  reaction_time_ms: number
  head_accuracy: number     // 0–100
  opening_duels: number     // 0–100
  partial?: boolean         // from the Leetify website: last matches only, no positioning or opening duels
}

export interface SteamStats {
  kd_ratio: number
  hs_percent: number
  accuracy: number
  win_rate: number
  hours_last_2_weeks: number
}

export interface FaceitStats {
  nickname: string
  recent_matches: number    // how many latest Faceit matches the recent_* values cover
  recent_kd: number
  recent_adr: number
  recent_hs: number         // 0–100
  recent_win_rate: number   // 0–100
  last_match_at?: string
}

/** How far a player's rank can be trusted, and signs that it is too low. */
export interface RankCheck {
  source: 'faceit' | 'premier'
  confidence: number          // 0–1, share of its normal weight the rank gets
  matches?: number            // matches behind the rank, if known
  listed: number              // rank as the platform shows it, 0–100
  adjusted: number            // rank the player is scored on, 0–100
  underrated: boolean         // aim and positioning sit well above what is typical for the rank
  mechanics?: number          // 0–100
  typical_mechanics?: number  // 0–100, what mechanics usually is at this rank
  outperforming: boolean      // performs like a clearly higher Faceit ELO
  implied_elo?: number
  lobby_elo?: number
  performance_matches?: number
}

export interface Player {
  id: string
  tournament_id: string
  steam64_id: string
  steam_name: string
  avatar_url: string
  vanity_url?: string
  faceit_elo?: number
  faceit_level?: number
  premier_rating?: number
  premier_rated_at?: string  // when the Premier rating was last earned, if known
  skill_score?: number       // null/undefined if private profile or no Leetify data
  hours_played?: number      // CS2 hours from Steam API, null if profile is private
  is_banned: boolean
  ban_info?: BanInfo[]
  privacy_mode: string
  rating?: Rating
  leetify?: LeetifyStats
  steam?: SteamStats
  faceit?: FaceitStats
  rank_check?: RankCheck
  fetched_at?: string
}

export interface Team {
  id: string
  tournament_id: string
  draft_id: string
  name: string
  avg_skill_score: number   // average of the players' scores in the draft
  players: Player[]         // skill_score is the score in the draft
}

/** What a skill score is built from. Mirrors services.ScoreSettings. */
export interface ScoreSettings {
  use_faceit: boolean
  use_premier: boolean
  use_steam: boolean
  // Only the ratios between the weights matter; 0 leaves a component out
  weight_rank: number
  weight_aim: number
  weight_positioning: number
  weight_utility: number
  weight_opening: number
  weight_form: number
  scale_by_confidence: boolean
  adjust_for_performance: boolean
  adjust_for_mechanics: boolean
}

/** Everything that decides how a draft turns out. */
export interface DraftSettings extends ScoreSettings {
  max_spread: number
  honor_requests: boolean
  fallback_to_full_model: boolean
}

export interface DraftScore {
  score: number
  estimated?: boolean   // the settings could not score the player; the full model stood in
}

/** One set of teams, with the settings and scores it was made with. */
export interface Draft {
  id: string
  tournament_id: string
  name: string
  settings: DraftSettings
  teams: Team[]
  scores: Record<string, DraftScore>               // by player id; missing = could not be scored
  request_statuses: Record<string, RequestStatus>  // by request id, as the draft left them
  created_at: string
}

export type RequestStatus =
  | 'honored'              // the two players are on the same team
  | 'dropped_for_balance'  // keeping them together would make teams unfair
  | 'illegal_chain'        // would chain a full team of friends together
  | 'player_unavailable'   // one of them is benched or has no skill score

export interface TeammateRequest {
  id: string
  tournament_id: string
  requester_id: string
  requested_id: string
}

export interface Tournament {
  id: string
  name: string
  team_size: number
  status: 'signup' | 'balanced' | 'active' | 'finished'
  created_at: string
  players: Player[]
  drafts: Draft[]
  teammate_requests: TeammateRequest[]
}
