import { useState } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import type { Draft, Player, Team } from '../types'
import { activeDraftOf, useTournamentStore } from '../store/tournament'
import { teamMean, teamSpread, teamVariance } from '../lib/stats'
import { describeSettings } from '../lib/draftSettings'
import CopyButton from './CopyButton'
import DraftCompare from './DraftCompare'
import FaceitLevel from './FaceitLevel'

const ESTIMATED_TITLE = 'The settings of this draft cannot rate this player. Scored with the full model instead.'

/** Balance metrics, always computed from the teams as they currently are. */
function BalanceMetrics({ teams }: { teams: Team[] }) {
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

function PlayerRow({ player, linked, estimated }: { player: Player; linked: boolean; estimated: boolean }) {
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded bg-zinc-800/60 border border-zinc-800 text-sm tabular-nums">
      {player.avatar_url
        ? <img src={player.avatar_url} alt="" className="w-5 h-5 rounded-sm flex-shrink-0" />
        : <div className="w-5 h-5 rounded-sm bg-zinc-700 flex-shrink-0" />}
      <span className="flex-1 min-w-0 truncate text-zinc-100">{player.steam_name}</span>
      {linked && (
        <span className="text-[10px] text-emerald-400" title="Playing with a requested teammate">
          friend
        </span>
      )}
      {player.is_banned && <span className="text-[10px] font-bold px-1 rounded-sm bg-red-600 text-white">BAN</span>}
      {player.faceit_level != null && player.faceit_level > 0 && <FaceitLevel level={player.faceit_level} size={18} />}
      <span
        className={`w-10 text-right font-medium ${estimated ? 'text-amber-300 cursor-help' : 'text-zinc-200'}`}
        title={estimated ? ESTIMATED_TITLE : undefined}
      >
        {estimated && '~'}{player.skill_score != null ? player.skill_score.toFixed(1) : '–'}
      </span>
    </div>
  )
}

function DraggablePlayer({ player, teamId, linked, estimated }: {
  player: Player; teamId: string; linked: boolean; estimated: boolean
}) {
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: player.id,
    data: { player, teamId },
  })
  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      className={`cursor-grab active:cursor-grabbing ${isDragging ? 'opacity-30' : ''}`}
    >
      <PlayerRow player={player} linked={linked} estimated={estimated} />
    </div>
  )
}

function TeamColumn({ team, mean, teamSize, linkedIds, draft }: {
  team: Team; mean: number; teamSize: number; linkedIds: Set<string>; draft: Draft
}) {
  const { setNodeRef, isOver } = useDroppable({ id: team.id })
  const diff = team.avg_skill_score - mean
  const wrongSize = team.players.length !== teamSize

  return (
    <div
      ref={setNodeRef}
      className={`rounded-md border bg-zinc-900 ${isOver ? 'border-amber-500' : 'border-zinc-800'}`}
    >
      <div className="flex items-baseline justify-between gap-2 px-3 py-2 border-b border-zinc-800 tabular-nums">
        <h3 className="font-semibold text-zinc-100">{team.name}</h3>
        <div className="text-sm">
          <span className="text-zinc-500">avg </span>
          <span className="text-zinc-100 font-medium">{team.avg_skill_score.toFixed(1)}</span>
          <span
            className={`ml-1.5 text-xs ${Math.abs(diff) < 0.05 ? 'text-zinc-600' : diff > 0 ? 'text-emerald-400' : 'text-red-400'}`}
            title="Difference from the all-team average"
          >
            {diff >= 0 ? '+' : '−'}{Math.abs(diff).toFixed(1)}
          </span>
        </div>
      </div>
      <div className="p-2 flex flex-col gap-1 min-h-16">
        {team.players.map(p => (
          <DraggablePlayer
            key={p.id} player={p} teamId={team.id}
            linked={linkedIds.has(p.id)} estimated={!!draft.scores[p.id]?.estimated}
          />
        ))}
        {team.players.length === 0 && <p className="text-zinc-600 text-sm text-center py-3">Drop players here</p>}
      </div>
      {wrongSize && (
        <p className="px-3 pb-2 text-xs text-amber-400">
          {team.players.length} players, should be {teamSize}
        </p>
      )}
    </div>
  )
}

const teamsAsText = (teams: Team[]) =>
  teams.map(t =>
    `${t.name} (avg ${t.avg_skill_score.toFixed(1)})\n` + t.players.map(p => `- ${p.steam_name}`).join('\n'),
  ).join('\n\n')

/** The teams of one draft, with drag and drop between them. */
function DraftTeams({ draft }: { draft: Draft }) {
  const { tournament, movePlayer, renameDraft, deleteDraft } = useTournamentStore()
  const [active, setActive] = useState<Player | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))

  if (!tournament) return null
  const teams = draft.teams

  const drafted = new Map<string, string>()
  teams.forEach(t => t.players.forEach(p => drafted.set(p.id, t.id)))
  // Scored in this draft but not on a team: the bench. No score: left out.
  const bench = tournament.players.filter(p => draft.scores[p.id] && !drafted.has(p.id))
  const leftOut = tournament.players.filter(p => !draft.scores[p.id] && !drafted.has(p.id))
  const estimated = teams.flatMap(t => t.players).filter(p => draft.scores[p.id]?.estimated).length

  // Players currently sharing a team with someone they are linked to by a request
  const linkedIds = new Set<string>()
  tournament.teammate_requests.forEach(r => {
    const a = drafted.get(r.requester_id), b = drafted.get(r.requested_id)
    if (a && a === b) { linkedIds.add(r.requester_id); linkedIds.add(r.requested_id) }
  })

  const handleDragStart = (e: DragStartEvent) => setActive((e.active.data.current as { player: Player }).player)
  const handleDragEnd = (e: DragEndEvent) => {
    setActive(null)
    if (!e.over) return
    const { player, teamId } = e.active.data.current as { player: Player; teamId: string }
    if (teamId !== e.over.id) movePlayer(draft.id, player.id, teamId, e.over.id as string)
  }

  const rename = () => {
    const name = window.prompt('Draft name', draft.name)?.trim()
    if (name && name !== draft.name) renameDraft(draft.id, name)
  }
  const remove = () => {
    if (window.confirm(`Delete the draft "${draft.name}"? Its teams and manual moves are lost.`)) deleteDraft(draft.id)
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-baseline justify-between gap-x-4 gap-y-1 flex-wrap">
        <div className="flex items-baseline gap-x-4 gap-y-1 flex-wrap">
          <BalanceMetrics teams={teams} />
          <span className="text-xs text-zinc-500" title="The settings this draft was made with">
            {describeSettings(draft.settings).join(' · ')}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <span className="text-xs text-zinc-500 mr-1">Drag players to move them</span>
          <button onClick={rename} className="btn-quiet">Rename</button>
          <button onClick={remove} className="btn-quiet hover:!text-red-400">Delete</button>
          <CopyButton text={() => teamsAsText(teams)} className="btn ml-1" title="Copy teams as text">
            Copy teams
          </CopyButton>
        </div>
      </div>

      <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd} onDragCancel={() => setActive(null)}>
        <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(15rem,1fr))]">
          {teams.map(team => (
            <TeamColumn
              key={team.id} team={team} mean={teamMean(teams)}
              teamSize={tournament.team_size} linkedIds={linkedIds} draft={draft}
            />
          ))}
        </div>
        <DragOverlay>
          {active && (
            <div className="shadow-xl shadow-black/50">
              <PlayerRow player={active} linked={false} estimated={!!draft.scores[active.id]?.estimated} />
            </div>
          )}
        </DragOverlay>
      </DndContext>

      {(bench.length > 0 || leftOut.length > 0 || estimated > 0) && (
        <div className="panel px-3 py-2 text-sm flex flex-col gap-1">
          {bench.length > 0 && (
            <div>
              <span className="panel-title mr-3">Bench <span className="text-zinc-600">{bench.length}</span></span>
              {bench.map((p, i) => (
                <span key={p.id} className="text-zinc-400 tabular-nums">
                  {i > 0 && <span className="text-zinc-700"> · </span>}
                  {p.steam_name} <span className="text-zinc-600">{draft.scores[p.id].score.toFixed(1)}</span>
                </span>
              ))}
            </div>
          )}
          {leftOut.length > 0 && (
            <div title="This draft's settings could not score these players">
              <span className="panel-title mr-3">Not scored <span className="text-zinc-600">{leftOut.length}</span></span>
              <span className="text-zinc-400">{leftOut.map(p => p.steam_name).join(' · ')}</span>
            </div>
          )}
          {estimated > 0 && (
            <p className="text-xs text-amber-300/80">
              ~ {estimated === 1 ? '1 player' : `${estimated} players`} could not be rated with these settings
              and {estimated === 1 ? 'was' : 'were'} scored with the full model instead.
            </p>
          )}
        </div>
      )}
    </div>
  )
}

/** Every draft of the tournament: one tab each, and a tab that compares them. */
export default function DraftView() {
  const { tournament, setActiveDraft } = useTournamentStore()
  const draft = useTournamentStore(activeDraftOf)
  const [comparing, setComparing] = useState(false)

  if (!tournament || !draft) return null
  const drafts = tournament.drafts
  const showCompare = comparing && drafts.length > 1

  const tab = (selected: boolean) =>
    `px-3 py-1.5 text-sm rounded-t border border-b-0 whitespace-nowrap ${
      selected
        ? 'bg-zinc-900 border-zinc-700 text-zinc-50'
        : 'border-transparent text-zinc-400 hover:text-zinc-100 hover:bg-zinc-900/60'
    }`

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-end gap-3 border-b border-zinc-700">
        <h2 className="panel-title pb-2">Drafts <span className="text-zinc-600">{drafts.length}</span></h2>
        <div className="flex items-end gap-1 overflow-x-auto -mb-px" role="tablist">
          {drafts.map(d => {
            const selected = !showCompare && d.id === draft.id
            return (
              <button
                key={d.id}
                role="tab"
                aria-selected={selected}
                onClick={() => { setActiveDraft(d.id); setComparing(false) }}
                title={`${describeSettings(d.settings).join(' · ')}. Spread ${teamSpread(d.teams).toFixed(2)} on its own scores.`}
                className={tab(selected)}
              >
                {d.name}
                <span className="ml-2 text-xs text-zinc-500 tabular-nums">spread {teamSpread(d.teams).toFixed(1)}</span>
              </button>
            )
          })}
          {drafts.length > 1 && (
            <button
              role="tab"
              aria-selected={showCompare}
              onClick={() => setComparing(true)}
              className={`${tab(showCompare)} ${showCompare ? '' : '!text-amber-400'}`}
            >
              Compare all
            </button>
          )}
        </div>
      </div>

      {showCompare
        ? <DraftCompare onOpen={id => { setActiveDraft(id); setComparing(false) }} />
        : <DraftTeams key={draft.id} draft={draft} />}
    </div>
  )
}
