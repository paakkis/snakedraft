import { useState, useEffect } from 'react'
import { useTournamentStore } from './store/tournament'
import PlayerInput from './components/PlayerInput'
import PlayerList from './components/PlayerList'
import TeammateRequests from './components/TeammateRequests'
import TeamBuilder from './components/TeamBuilder'
import TeamView from './components/TeamView'
import TournamentList from './components/TournamentList'
import CopyButton from './components/CopyButton'

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
      <form onSubmit={handleSubmit} className="flex flex-wrap gap-2 items-end">
        <label className="flex-1 min-w-48">
          <span className="block text-xs text-zinc-500 mb-1">Name</span>
          <input
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder="Friday Night Cup"
            className="field w-full"
            autoFocus
          />
        </label>
        <label>
          <span className="block text-xs text-zinc-500 mb-1">Format</span>
          <select value={teamSize} onChange={e => setTeamSize(Number(e.target.value))} className="field">
            {[2, 3, 4, 5, 6].map(n => <option key={n} value={n}>{n}v{n}</option>)}
          </select>
        </label>
        <button type="submit" disabled={loading || !name.trim()} className="btn-primary">
          {loading ? 'Creating…' : 'Create'}
        </button>
      </form>
      {error && <p className="notice-error">{error}</p>}
    </div>
  )
}

export default function App() {
  const { tournament, reset, loadTournament } = useTournamentStore()

  // On first load, restore tournament from URL hash
  useEffect(() => {
    const id = window.location.hash.replace('#', '')
    if (id) loadTournament(id)
  }, [])

  // Keep hash in sync as the active tournament changes
  useEffect(() => {
    if (tournament?.id) window.location.hash = tournament.id
  }, [tournament?.id])

  return (
    <div className="min-h-screen">
      {tournament && (
        <header className="border-b border-zinc-800 bg-zinc-900">
          <div className="max-w-[90rem] mx-auto px-4 h-12 flex items-center justify-between gap-4">
            <span className="font-semibold text-zinc-100 truncate">
              {tournament.name}
              <span className="ml-2 text-sm font-normal text-zinc-400">
                {tournament.team_size}v{tournament.team_size}
              </span>
            </span>
            <div className="flex items-center gap-1 flex-shrink-0">
              <CopyButton text={() => window.location.href} className="btn-quiet" title="Copy tournament link">
                Copy link
              </CopyButton>
              <button onClick={() => { window.location.hash = ''; reset() }} className="btn-quiet">
                All tournaments
              </button>
            </div>
          </div>
        </header>
      )}

      <main className="max-w-[90rem] mx-auto px-4 py-5">
        {!tournament ? (
          <div className="max-w-xl mx-auto space-y-4">
            <div className="panel p-4">
              <h2 className="panel-title mb-3">New tournament</h2>
              <CreateTournamentForm />
            </div>
            <TournamentList />
          </div>
        ) : (
          <div className="space-y-4">
            <div className="panel p-3">
              <h2 className="panel-title mb-2">Add players</h2>
              <PlayerInput />
            </div>
            <PlayerList />
            {tournament.players.length >= 2 && <TeammateRequests />}
            {tournament.players.length >= 2 && <TeamBuilder />}
            <TeamView />
          </div>
        )}
      </main>
    </div>
  )
}
