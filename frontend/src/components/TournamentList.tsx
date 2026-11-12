import { useEffect, useState } from 'react'
import { listTournaments } from '../api/client'
import type { Tournament } from '../types'
import { useTournamentStore } from '../store/tournament'

const STATUS_LABEL: Record<string, string> = {
  signup: 'Sign-up',
  balanced: 'Teams drafted',
  active: 'Active',
  finished: 'Finished',
}

export default function TournamentList() {
  const { loadTournament } = useTournamentStore()
  const [tournaments, setTournaments] = useState<Tournament[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    listTournaments()
      .then(setTournaments)
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <p className="text-zinc-500 text-sm">Loading tournaments…</p>
  if (tournaments.length === 0) return null

  return (
    <div className="panel">
      <h2 className="panel-title px-3 py-2.5 border-b border-zinc-800">Tournaments</h2>
      <ul className="divide-y divide-zinc-800/70">
        {tournaments.map(t => (
          <li key={t.id}>
            <button
              onClick={() => loadTournament(t.id)}
              className="w-full flex items-center justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-zinc-800/50"
            >
              <span>
                <span className="font-medium text-zinc-100">{t.name}</span>
                <span className="ml-2 text-xs text-zinc-500">{t.team_size}v{t.team_size}</span>
              </span>
              <span className="text-xs text-zinc-500">
                {STATUS_LABEL[t.status] ?? t.status} · {new Date(t.created_at).toLocaleDateString()}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
