import { useState, type FormEvent } from 'react'
import type { Player, RequestStatus, TeammateRequest } from '../types'
import { useTournamentStore } from '../store/tournament'

const STATUS: Record<RequestStatus, { label: string; className: string; title: string }> = {
  honored: {
    label: 'Together',
    className: 'text-emerald-400',
    title: 'On the same team',
  },
  dropped_for_balance: {
    label: 'Split',
    className: 'text-amber-400',
    title: 'On different teams',
  },
  illegal_chain: {
    label: 'Chain too long',
    className: 'text-red-400',
    title: 'Would link a full team of friends',
  },
  player_unavailable: {
    label: 'Not drafted',
    className: 'text-zinc-500',
    title: 'One of them is not on a team',
  },
}

type View = 'list' | 'visual'
const VIEW_KEY = 'teammate-requests-view'

const ARC_COLOR: Record<RequestStatus | 'none', string> = {
  honored: '#34d399',
  dropped_for_balance: '#fbbf24',
  illegal_chain: '#f87171',
  player_unavailable: '#71717a',
  none: '#a1a1aa',
}

function Chip({ player, selected, onClick }: { player: Player; selected: boolean; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={player.steam_name}
      className={`w-28 flex items-center gap-1.5 px-1.5 py-1 rounded border text-xs text-left ${
        selected
          ? 'border-amber-500 bg-amber-500/15 text-zinc-50'
          : 'border-zinc-700 bg-zinc-800 text-zinc-200 hover:border-zinc-500'
      }`}
    >
      {player.avatar_url
        ? <img src={player.avatar_url} alt="" className="w-4 h-4 rounded-sm flex-shrink-0" />
        : <span className="w-4 h-4 rounded-sm bg-zinc-700 flex-shrink-0" />}
      <span className="truncate">{player.steam_name}</span>
    </button>
  )
}

// Chip width (w-28) plus the gap between chips (gap-2), in px
const PITCH = 120
const ARC_HEIGHT = 30

/** One friend group: its players in a row, with an arc for every request between them. */
function FriendGroup({ members, links, selectedId, onPick, onRemove, statusOf }: {
  members: Player[]
  links: TeammateRequest[]
  selectedId: string
  onPick: (id: string) => void
  onRemove: (id: string) => void
  statusOf: (r: TeammateRequest) => RequestStatus | undefined
}) {
  const x = (id: string) => members.findIndex(m => m.id === id) * PITCH + (PITCH - 8) / 2
  return (
    <div className="rounded border border-zinc-800 bg-zinc-950/40 px-2 pb-2">
      <svg width={members.length * PITCH - 8} height={ARC_HEIGHT} className="block overflow-visible">
        {links.map(r => {
          const a = x(r.requester_id), b = x(r.requested_id)
          // Longer links arc higher so they do not hide the short ones under them
          const lift = Math.min(ARC_HEIGHT - 6, 10 + Math.abs(a - b) / PITCH * 7)
          const status = statusOf(r)
          const d = `M ${a} ${ARC_HEIGHT} Q ${(a + b) / 2} ${ARC_HEIGHT - lift * 2} ${b} ${ARC_HEIGHT}`
          return (
            <g key={r.id} onClick={() => onRemove(r.id)} className="cursor-pointer group">
              <title>{`${status ? STATUS[status].label + '. ' : ''}Click to remove`}</title>
              {/* Wide invisible stroke makes the thin arc easy to hit */}
              <path d={d} fill="none" stroke="transparent" strokeWidth="12" />
              <path
                d={d} fill="none" strokeWidth="2" strokeLinecap="round"
                stroke={ARC_COLOR[status ?? 'none']}
                className="group-hover:stroke-red-400"
              />
            </g>
          )
        })}
      </svg>
      <div className="flex gap-2">
        {members.map(m => (
          <Chip key={m.id} player={m} selected={selectedId === m.id} onClick={() => onPick(m.id)} />
        ))}
      </div>
    </div>
  )
}

export default function TeammateRequests() {
  const {
    tournament, requestError, requestStatuses,
    addTeammateRequest, removeTeammateRequest,
  } = useTournamentStore()
  const [requesterId, setRequesterId] = useState('')
  const [requestedId, setRequestedId] = useState('')
  const [view, setView] = useState<View>(() => {
    try { return localStorage.getItem(VIEW_KEY) === 'visual' ? 'visual' : 'list' } catch { return 'list' }
  })
  const [pickedId, setPickedId] = useState('')

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

  const changeView = (v: View) => {
    setView(v)
    setPickedId('')
    try { localStorage.setItem(VIEW_KEY, v) } catch { /* storage unavailable */ }
  }

  // Visual view: click one player, then another, to link them
  const pick = async (id: string) => {
    if (!pickedId) return setPickedId(id)
    if (pickedId !== id) await addTeammateRequest(pickedId, id)
    setPickedId('')
  }

  // Friend groups: players connected through requests, directly or in a chain
  const groupOf = new Map<string, string>()
  const find = (id: string): string => {
    const parent = groupOf.get(id)
    if (!parent || parent === id) return id
    const root = find(parent)
    groupOf.set(id, root)
    return root
  }
  requests.forEach(r => groupOf.set(find(r.requester_id), find(r.requested_id)))
  const groups = new Map<string, Player[]>()
  players.forEach(p => {
    if (!requests.some(r => r.requester_id === p.id || r.requested_id === p.id)) return
    const root = find(p.id)
    groups.set(root, [...(groups.get(root) ?? []), p])
  })
  const loose = players.filter(p => !groups.has(find(p.id)))

  return (
    <div className="panel">
      <div className="flex items-center justify-between gap-3 flex-wrap px-3 py-2 border-b border-zinc-800">
        <h2 className="panel-title">
          Teammate requests <span className="text-zinc-600">{requests.length}</span>
          {tournament.teams.length > 0 && requests.length > 0 && (
            <span className="ml-2 normal-case tracking-normal font-normal text-zinc-500">
              {together} of {requests.length} together
            </span>
          )}
        </h2>
        <div className="flex items-center gap-3">
          <span className="text-xs text-zinc-500">
            Kept if teams stay fair · max {maxGroup} friends per team
          </span>
          <div className="flex rounded border border-zinc-700 overflow-hidden text-xs" role="tablist">
            {(['list', 'visual'] as const).map(v => (
              <button
                key={v}
                type="button"
                role="tab"
                aria-selected={view === v}
                onClick={() => changeView(v)}
                className={`px-2.5 py-1 ${view === v ? 'bg-zinc-700 text-zinc-50' : 'text-zinc-400 hover:text-zinc-100'}`}
              >
                {v === 'list' ? 'List' : 'Visual'}
              </button>
            ))}
          </div>
        </div>
      </div>

      <div className="p-3 flex flex-col gap-3">
        {requestError && <p className="notice-error">{requestError}</p>}

        {view === 'visual' ? (
          <>
            <p className="text-xs text-zinc-500">
              {pickedId
                ? <>Now click who <span className="text-amber-400">{nameOf(pickedId)}</span> wants to play with.</>
                : 'Click two players to link them. Click a line to remove it.'}
            </p>
            {groups.size > 0 && (
              <div className="flex flex-wrap gap-2 items-end">
                {[...groups.entries()].map(([root, members]) => (
                  <FriendGroup
                    key={root}
                    members={members}
                    links={requests.filter(r => find(r.requester_id) === root)}
                    selectedId={pickedId}
                    onPick={pick}
                    onRemove={removeTeammateRequest}
                    statusOf={statusOf}
                  />
                ))}
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              {loose.map(p => (
                <Chip key={p.id} player={p} selected={pickedId === p.id} onClick={() => pick(p.id)} />
              ))}
            </div>
          </>
        ) : (
          <>
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
          </>
        )}
      </div>
    </div>
  )
}
