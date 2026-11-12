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
import type { Player, Team } from '../types'
import { useTournamentStore } from '../store/tournament'
import { teamMean } from '../lib/stats'
import { BalanceMetrics } from './TeamBuilder'
import CopyButton from './CopyButton'
import FaceitLevel from './FaceitLevel'

function PlayerRow({ player, linked }: { player: Player; linked: boolean }) {
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded bg-zinc-800/60 border border-zinc-800 text-sm tabular-nums">
      {player.avatar_url
        ? <img src={player.avatar_url} alt="" className="w-5 h-5 rounded-sm flex-shrink-0" />
        : <div className="w-5 h-5 rounded-sm bg-zinc-700 flex-shrink-0" />}
      <span className="flex-1 min-w-0 truncate text-zinc-100">{player.steam_name}</span>
      {linked && (
        <span className="text-[10px] text-emerald-400" title="On this team with a requested teammate">
          friend
        </span>
      )}
      {player.is_banned && <span className="text-[10px] font-bold px-1 rounded-sm bg-red-600 text-white">BAN</span>}
      {player.faceit_level != null && player.faceit_level > 0 && <FaceitLevel level={player.faceit_level} size={18} />}
      <span className="w-9 text-right font-medium text-zinc-200">
        {player.skill_score != null ? player.skill_score.toFixed(1) : '–'}
      </span>
    </div>
  )
}

function DraggablePlayer({ player, teamId, linked }: { player: Player; teamId: string; linked: boolean }) {
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
      <PlayerRow player={player} linked={linked} />
    </div>
  )
}

function TeamColumn({ team, mean, teamSize, linkedIds }: {
  team: Team; mean: number; teamSize: number; linkedIds: Set<string>
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
            title="Difference from the average of all teams"
          >
            {diff >= 0 ? '+' : '−'}{Math.abs(diff).toFixed(1)}
          </span>
        </div>
      </div>
      <div className="p-2 flex flex-col gap-1 min-h-16">
        {team.players.map(p => (
          <DraggablePlayer key={p.id} player={p} teamId={team.id} linked={linkedIds.has(p.id)} />
        ))}
        {team.players.length === 0 && <p className="text-zinc-600 text-sm text-center py-3">Drop players here</p>}
      </div>
      {wrongSize && (
        <p className="px-3 pb-2 text-xs text-amber-400">
          {team.players.length} players — should be {teamSize}
        </p>
      )}
    </div>
  )
}

const teamsAsText = (teams: Team[]) =>
  teams.map(t =>
    `${t.name} (avg ${t.avg_skill_score.toFixed(1)})\n` + t.players.map(p => `- ${p.steam_name}`).join('\n'),
  ).join('\n\n')

export default function TeamView() {
  const { tournament, movePlayer } = useTournamentStore()
  const [active, setActive] = useState<Player | null>(null)
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))

  if (!tournament || tournament.teams.length === 0) return null
  const teams = tournament.teams

  // Anyone with a score who is not on a team is on the bench
  const drafted = new Map<string, string>()
  teams.forEach(t => t.players.forEach(p => drafted.set(p.id, t.id)))
  const bench = tournament.players.filter(p => p.skill_score != null && p.skill_score >= 0 && !drafted.has(p.id))

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
    if (teamId !== e.over.id) movePlayer(player.id, teamId, e.over.id as string)
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-baseline justify-between gap-4 flex-wrap">
        <div className="flex items-baseline gap-4 flex-wrap">
          <h2 className="panel-title">Teams <span className="text-zinc-600">{teams.length}</span></h2>
          <BalanceMetrics />
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-zinc-500">Drag players between teams — balance updates live</span>
          <CopyButton text={() => teamsAsText(teams)} className="btn" title="Copy the teams as plain text, e.g. for Discord">
            Copy teams
          </CopyButton>
        </div>
      </div>

      <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd} onDragCancel={() => setActive(null)}>
        <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(15rem,1fr))]">
          {teams.map(team => (
            <TeamColumn
              key={team.id} team={team} mean={teamMean(teams)}
              teamSize={tournament.team_size} linkedIds={linkedIds}
            />
          ))}
        </div>
        <DragOverlay>
          {active && <div className="shadow-xl shadow-black/50"><PlayerRow player={active} linked={false} /></div>}
        </DragOverlay>
      </DndContext>

      {bench.length > 0 && (
        <div className="panel px-3 py-2 text-sm">
          <span className="panel-title mr-3">Bench <span className="text-zinc-600">{bench.length}</span></span>
          {bench.map((p, i) => (
            <span key={p.id} className="text-zinc-400 tabular-nums">
              {i > 0 && <span className="text-zinc-700"> · </span>}
              {p.steam_name} <span className="text-zinc-600">{p.skill_score?.toFixed(1)}</span>
            </span>
          ))}
        </div>
      )}
    </div>
  )
}
