import type { Tournament, Player, Team, BalanceResult } from '../types'

async function request<T>(path: string, options?: RequestInit): Promise<T> {
    const res = await fetch(`/api${path}`, {
        headers: { 'Content-Type': 'application/json' },
        ...options,
    })
    if (!res.ok) {
        const err = await res.json().catch(() => ({}))
        throw new Error((err as { error?: string }).error ?? `HTTP ${res.status}`)
    }
    return res.json() as Promise<T>
}

export const createTournament = (name: string, team_size: number): Promise<Tournament> =>
    request('/tournaments', {
        method: 'POST',
        body: JSON.stringify({ name, team_size }),
    })

export const getTournament = (id: string): Promise<Tournament> =>
    request(`/tournaments/${id}`)

export const listTournaments = (): Promise<Tournament[]> =>
    request('/tournaments')

export const addPlayer = (tournamentId: string, steam_url: string): Promise<Player> =>
    request(`/tournaments/${tournamentId}/players`, {
        method: 'POST',
        body: JSON.stringify({ steam_url }),
    })

export const removePlayer = (tournamentId: string, playerId: string): Promise<{ ok: boolean }> =>
    request(`/tournaments/${tournamentId}/players/${playerId}`, {
        method: 'DELETE',
    })

export const refreshPlayer = (tournamentId: string, playerId: string): Promise<Player> =>
    request(`/tournaments/${tournamentId}/players/${playerId}/refresh`, {
        method: 'POST',
    })

export const balanceTeams = (tournamentId: string, team_size?: number): Promise<BalanceResult> =>
    request(`/tournaments/${tournamentId}/balance`, {
        method: 'POST',
        body: JSON.stringify({ team_size }),
    })

export const movePlayer = (
    tournamentId: string,
    player_id: string,
    from_team_id: string,
    to_team_id: string,
): Promise<{ ok: boolean }> =>
    request(`/tournaments/${tournamentId}/teams/move`, {
        method: 'PATCH',
        body: JSON.stringify({ player_id, from_team_id, to_team_id }),
    })

export type { Tournament, Player, Team }
