import { useState, useEffect } from 'react'
import { useTournamentStore } from './store/tournament'
import PlayerInput from './components/PlayerInput'
import PlayerList from './components/PlayerList'
import TeamBuilder from './components/TeamBuilder'
import TeamView from './components/TeamView'

function CreateTournamentForm() {
  const { createTournament, loading, error, clearError } = useTournamentStore()
  const [name, setName] = useState('')
  const [teamSize, setTeamSize] = useState(5)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name.trim()) return
    clearError()
    await createTournament(name.trim(), teamSize)
  }

  return (
    <div className="flex flex-col gap-3">
    <form onSubmit={handleSubmit} className="flex flex-wrap gap-3 items-end">
      <div className="flex-1 min-w-48">
        <label className="block text-xs text-gray-500 mb-1">Tournament name</label>
        <input
          type="text"
          value={name}
          onChange={e => setName(e.target.value)}
          placeholder="Friday Night Cup"
          className="w-full bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500 transition-colors"
        />
      </div>
      <div>
        <label className="block text-xs text-gray-500 mb-1">Team size</label>
        <select
          value={teamSize}
          onChange={e => setTeamSize(Number(e.target.value))}
          className="bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-gray-100 focus:outline-none focus:border-blue-500"
        >
          {[2, 3, 4, 5, 6].map(n => (
            <option key={n} value={n}>{n}v{n}</option>
          ))}
        </select>
      </div>
      <button
        type="submit"
        disabled={loading || !name.trim()}
        className="px-5 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-gray-700 disabled:text-gray-500 text-white font-semibold rounded-lg text-sm transition-colors"
      >
        {loading ? 'Creating...' : 'Create'}
      </button>
    </form>
    {error && (
      <p className="text-sm text-red-400 bg-red-950 border border-red-800 rounded px-3 py-2">
        {error}
      </p>
    )}
    </div>
  )
}

export default function App() {
  const { tournament, reset } = useTournamentStore()

  // Persist tournament ID in URL hash for easy sharing / reload
  useEffect(() => {
    if (tournament?.id) {
      window.location.hash = tournament.id
    }
  }, [tournament?.id])

  return (
    <div className="min-h-screen bg-gray-950">
      {/* Header */}
      <header className="border-b border-gray-800 bg-gray-900">
        <div className="max-w-7xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-2xl">🎮</span>
            <div>
              <h1 className="text-lg font-bold text-white leading-tight">CS2 Tournament Balancer</h1>
              {tournament && (
                <p className="text-xs text-gray-500">{tournament.name} · {tournament.team_size}v{tournament.team_size}</p>
              )}
            </div>
          </div>
          {tournament && (
            <button
              onClick={() => {
                window.location.hash = ''
                reset()
              }}
              className="text-xs text-gray-500 hover:text-gray-300 transition-colors"
            >
              ← New tournament
            </button>
          )}
        </div>
      </header>

      <main className="max-w-7xl mx-auto px-4 py-6 space-y-8">
        {!tournament ? (
          <section className="max-w-xl mx-auto">
            <div className="bg-gray-900 border border-gray-800 rounded-xl p-6">
              <h2 className="font-semibold text-white mb-4">New Tournament</h2>
              <CreateTournamentForm />
            </div>
          </section>
        ) : (
          <>
            {/* Add Player */}
            <section className="bg-gray-900 border border-gray-800 rounded-xl p-5">
              <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-3">
                Add Player
              </h2>
              <PlayerInput />
            </section>

            {/* Player List */}
            <section>
              <PlayerList />
            </section>

            {/* Balance */}
            {tournament.players.length >= 2 && (
              <section className="bg-gray-900 border border-gray-800 rounded-xl p-5">
                <TeamBuilder />
              </section>
            )}

            {/* Teams */}
            {tournament.teams.length > 0 && (
              <section>
                <TeamView />
              </section>
            )}
          </>
        )}
      </main>
    </div>
  )
}
