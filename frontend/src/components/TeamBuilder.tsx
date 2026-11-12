import { useState } from 'react'
import { useTournamentStore } from '../store/tournament'
import { teamSpread, teamVariance } from '../lib/stats'

/** Balance metrics, always computed from the teams as they currently are. */
export function BalanceMetrics() {
  const teams = useTournamentStore(s => s.tournament?.teams ?? [])
  if (teams.length === 0) return null

  const variance = teamVariance(teams)
  const spread = teamSpread(teams)
  const verdict = variance < 1 ? 'Excellent' : variance < 5 ? 'Good' : 'Poor'
  const color = variance < 1 ? 'text-emerald-400' : variance < 5 ? 'text-amber-400' : 'text-red-400'

  return (
    <div className="flex items-baseline gap-4 text-sm tabular-nums">
      <span className={`font-semibold ${color}`}>{verdict} balance</span>
      <span title="Variance of team averages. 0 is perfectly even.">
        <span className="text-zinc-500">σ² </span>
        <span className="text-zinc-100 font-medium">{variance.toFixed(2)}</span>
      </span>
      <span title="Strongest minus weakest team average">
        <span className="text-zinc-500">spread </span>
        <span className="text-zinc-100 font-medium">{spread.toFixed(2)}</span>
      </span>
    </div>
  )
}

export default function TeamBuilder() {
  const { tournament, balanceTeams, loading } = useTournamentStore()
  const [confirmRebalance, setConfirmRebalance] = useState(false)

  if (!tournament) return null

  const scored = tournament.players.filter(p => p.skill_score != null && p.skill_score >= 0).length
  const size = tournament.team_size
  const teamsPossible = Math.floor(scored / size)
  const benched = scored - teamsPossible * size
  const teamsExist = tournament.teams.length > 0
  const canBalance = teamsPossible >= 2

  const run = async () => {
    setConfirmRebalance(false)
    await balanceTeams()
  }

  return (
    <div className="panel px-3 py-2.5 flex items-center justify-between gap-4 flex-wrap">
      <div className="text-sm text-zinc-400">
        {canBalance ? (
          <>
            <span className="text-zinc-100 font-medium">{teamsPossible} teams</span> of {size} from {scored} scored players
            {benched > 0 && <span className="text-amber-400"> · {benched} benched</span>}
          </>
        ) : (
          <>Need {size * 2} scored players, have {scored}</>
        )}
      </div>

      {confirmRebalance ? (
        <div className="flex items-center gap-2 text-sm">
          <span className="text-amber-400">Replaces current teams and manual moves.</span>
          <button onClick={run} className="btn-primary">Re-draft</button>
          <button onClick={() => setConfirmRebalance(false)} className="btn">Cancel</button>
        </div>
      ) : (
        <button
          onClick={() => (teamsExist ? setConfirmRebalance(true) : run())}
          disabled={loading || !canBalance}
          className="btn-primary"
        >
          {loading ? 'Drafting…' : teamsExist ? 'Re-draft teams' : 'Draft balanced teams'}
        </button>
      )}
    </div>
  )
}
