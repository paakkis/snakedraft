export interface Rating {
  aim: number
  positioning: number
  utility: number
  clutch: number
}

export interface BanInfo {
  platform: string
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
  skill_score?: number       // null/undefined if private profile or no Leetify data
  is_banned: boolean
  ban_info?: BanInfo[]
  privacy_mode: string
  rating?: Rating
  fetched_at?: string
}

export interface Team {
  id: string
  tournament_id: string
  name: string
  avg_skill_score: number
  players: Player[]
}

export interface Tournament {
  id: string
  name: string
  team_size: number
  status: 'signup' | 'balanced' | 'active' | 'finished'
  players: Player[]
  teams: Team[]
  variance?: number
}

export interface BalanceResult {
  variance: number
  teams: Team[]
  bench_players: string[]
}
