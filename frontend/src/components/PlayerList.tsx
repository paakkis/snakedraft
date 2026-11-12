import { useMemo, useState } from 'react'
import type { Player } from '../types'
import { useTournamentStore } from '../store/tournament'
import CopyButton from './CopyButton'
import FaceitLevel from './FaceitLevel'

type SortKey = 'score' | 'name' | 'faceit' | 'premier' | 'hours' | 'winrate' | 'kd' | 'recentKd'

const SORT_VALUE: Record<SortKey, (p: Player) => number | string | undefined> = {
  score: p => p.skill_score,
  name: p => p.steam_name?.toLowerCase(),
  faceit: p => p.faceit_elo ?? (p.faceit_level ? p.faceit_level * 100 : undefined),
  premier: p => p.premier_rating,
  hours: p => p.hours_played,
  winrate: p => p.leetify?.winrate ?? p.steam?.win_rate,
  kd: p => p.steam?.kd_ratio,
  recentKd: p => (p.faceit?.recent_matches ? p.faceit.recent_kd : undefined),
}

const steamUrl = (p: Player) => `https://steamcommunity.com/profiles/${p.steam64_id}`
const leetifyUrl = (p: Player) => `https://leetify.com/app/profile/${p.steam64_id}`
const faceitUrl = (nickname: string) => `https://www.faceit.com/en/players/${encodeURIComponent(nickname)}`

const num = (v: number | undefined | null, digits = 0) =>
  v == null ? <span className="text-zinc-700">–</span> : v.toLocaleString(undefined, {
    minimumFractionDigits: digits, maximumFractionDigits: digits,
  })

function timeAgo(iso?: string): string {
  if (!iso) return 'never'
  const mins = Math.round((Date.now() - new Date(iso).getTime()) / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins} min ago`
  if (mins < 60 * 24) return `${Math.round(mins / 60)} h ago`
  return `${Math.round(mins / 60 / 24)} d ago`
}

// A rank nobody has played for in this long says little about current skill.
const STALE_RANK_DAYS = 180

/** Amber note next to a rank that has not been played for in a long time. */
function StaleRank({ platform, lastPlayed, unknownNote }: {
  platform: string; lastPlayed?: string; unknownNote?: string
}) {
  let title: string
  if (lastPlayed) {
    const days = (Date.now() - new Date(lastPlayed).getTime()) / 86_400_000
    if (days < STALE_RANK_DAYS) return null
    const months = Math.round(days / 30.44)
    const age = months >= 24 ? `${Math.floor(months / 12)} years` : `${months} months`
    title = `Last rated ${platform} match ${age} ago (${new Date(lastPlayed).toLocaleDateString()})`
  } else if (unknownNote) {
    title = unknownNote
  } else {
    return null
  }
  return (
    <span className="text-xs text-amber-400 cursor-help" title={title} aria-label={title}>⚠</span>
  )
}

/** Where the skill score comes from, and why it may be missing. */
function source(p: Player): { label: string; className: string; title: string } {
  const hasScore = p.skill_score != null && p.skill_score >= 0
  if (p.privacy_mode === 'public' && hasScore) {
    return {
      label: p.faceit ? 'Leetify + Faceit' : 'Leetify',
      className: 'text-emerald-400',
      title: 'Rank (Faceit or Premier) and Leetify stats' + (p.steam ? ', 5% Steam lifetime stats' : ''),
    }
  }
  if (p.privacy_mode === 'faceit_only') {
    return {
      label: 'Faceit',
      className: 'text-orange-400',
      title: 'Faceit ELO and recent Faceit matches' + (p.steam ? ', 10% Steam lifetime stats' : ''),
    }
  }
  if (p.privacy_mode === 'steam_only') {
    return {
      label: 'Steam only',
      className: 'text-sky-400',
      title: 'Steam lifetime stats only. Less reliable.',
    }
  }
  if (!hasScore && p.privacy_mode === 'private') {
    return {
      label: 'Private',
      className: 'text-amber-400',
      title: 'Profile is private. Cannot be drafted.',
    }
  }
  return {
    label: 'No stats',
    className: 'text-red-400',
    title: 'No stats found. Cannot be drafted.',
  }
}

function ScoreCell({ score }: { score?: number }) {
  if (score == null || score < 0) return <span className="text-zinc-700">–</span>
  return (
    <div className="flex items-center justify-end gap-2">
      <div className="w-14 h-1 bg-zinc-800 rounded-sm overflow-hidden">
        <div className="h-full bg-amber-500" style={{ width: `${Math.min(score, 100)}%` }} />
      </div>
      <span className="font-semibold text-zinc-50 w-9 text-right">{score.toFixed(1)}</span>
    </div>
  )
}

export default function PlayerList() {
  const { tournament, removePlayer, refreshPlayers, refreshingIds, error, clearError } = useTournamentStore()
  const [sortKey, setSortKey] = useState<SortKey>('score')
  const [ascending, setAscending] = useState(false)

  const players = useMemo(() => {
    const list = [...(tournament?.players ?? [])]
    const value = SORT_VALUE[sortKey]
    list.sort((a, b) => {
      const va = value(a), vb = value(b)
      if (va == null && vb == null) return 0
      if (va == null) return 1 // missing values always last
      if (vb == null) return -1
      const cmp = va < vb ? -1 : va > vb ? 1 : 0
      return ascending ? cmp : -cmp
    })
    return list
  }, [tournament?.players, sortKey, ascending])

  if (!tournament) return null

  const undraftable = players.filter(p => p.skill_score == null || p.skill_score < 0).length
  const refreshing = refreshingIds.length > 0

  const sortBy = (key: SortKey) => {
    if (key === sortKey) setAscending(a => !a)
    else { setSortKey(key); setAscending(key === 'name') }
  }
  const Th = ({ k, children, align = 'right' }: { k?: SortKey; children: React.ReactNode; align?: 'left' | 'right' }) => (
    <th className={`px-2 py-2 font-medium whitespace-nowrap ${align === 'right' ? 'text-right' : 'text-left'}`}>
      {k ? (
        <button onClick={() => sortBy(k)} className={`hover:text-zinc-100 ${sortKey === k ? 'text-zinc-100' : ''}`}>
          {children}{sortKey === k ? (ascending ? ' ↑' : ' ↓') : ''}
        </button>
      ) : children}
    </th>
  )

  return (
    <div className="panel">
      <div className="flex items-center justify-between gap-3 flex-wrap px-3 py-2.5 border-b border-zinc-800">
        <h2 className="panel-title">
          Players <span className="text-zinc-600">{players.length}</span>
          {undraftable > 0 && (
            <span className="ml-2 normal-case tracking-normal font-normal text-red-400">
              {undraftable} without a score
            </span>
          )}
        </h2>
        {players.length > 0 && (
          <div className="flex items-center gap-1">
            <CopyButton
              text={() => players.map(p => p.steam64_id).join('\n')}
              className="btn-quiet"
              title="Copy all Steam64 IDs"
            >
              Copy all IDs
            </CopyButton>
            <button
              onClick={() => refreshPlayers(players.map(p => p.id))}
              disabled={refreshing}
              className="btn-quiet disabled:opacity-50"
              title="Refresh all players"
            >
              {refreshing ? `Refreshing, ${refreshingIds.length} left` : 'Refresh all'}
            </button>
          </div>
        )}
      </div>

      {error && (
        <div className="notice-error m-3 flex items-start justify-between gap-3">
          <span>{error}</span>
          <button onClick={clearError} className="text-red-400 hover:text-red-200">×</button>
        </div>
      )}

      {players.length === 0 ? (
        <p className="text-zinc-500 text-sm py-10 text-center">No players yet.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm tabular-nums">
            <thead className="text-xs text-zinc-500 border-b border-zinc-800">
              <tr>
                <Th align="left">#</Th>
                <Th k="name" align="left">Player</Th>
                <Th align="left">Steam64 ID</Th>
                <Th k="faceit">Faceit</Th>
                <Th k="premier">Premier</Th>
                <Th>Aim / Pos / Util</Th>
                <Th k="winrate">Win %</Th>
                <Th k="recentKd">Recent K/D</Th>
                <Th k="kd">Lifetime K/D</Th>
                <Th k="hours">Hours</Th>
                <Th align="left">Source</Th>
                <Th k="score">Score</Th>
                <Th>{''}</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800/70">
              {players.map((p, i) => {
                const src = source(p)
                const busy = refreshingIds.includes(p.id)
                const winrate = p.leetify?.winrate ?? p.steam?.win_rate
                return (
                  <tr key={p.id} className={`hover:bg-zinc-800/40 ${busy ? 'opacity-50' : ''}`}>
                    <td className="px-2 py-1.5 text-zinc-600 text-xs">{i + 1}</td>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center gap-2 min-w-40">
                        {p.avatar_url
                          ? <img src={p.avatar_url} alt="" className="w-6 h-6 rounded-sm flex-shrink-0" />
                          : <div className="w-6 h-6 rounded-sm bg-zinc-800 flex-shrink-0" />}
                        <a
                          href={steamUrl(p)} target="_blank" rel="noreferrer"
                          className="font-medium text-zinc-100 hover:text-amber-400 truncate max-w-48"
                          title="Open Steam profile"
                        >
                          {p.steam_name || p.steam64_id}
                        </a>
                        {p.is_banned && (
                          <span
                            className="text-[10px] font-bold px-1 rounded-sm bg-red-600 text-white"
                            title={`Banned${p.ban_info?.length ? ` on ${p.ban_info.map(b => b.platform).join(', ')}` : ''}`}
                          >
                            BAN
                          </span>
                        )}
                      </div>
                    </td>
                    <td className="px-2 py-1.5 whitespace-nowrap">
                      <span className="font-mono text-xs text-zinc-400 select-all">{p.steam64_id}</span>
                      <CopyButton text={p.steam64_id} className="btn-quiet ml-1" title="Copy Steam64 ID">ID</CopyButton>
                      <CopyButton text={steamUrl(p)} className="btn-quiet" title="Copy Steam profile link">Link</CopyButton>
                      <a href={leetifyUrl(p)} target="_blank" rel="noreferrer" className="btn-quiet" title="Open Leetify profile">
                        Leetify↗
                      </a>
                    </td>
                    {/* Fixed-width slots keep the numbers in one line down the column,
                        whether or not a row has a level badge or a warning. */}
                    <td className="px-2 py-1.5">
                      <div className="flex items-center justify-end gap-1.5">
                        {p.faceit_level
                          ? p.faceit?.nickname
                            ? <a
                                href={faceitUrl(p.faceit.nickname)} target="_blank" rel="noreferrer"
                                className="text-zinc-100 hover:text-amber-400"
                                title={`Open Faceit profile (${p.faceit.nickname})`}
                              >
                                {num(p.faceit_elo)}
                              </a>
                            : <span className="text-zinc-100" title="From Leetify, may be outdated">{num(p.faceit_elo)}</span>
                          : num(undefined)}
                        <span className="w-5 flex justify-center">
                          {p.faceit_level ? <FaceitLevel level={p.faceit_level} /> : null}
                        </span>
                        <span className="w-3.5 flex justify-center">
                          {p.faceit_level && p.faceit && (
                            <StaleRank
                              platform="Faceit"
                              lastPlayed={p.faceit.last_match_at}
                              unknownNote="No recent Faceit CS2 matches. Rank is probably old."
                            />
                          )}
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5">
                      <div className="flex items-center justify-end gap-1.5">
                        {num(p.premier_rating)}
                        <span className="w-3.5 flex justify-center">
                          {p.premier_rating != null && <StaleRank platform="Premier" lastPlayed={p.premier_rated_at} />}
                        </span>
                      </div>
                    </td>
                    <td className="px-2 py-1.5 text-right whitespace-nowrap">
                      {p.rating
                        ? <span title={p.leetify
                            ? `Leetify ${p.leetify.rating.toFixed(2)} · reaction ${p.leetify.reaction_time_ms.toFixed(0)} ms · HS accuracy ${p.leetify.head_accuracy.toFixed(0)}% · opening duels ${p.leetify.opening_duels.toFixed(0)}%`
                            : undefined}>
                            {p.rating.aim.toFixed(0)} / {p.rating.positioning.toFixed(0)} / {p.rating.utility.toFixed(0)}
                          </span>
                        : num(undefined)}
                    </td>
                    <td
                      className="px-2 py-1.5 text-right"
                      title={p.leetify ? `${p.leetify.total_matches.toLocaleString()} matches` : undefined}
                    >
                      {num(winrate)}
                    </td>
                    <td
                      className="px-2 py-1.5 text-right"
                      title={p.faceit?.recent_matches
                        ? `Last ${p.faceit.recent_matches} Faceit matches · ADR ${p.faceit.recent_adr.toFixed(0)} · HS ${p.faceit.recent_hs.toFixed(0)}% · won ${p.faceit.recent_win_rate.toFixed(0)}%`
                          + (p.faceit.last_match_at ? ` · last ${new Date(p.faceit.last_match_at).toLocaleDateString()}` : '')
                        : undefined}
                    >
                      {num(p.faceit?.recent_matches ? p.faceit.recent_kd : undefined, 2)}
                    </td>
                    <td
                      className="px-2 py-1.5 text-right text-zinc-500"
                      title={p.steam ? `Steam lifetime · HS ${p.steam.hs_percent.toFixed(0)}% · accuracy ${p.steam.accuracy.toFixed(0)}%` : undefined}
                    >
                      {num(p.steam?.kd_ratio, 2)}
                    </td>
                    <td
                      className="px-2 py-1.5 text-right"
                      title={p.steam && p.hours_played != null
                        ? `${p.steam.hours_last_2_weeks.toFixed(1)} h last 2 weeks`
                        : undefined}
                    >
                      {num(p.hours_played)}
                    </td>
                    <td className={`px-2 py-1.5 text-xs whitespace-nowrap ${src.className}`} title={src.title}>
                      {src.label}
                    </td>
                    <td className="px-2 py-1.5"><ScoreCell score={p.skill_score} /></td>
                    <td className="px-2 py-1.5 text-right whitespace-nowrap">
                      <button
                        onClick={() => refreshPlayers([p.id])}
                        disabled={refreshing}
                        className="btn-quiet disabled:opacity-40"
                        title={`Updated ${timeAgo(p.fetched_at)}`}
                      >
                        {busy ? '…' : 'Refresh'}
                      </button>
                      <button
                        onClick={() => {
                          if (window.confirm(`Remove ${p.steam_name || p.steam64_id} from the tournament?`)) removePlayer(p.id)
                        }}
                        className="btn-quiet hover:!text-red-400"
                        title="Remove player"
                      >
                        Remove
                      </button>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
