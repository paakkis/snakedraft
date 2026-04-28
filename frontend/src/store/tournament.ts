import { create } from 'zustand'
import type { Tournament, Team } from '../types'
import * as api from '../api/client'

interface TournamentStore {
  tournament: Tournament | null
  loading: boolean
  addingPlayer: boolean
  error: string | null
  benchPlayerIds: string[]

  createTournament: (name: string, teamSize: number) => Promise<void>
  loadTournament: (id: string) => Promise<void>
  addPlayer: (steamUrl: string) => Promise<void>
  removePlayer: (playerId: string) => Promise<void>
  balanceTeams: () => Promise<void>
  movePlayer: (playerId: string, fromTeamId: string, toTeamId: string) => Promise<void>
  clearError: () => void
  reset: () => void
}

export const useTournamentStore = create<TournamentStore>((set, get) => ({
  tournament: null,
  loading: false,
  addingPlayer: false,
  error: null,
  benchPlayerIds: [],

  clearError: () => set({ error: null }),

  reset: () => set({ tournament: null, loading: false, addingPlayer: false, error: null, benchPlayerIds: [] }),

  createTournament: async (name, teamSize) => {
    set({ loading: true, error: null })
    try {
      const tournament = await api.createTournament(name, teamSize)
      set({ tournament: { ...tournament, players: [], teams: [] } })
    } catch (e) {
      set({ error: (e as Error).message })
    } finally {
      set({ loading: false })
    }
  },

  loadTournament: async (id) => {
    set({ loading: true, error: null })
    try {
      const tournament = await api.getTournament(id)
      set({ tournament })
    } catch (e) {
      set({ error: (e as Error).message })
    } finally {
      set({ loading: false })
    }
  },

  addPlayer: async (steamUrl) => {
    set({ addingPlayer: true, error: null })
    try {
      const { tournament } = get()
      const player = await api.addPlayer(tournament!.id, steamUrl)
      set(s => ({
        tournament: {
          ...s.tournament!,
          players: [...s.tournament!.players, player].sort(
            (a, b) => (b.skill_score ?? -1) - (a.skill_score ?? -1),
          ),
        },
      }))
    } catch (e) {
      set({ error: (e as Error).message })
    } finally {
      set({ addingPlayer: false })
    }
  },

  removePlayer: async (playerId) => {
    const { tournament } = get()
    if (!tournament) return
    try {
      await api.removePlayer(tournament.id, playerId)
      set(s => ({
        tournament: {
          ...s.tournament!,
          players: s.tournament!.players.filter(p => p.id !== playerId),
        },
      }))
    } catch (e) {
      set({ error: (e as Error).message })
    }
  },

  balanceTeams: async () => {
    set({ loading: true, error: null })
    try {
      const { tournament } = get()
      const result = await api.balanceTeams(tournament!.id, tournament!.team_size)
      set(s => ({
        tournament: {
          ...s.tournament!,
          teams: result.teams,
          variance: result.variance,
          status: 'balanced',
        },
        benchPlayerIds: result.bench_players ?? [],
      }))
    } catch (e) {
      set({ error: (e as Error).message })
    } finally {
      set({ loading: false })
    }
  },

  movePlayer: async (playerId, fromTeamId, toTeamId) => {
    const { tournament } = get()
    if (!tournament) return

    // Optimistic update
    set(s => {
      const movingPlayer = s.tournament!.teams
        .flatMap(t => t.players)
        .find(p => p.id === playerId)
      if (!movingPlayer) return s

      const teams: Team[] = s.tournament!.teams.map(team => {
        if (team.id === fromTeamId) {
          const players = team.players.filter(p => p.id !== playerId)
          const avg = players.reduce((sum, p) => sum + (p.skill_score ?? 0), 0) / (players.length || 1)
          return { ...team, players, avg_skill_score: avg }
        }
        if (team.id === toTeamId) {
          const players = [...team.players, movingPlayer]
          const avg = players.reduce((sum, p) => sum + (p.skill_score ?? 0), 0) / (players.length || 1)
          return { ...team, players, avg_skill_score: avg }
        }
        return team
      })
      return { tournament: { ...s.tournament!, teams } }
    })

    try {
      await api.movePlayer(tournament.id, playerId, fromTeamId, toTeamId)
    } catch (e) {
      // Revert on failure
      set({ error: (e as Error).message })
      get().loadTournament(tournament.id)
    }
  },
}))
