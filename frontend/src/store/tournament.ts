import { create } from 'zustand'
import type { Tournament, Team, RequestStatus } from '../types'
import * as api from '../api/client'

interface TournamentStore {
  tournament: Tournament | null
  loading: boolean
  addingPlayer: boolean
  error: string | null
  benchPlayerIds: string[]
  requestError: string | null
  // Outcome of each teammate request in the latest balance run, by request id
  requestStatuses: Record<string, RequestStatus>

  addTeammateRequest: (requesterId: string, requestedId: string) => Promise<boolean>
  removeTeammateRequest: (requestId: string) => Promise<void>
  createTournament: (name: string, teamSize: number) => Promise<void>
  loadTournament: (id: string) => Promise<void>
  refreshingIds: string[]
  addPlayer: (steamUrl: string) => Promise<boolean>
  refreshPlayers: (playerIds: string[]) => Promise<void>
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
  requestError: null,
  requestStatuses: {},
  refreshingIds: [],

  clearError: () => set({ error: null }),

  reset: () => set({
    tournament: null, loading: false, addingPlayer: false, error: null,
    benchPlayerIds: [], requestError: null, requestStatuses: {},
  }),

  createTournament: async (name, teamSize) => {
    set({ loading: true, error: null })
    try {
      const tournament = await api.createTournament(name, teamSize)
      set({ tournament: { ...tournament, players: [], teams: [], teammate_requests: [] } })
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
      set({
        tournament: { ...tournament, teammate_requests: tournament.teammate_requests ?? [] },
        benchPlayerIds: [],
        requestError: null,
        requestStatuses: {},
      })
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
      return true
    } catch (e) {
      set({ error: (e as Error).message })
      return false
    } finally {
      set({ addingPlayer: false })
    }
  },

  // Re-fetches stats one player at a time (to stay inside API rate limits),
  // then reloads the tournament so scores, details and team averages agree.
  refreshPlayers: async (playerIds) => {
    const { tournament } = get()
    if (!tournament) return
    set({ error: null, refreshingIds: playerIds })
    const failed: string[] = []
    for (const id of playerIds) {
      try {
        await api.refreshPlayer(tournament.id, id)
      } catch {
        failed.push(tournament.players.find(p => p.id === id)?.steam_name ?? id)
      }
      set(s => ({ refreshingIds: s.refreshingIds.filter(x => x !== id) }))
    }
    try {
      const fresh = await api.getTournament(tournament.id)
      set({ tournament: { ...fresh, teammate_requests: fresh.teammate_requests ?? [] } })
    } catch (e) {
      set({ error: (e as Error).message })
    }
    if (failed.length > 0) {
      set({ error: `Could not refresh: ${failed.join(', ')}` })
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
          // The server deletes a removed player's requests along with them
          teammate_requests: s.tournament!.teammate_requests.filter(
            r => r.requester_id !== playerId && r.requested_id !== playerId,
          ),
        },
      }))
    } catch (e) {
      set({ error: (e as Error).message })
    }
  },

  addTeammateRequest: async (requesterId, requestedId) => {
    const { tournament } = get()
    if (!tournament) return false
    set({ requestError: null })
    try {
      const request = await api.addTeammateRequest(tournament.id, requesterId, requestedId)
      set(s => ({
        tournament: {
          ...s.tournament!,
          teammate_requests: [...s.tournament!.teammate_requests, request],
        },
      }))
      return true
    } catch (e) {
      set({ requestError: (e as Error).message })
      return false
    }
  },

  removeTeammateRequest: async (requestId) => {
    const { tournament } = get()
    if (!tournament) return
    set({ requestError: null })
    try {
      await api.removeTeammateRequest(tournament.id, requestId)
      set(s => ({
        tournament: {
          ...s.tournament!,
          teammate_requests: s.tournament!.teammate_requests.filter(r => r.id !== requestId),
        },
      }))
    } catch (e) {
      set({ requestError: (e as Error).message })
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
        requestStatuses: Object.fromEntries(
          (result.requests ?? []).flatMap(r => (r.status ? [[r.id, r.status]] : [])),
        ),
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
