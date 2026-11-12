import { useState, type FormEvent } from 'react'
import type { RequestStatus, TeammateRequest } from '../types'
import { useTournamentStore } from '../store/tournament'

const STATUS: Record<RequestStatus, { label: string; className: string; title: string }> = {
  honored: {
    label: 'Together',
    className: 'text-emerald-400',
    title: 'These players are on the same team',
  },
  dropped_for_balance: {
    label: 'Split',
    className: 'text-amber-400',
    title: 'These players are on different teams — keeping them together would have made the teams unfair',
  },
  illegal_chain: {
    label: 'Chain too long',
    className: 'text-red-400',
    title: 'This request would chain a full team of friends together',
  },
  player_unavailable: {
    label: 'Not drafted',
    className: 'text-zinc-500',
    title: 'One of these players is not on a team (benched or no skill score)',
  },
}

export default function TeammateRequests() {
  const {
    tournament, requestError, requestStatuses,
    addTeammateRequest, removeTeammateRequest,
  } = useTournamentStore()
  const [requesterId, setRequesterId] = useState('')
  const [requestedId, setRequestedId] = useState('')

  if (!tournament) return null

  const players = [...tournament.players].sort((a, b) => a.steam_name.localeCompare(b.steam_name))
  const nameOf = (id: string) => tournament.players.find(p => p.id === id)?.steam_name ?? 'Unknown player'
  const requests = tournament.teammate_requests
  const maxGroup = tournament.team_size - 1

  // Status follows the teams as they are right now, so it stays correct
  // after players are dragged between teams or the page is reloaded.
  const teamOf = new Map<string, string>()
  tournament.teams.forEach(t => t.players.forEach(p => teamOf.set(p.id, t.id)))
  const statusOf = (r: TeammateRequest): RequestStatus | undefined => {
    if (tournament.teams.length === 0) return undefined
    const a = teamOf.get(r.requester_id), b = teamOf.get(r.requested_id)
    if (a && b) return a === b ? 'honored' : 'dropped_for_balance'
    return requestStatuses[r.id] === 'illegal_chain' ? 'illegal_chain' : 'player_unavailable'
  }
  const together = requests.filter(r => statusOf(r) === 'honored').length

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!requesterId || !requestedId || requesterId === requestedId) return
    if (await addTeammateRequest(requesterId, requestedId)) setRequestedId('')
  }

  return (
    <div className="panel">
      <div className="flex items-baseline justify-between gap-3 flex-wrap px-3 py-2.5 border-b border-zinc-800">
        <h2 className="panel-title">
          Teammate requests <span className="text-zinc-600">{requests.length}</span>
          {tournament.teams.length > 0 && requests.length > 0 && (
            <span className="ml-2 normal-case tracking-normal font-normal text-zinc-500">
              {together} of {requests.length} together
            </span>
          )}
        </h2>
        <span className="text-xs text-zinc-500">
          Kept when teams stay fair · a friend group can have at most {maxGroup} players
        </span>
      </div>

      <div className="p-3 flex flex-col gap-3">
        <form onSubmit={handleSubmit} className="flex flex-wrap gap-2 items-center">
          <select
            value={requesterId}
            onChange={e => setRequesterId(e.target.value)}
            className="field flex-1 min-w-40"
            aria-label="Player making the request"
          >
            <option value="">Player…</option>
            {players.map(p => <option key={p.id} value={p.id}>{p.steam_name}</option>)}
          </select>
          <span className="text-sm text-zinc-500">wants to play with</span>
          <select
            value={requestedId}
            onChange={e => setRequestedId(e.target.value)}
            className="field flex-1 min-w-40"
            aria-label="Requested teammate"
          >
            <option value="">Teammate…</option>
            {players.filter(p => p.id !== requesterId).map(p => (
              <option key={p.id} value={p.id}>{p.steam_name}</option>
            ))}
          </select>
          <button
            type="submit"
            disabled={!requesterId || !requestedId || requesterId === requestedId}
            className="btn"
          >
            Add request
          </button>
        </form>

        {requestError && <p className="notice-error">{requestError}</p>}

        {requests.length > 0 && (
          <ul className="divide-y divide-zinc-800/70 border-t border-zinc-800 text-sm">
            {requests.map(r => {
              const key = statusOf(r)
              const status = key ? STATUS[key] : undefined
              return (
                <li key={r.id} className="flex items-center gap-3 py-1.5">
                  <span className="flex-1 min-w-0 truncate">
                    <span className="text-zinc-100">{nameOf(r.requester_id)}</span>
                    <span className="text-zinc-600"> → </span>
                    <span className="text-zinc-100">{nameOf(r.requested_id)}</span>
                  </span>
                  {status && (
                    <span title={status.title} className={`text-xs flex-shrink-0 ${status.className}`}>
                      {status.label}
                    </span>
                  )}
                  <button
                    onClick={() => removeTeammateRequest(r.id)}
                    className="btn-quiet hover:!text-red-400"
                    title="Remove request"
                  >
                    Remove
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </div>
    </div>
  )
}
