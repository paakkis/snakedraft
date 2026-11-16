import { useState } from 'react'
import type { Draft, Player } from '../types'
import { useTournamentStore } from '../store/tournament'
import { averageScore, spread, teamSpread } from '../lib/stats'
import { describeSettings } from '../lib/draftSettings'

// One colour per team number. Team numbers mean nothing across drafts: the
// colours only tell the teams of one draft apart.
const TEAM_COLORS = [
  'bg-sky-500/20 text-sky-300 border-sky-500/50',
  'bg-amber-500/20 text-amber-300 border-amber-500/50',
  'bg-emerald-500/20 text-emerald-300 border-emerald-500/50',
  'bg-fuchsia-500/20 text-fuchsia-300 border-fuchsia-500/50',
  'bg-red-500/20 text-red-300 border-red-500/50',
  'bg-lime-500/20 text-lime-300 border-lime-500/50',
  'bg-violet-500/20 text-violet-300 border-violet-500/50',
  'bg-orange-500/20 text-orange-300 border-orange-500/50',
]

const hasScore = (p: Player) => p.skill_score != null && p.skill_score >= 0

/** Every draft side by side: how even each one is, and where each player ended up. */
export default function DraftCompare({ onOpen }: { onOpen: (draftId: string) => void }) {
  const tournament = useTournamentStore(s => s.tournament)
  // The team under the pointer, so its players light up down the column
  const [hover, setHover] = useState<{ draftId: string; team: number } | null>(null)

  if (!tournament) return null
  const { drafts, players, teammate_requests: requests } = tournament

  // Current full-model score of every player, to measure all drafts with one ruler
  const fullScore = new Map(players.map(p => [p.id, p.skill_score]))

  const teamIndex = (d: Draft) => {
    const index = new Map<string, number>()
    d.teams.forEach((t, i) => t.players.forEach(p => index.set(p.id, i)))
    return index
  }
  const indexes = new Map(drafts.map(d => [d.id, teamIndex(d)]))

  const summary = drafts.map(d => {
    const index = indexes.get(d.id)!
    const together = requests.filter(r => {
      const a = index.get(r.requester_id), b = index.get(r.requested_id)
      return a != null && a === b
    }).length
    return {
      draft: d,
      own: teamSpread(d.teams),
      full: spread(d.teams.map(t => averageScore(t.players.map(p => fullScore.get(p.id))))),
      together,
      benched: Object.keys(d.scores).filter(id => !index.has(id) && fullScore.has(id)).length,
      leftOut: players.filter(p => !d.scores[p.id]).length,
    }
  })
  const bestFull = Math.min(...summary.map(s => s.full))

  const spreadColor = (v: number) => (v <= 2 ? 'text-emerald-400' : v <= 5 ? 'text-amber-400' : 'text-red-400')

  return (
    <div className="flex flex-col gap-4">
      <div className="panel overflow-x-auto">
        <table className="w-full text-sm tabular-nums">
          <thead className="text-xs text-zinc-500 border-b border-zinc-800">
            <tr>
              <th className="px-3 py-2 text-left font-medium">Draft</th>
              <th className="px-3 py-2 text-left font-medium">Made with</th>
              <th className="px-3 py-2 text-right font-medium" title="Strongest minus weakest team average, on the scores the draft was made with">
                Spread, own scores
              </th>
              <th className="px-3 py-2 text-right font-medium" title="The same teams measured with the full model's current scores, so drafts made on different scores can be compared">
                Spread, full model
              </th>
              <th className="px-3 py-2 text-right font-medium" title="Teammate requests where both players are on the same team">
                Requests together
              </th>
              <th className="px-3 py-2 text-right font-medium" title="Benched: scored, but not on a team. Not scored: the settings could not rate them">
                Benched / not scored
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800/70">
            {summary.map(s => (
              <tr key={s.draft.id} className="hover:bg-zinc-800/40">
                <td className="px-3 py-1.5 whitespace-nowrap">
                  <button onClick={() => onOpen(s.draft.id)} className="font-medium text-zinc-100 hover:text-amber-400" title="Open this draft">
                    {s.draft.name}
                  </button>
                </td>
                <td className="px-3 py-1.5 text-xs text-zinc-400">{describeSettings(s.draft.settings).join(' · ')}</td>
                <td className={`px-3 py-1.5 text-right font-medium ${spreadColor(s.own)}`}>{s.own.toFixed(2)}</td>
                <td className={`px-3 py-1.5 text-right font-medium ${spreadColor(s.full)}`}>
                  {s.full.toFixed(2)}
                  {s.full === bestFull && summary.length > 1 && <span className="ml-1 text-xs font-normal text-zinc-500">most even</span>}
                </td>
                <td className="px-3 py-1.5 text-right text-zinc-300">
                  {requests.length > 0 ? `${s.together} of ${requests.length}` : <span className="text-zinc-700">–</span>}
                </td>
                <td className="px-3 py-1.5 text-right text-zinc-300">
                  {s.benched} / {s.leftOut}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="panel overflow-x-auto">
        <table className="w-full text-sm tabular-nums">
          <thead className="text-xs text-zinc-500 border-b border-zinc-800">
            <tr>
              <th className="px-3 py-2 text-left font-medium">Player</th>
              <th className="px-3 py-2 text-right font-medium" title="Current score in the player list">Full model</th>
              {drafts.map(d => (
                <th key={d.id} className="px-3 py-2 text-left font-medium whitespace-nowrap">
                  <button onClick={() => onOpen(d.id)} className="hover:text-zinc-100" title="Open this draft">{d.name}</button>
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-800/70">
            {players.map(p => (
              <tr key={p.id}>
                <td className="px-3 py-1">
                  <div className="flex items-center gap-2">
                    {p.avatar_url
                      ? <img src={p.avatar_url} alt="" className="w-5 h-5 rounded-sm flex-shrink-0" />
                      : <div className="w-5 h-5 rounded-sm bg-zinc-800 flex-shrink-0" />}
                    <span className="text-zinc-100 truncate max-w-48">{p.steam_name || p.steam64_id}</span>
                  </div>
                </td>
                <td className="px-3 py-1 text-right text-zinc-300">
                  {hasScore(p) ? p.skill_score!.toFixed(1) : <span className="text-zinc-700">–</span>}
                </td>
                {drafts.map(d => {
                  const team = indexes.get(d.id)!.get(p.id)
                  const score = d.scores[p.id]
                  const lit = hover != null && hover.draftId === d.id && hover.team === team
                  const dimmed = hover != null && hover.draftId === d.id && !lit
                  return (
                    <td key={d.id} className={`px-3 py-1 ${lit ? 'bg-zinc-800' : ''}`}>
                      <div className={`flex items-center gap-2 ${dimmed ? 'opacity-30' : ''}`}>
                        {team != null ? (
                          <span
                            onMouseEnter={() => setHover({ draftId: d.id, team })}
                            onMouseLeave={() => setHover(null)}
                            title={`${d.teams[team].name} in ${d.name}: ${d.teams[team].players.map(m => m.steam_name).join(', ')}`}
                            className={`inline-block w-16 text-center text-xs rounded border px-1 py-0.5 cursor-default ${TEAM_COLORS[team % TEAM_COLORS.length]}`}
                          >
                            {d.teams[team].name}
                          </span>
                        ) : (
                          <span className="inline-block w-16 text-center text-xs text-zinc-600">
                            {score ? 'bench' : 'not scored'}
                          </span>
                        )}
                        {score && (
                          <span
                            className={score.estimated ? 'text-amber-300 cursor-help' : 'text-zinc-400'}
                            title={score.estimated ? 'These settings cannot rate this player. Scored with the full model instead.' : 'Score in this draft'}
                          >
                            {score.estimated && '~'}{score.score.toFixed(1)}
                          </span>
                        )}
                      </div>
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
        <p className="px-3 py-2 border-t border-zinc-800 text-xs text-zinc-500">
          Hover over a team to light up its players. Team numbers are per draft: Team 1 in one draft has nothing to do with Team 1 in another.
        </p>
      </div>
    </div>
  )
}
