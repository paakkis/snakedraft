import { useEffect, useState } from 'react'
import { listTournaments } from '../api/client'
import type { Tournament } from '../types'
import { useTournamentStore } from '../store/tournament'

const STATUS_LABEL: Record<string, string> = {
  signup: 'Sign-up',
  balanced: 'Balanced',
  active: 'Active',
  finished: 'Finished',
}

const STATUS_COLOR: Record<string, string> = {
  signup: 'bg-blue-900 text-blue-300',
  balanced: 'bg-green-900 text-green-300',
  active: 'bg-yellow-900 text-yellow-300',
  finished: 'bg-gray-700 text-gray-400',
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

  if (loading) {
    return <p className="text-gray-500 text-sm">Loading tournaments…</p>
  }

  if (tournaments.length === 0) {
    return null
  }

  return (
    <div>
      <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
        Previous Tournaments
      </h2>
      <div className="flex flex-col gap-2">
        {tournaments.map(t => (
          <button
            key={t.id}
            onClick={() => loadTournament(t.id)}
            className="flex items-center justify-between px-4 py-3 rounded-lg bg-gray-800 border border-gray-700 hover:border-gray-500 hover:bg-gray-750 transition-colors text-left"
          >
            <div>
              <span className="font-semibold text-white text-sm">{t.name}</span>
              <span className="ml-2 text-xs text-gray-500">{t.team_size}v{t.team_size}</span>
            </div>
            <span className={`text-xs font-semibold px-2 py-0.5 rounded ${STATUS_COLOR[t.status] ?? 'bg-gray-700 text-gray-400'}`}>
              {STATUS_LABEL[t.status] ?? t.status}
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}
