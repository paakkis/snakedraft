import { useState } from 'react'
import { useTournamentStore } from '../store/tournament'

function VarianceBadge({ variance }: { variance: number }) {
  const label = variance < 1 ? 'Excellent' : variance < 5 ? 'Good' : 'Poor'
  const color = variance < 1
    ? 'bg-green-900 text-green-300 border-green-700'
    : variance < 5
    ? 'bg-yellow-900 text-yellow-300 border-yellow-700'
    : 'bg-red-900 text-red-300 border-red-700'

  return (
    <span className={`text-xs font-semibold px-2 py-1 rounded border ${color}`}>
      {label} (σ²={variance.toFixed(2)})
    </span>
  )
}

export default function TeamBuilder() {
  const { tournament, balanceTeams, loading } = useTournamentStore()
  const [confirmRebalance, setConfirmRebalance] = useState(false)

  if (!tournament) return null

  const scoredPlayers = tournament.players.filter(
    p => p.skill_score !== null && p.skill_score !== undefined && p.skill_score >= 0,
  )
  const teamsExist = tournament.teams.length > 0
  const canBalance = scoredPlayers.length >= tournament.team_size * 2

  const handleBalance = async () => {
    if (teamsExist && !confirmRebalance) {
      setConfirmRebalance(true)
      return
    }
    setConfirmRebalance(false)
    await balanceTeams()
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between flex-wrap gap-2">
        <div className="text-sm text-gray-400">
          {scoredPlayers.length} players with scores
          {' · '}team size {tournament.team_size}
          {' · '}
          {scoredPlayers.length >= tournament.team_size * 2
            ? `${Math.floor(scoredPlayers.length / tournament.team_size)} teams possible`
            : 'need more players'}
        </div>
        {tournament.variance !== undefined && (
          <VarianceBadge variance={tournament.variance} />
        )}
      </div>

      {confirmRebalance && (
        <div className="bg-yellow-950 border border-yellow-700 rounded-lg px-4 py-3 text-sm text-yellow-300">
          Manual moves will be lost. Re-balance anyway?
          <div className="flex gap-2 mt-2">
            <button
              onClick={handleBalance}
              className="px-3 py-1 bg-yellow-700 hover:bg-yellow-600 text-white rounded text-xs font-semibold"
            >
              Yes, re-balance
            </button>
            <button
              onClick={() => setConfirmRebalance(false)}
              className="px-3 py-1 bg-gray-700 hover:bg-gray-600 text-gray-200 rounded text-xs"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {!confirmRebalance && (
        <button
          onClick={handleBalance}
          disabled={loading || !canBalance}
          className="w-full py-3 bg-indigo-600 hover:bg-indigo-500 disabled:bg-gray-800 disabled:text-gray-600 text-white font-bold rounded-lg transition-colors flex items-center justify-center gap-2"
        >
          {loading ? (
            <>
              <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
              </svg>
              Balancing...
            </>
          ) : (
            <>⚡ {teamsExist ? 'Re-generate' : 'Generate'} Balanced Teams</>
          )}
        </button>
      )}
    </div>
  )
}
