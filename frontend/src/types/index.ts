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
  fetched_at?: string
}

export interface Team {
  id: string
  tournament_id: string
  name: string
  avg_skill_score: number
  players: Player[]
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
  status?: RequestStatus    // only present in a balance result
}

export interface Tournament {
  id: string
  name: string
  team_size: number
  status: 'signup' | 'balanced' | 'active' | 'finished'
  created_at: string
  players: Player[]
  teams: Team[]
  teammate_requests: TeammateRequest[]
  variance?: number
}

export interface BalanceResult {
  variance: number
  teams: Team[]
  bench_players: string[]
  spread: number
  baseline_spread: number
  requests: TeammateRequest[]
}
