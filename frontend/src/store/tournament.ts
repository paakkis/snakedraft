import { create } from 'zustand'
import type { Tournament, Team, Draft, DraftScore, DraftSettings, Player, RankCheck, ScorePreview } from '../types'
import * as api from '../api/client'
import { averageScore } from '../lib/stats'
import { DEFAULT_SETTINGS, isFullModel } from '../lib/draftSettings'

type Scoring = { settings: DraftSettings; preview: ScorePreview | null }

/**
 * The score a player has under the selected draft settings, or undefined when
 * they cannot be scored. With the full model that is the score they were
 * loaded with. Until the first preview for other settings has arrived, the
 * full model's scores stand in.
 */
export const scoreOf = (s: Scoring, p: Player): DraftScore | undefined => {
  if (!isFullModel(s.settings) && s.preview) return s.preview.scores[p.id]
  return p.skill_score != null && p.skill_score >= 0 ? { score: p.skill_score } : undefined
}

/** How the selected settings assess a player's rank. */
export const rankCheckOf = (s: Scoring, p: Player): RankCheck | undefined =>
  !isFullModel(s.settings) && s.preview ? s.preview.rank_checks[p.id] : p.rank_check

// Typing in a weight field should not fire a request per keystroke
const PREVIEW_DELAY_MS = 200
let previewTimer: ReturnType<typeof setTimeout> | undefined
let previewRun = 0

/** The draft that is open: the chosen one, or else the newest. */
export const activeDraftOf = (s: { tournament: Tournament | null; activeDraftId: string | null }): Draft | undefined => {
  const drafts = s.tournament?.drafts ?? []
  return drafts.find(d => d.id === s.activeDraftId) ?? drafts[drafts.length - 1]
}

const withAverage = (team: Team, players: Team['players']): Team => ({
  ...team, players, avg_skill_score: averageScore(players.map(p => p.skill_score)),
})

// The API sends null for an empty list in a few places
const normalise = (t: Tournament): Tournament => ({
  ...t,
  players: t.players ?? [],
  drafts: t.drafts ?? [],
  teammate_requests: t.teammate_requests ?? [],
})

interface TournamentStore {
  tournament: Tournament | null
  loading: boolean
  addingPlayer: boolean
  error: string | null
  requestError: string | null
  draftError: string | null
  activeDraftId: string | null
  // The settings the next draft is made with. The player list shows the
  // scores they give: `preview` holds them unless they are the full model's.
  settings: DraftSettings
  preview: ScorePreview | null
  previewPending: boolean
  setSettings: (settings: DraftSettings) => void
  refreshPreview: () => Promise<void>

  addTeammateRequest: (requesterId: string, requestedId: string) => Promise<boolean>
  removeTeammateRequest: (requestId: string) => Promise<void>
  createTournament: (name: string, teamSize: number) => Promise<void>
  loadTournament: (id: string) => Promise<void>
  refreshingIds: string[]
  addPlayer: (steamUrl: string) => Promise<boolean>
  refreshPlayers: (playerIds: string[]) => Promise<void>
  removePlayer: (playerId: string) => Promise<void>
  setActiveDraft: (draftId: string) => void
  createDraft: (name: string, settings: DraftSettings) => Promise<boolean>
  renameDraft: (draftId: string, name: string) => Promise<void>
  deleteDraft: (draftId: string) => Promise<void>
  movePlayer: (draftId: string, playerId: string, fromTeamId: string, toTeamId: string) => Promise<void>
  clearError: () => void
  reset: () => void
}

export const useTournamentStore = create<TournamentStore>((set, get) => ({
  tournament: null,
  loading: false,
  addingPlayer: false,
  error: null,
  requestError: null,
  draftError: null,
  activeDraftId: null,
  settings: DEFAULT_SETTINGS,
  preview: null,
  previewPending: false,
  refreshingIds: [],

  setSettings: (settings) => {
    set({ settings })
    clearTimeout(previewTimer)
    if (isFullModel(settings)) {
      previewRun++ // drop any answer still on its way
      set({ preview: null, previewPending: false })
      return
    }
    set({ previewPending: true })
    previewTimer = setTimeout(() => get().refreshPreview(), PREVIEW_DELAY_MS)
  },

  // Scores every player with the selected settings. Called when the settings
  // or the players change. Only the latest answer is kept.
  refreshPreview: async () => {
    const { tournament, settings } = get()
    if (!tournament || isFullModel(settings)) {
      set({ preview: null, previewPending: false })
      return
    }
    const run = ++previewRun
    set({ previewPending: true })
    try {
      const preview = await api.previewScores(tournament.id, settings)
      if (run === previewRun) {
        set({ preview: { scores: preview.scores ?? {}, rank_checks: preview.rank_checks ?? {} }, previewPending: false })
      }
    } catch (e) {
      if (run === previewRun) set({ draftError: (e as Error).message, previewPending: false })
    }
  },

  clearError: () => set({ error: null }),

  reset: () => set({
    tournament: null, loading: false, addingPlayer: false, error: null,
    requestError: null, draftError: null, activeDraftId: null,
    settings: DEFAULT_SETTINGS, preview: null, previewPending: false,
  }),

  createTournament: async (name, teamSize) => {
    set({ loading: true, error: null })
    try {
      const tournament = await api.createTournament(name, teamSize)
      set({ tournament: normalise(tournament), activeDraftId: null })
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
      set({ tournament: normalise(tournament), requestError: null, draftError: null, preview: null })
      get().refreshPreview()
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
      get().refreshPreview()
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
      set({ tournament: normalise(fresh) })
      get().refreshPreview()
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
          // …and takes them out of every draft
          drafts: s.tournament!.drafts.map(d => ({
            ...d,
            teams: d.teams.map(t =>
              t.players.some(p => p.id === playerId) ? withAverage(t, t.players.filter(p => p.id !== playerId)) : t,
            ),
          })),
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

  setActiveDraft: (draftId) => set({ activeDraftId: draftId }),

  createDraft: async (name, settings) => {
    const { tournament } = get()
    if (!tournament) return false
    set({ loading: true, draftError: null })
    try {
      const draft = await api.createDraft(tournament.id, name, settings)
      set(s => ({
        tournament: { ...s.tournament!, drafts: [...s.tournament!.drafts, draft], status: 'balanced' },
        activeDraftId: draft.id,
      }))
      return true
    } catch (e) {
      set({ draftError: (e as Error).message })
      return false
    } finally {
      set({ loading: false })
    }
  },

  renameDraft: async (draftId, name) => {
    const { tournament } = get()
    if (!tournament) return
    set({ draftError: null })
    try {
      const saved = await api.renameDraft(tournament.id, draftId, name)
      set(s => ({
        tournament: {
          ...s.tournament!,
          drafts: s.tournament!.drafts.map(d => (d.id === draftId ? { ...d, name: saved.name } : d)),
        },
      }))
    } catch (e) {
      set({ draftError: (e as Error).message })
    }
  },

  deleteDraft: async (draftId) => {
    const { tournament } = get()
    if (!tournament) return
    set({ draftError: null })
    try {
      await api.deleteDraft(tournament.id, draftId)
      set(s => ({
        tournament: { ...s.tournament!, drafts: s.tournament!.drafts.filter(d => d.id !== draftId) },
        activeDraftId: s.activeDraftId === draftId ? null : s.activeDraftId,
      }))
    } catch (e) {
      set({ draftError: (e as Error).message })
    }
  },

  movePlayer: async (draftId, playerId, fromTeamId, toTeamId) => {
    const { tournament } = get()
    if (!tournament) return

    // Optimistic update
    set(s => {
      const draft = s.tournament!.drafts.find(d => d.id === draftId)
      const movingPlayer = draft?.teams.find(t => t.id === fromTeamId)?.players.find(p => p.id === playerId)
      if (!draft || !movingPlayer) return s

      const teams = draft.teams.map(team => {
        if (team.id === fromTeamId) return withAverage(team, team.players.filter(p => p.id !== playerId))
        if (team.id === toTeamId) return withAverage(team, [...team.players, movingPlayer])
        return team
      })
      return {
        tournament: {
          ...s.tournament!,
          drafts: s.tournament!.drafts.map(d => (d.id === draftId ? { ...d, teams } : d)),
        },
      }
    })

    try {
      await api.movePlayer(tournament.id, playerId, fromTeamId, toTeamId)
    } catch (e) {
      // Revert on failure
      set({ draftError: (e as Error).message })
      get().loadTournament(tournament.id)
    }
  },
}))
