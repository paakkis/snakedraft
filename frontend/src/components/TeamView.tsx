import { useState } from 'react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  useDroppable,
  type DragStartEvent,
  type DragEndEvent,
} from '@dnd-kit/core'
import type { Player, Team } from '../types'
import { useTournamentStore } from '../store/tournament'
import DraggablePlayer from './DraggablePlayer'
import PlayerCard from './PlayerCard'

function DroppableTeam({ team }: { team: Team }) {
  const { setNodeRef, isOver } = useDroppable({ id: team.id })

  return (
    <div
      ref={setNodeRef}
      className={`flex-1 min-w-64 rounded-xl border transition-colors ${
        isOver ? 'border-indigo-500 bg-indigo-950/30' : 'border-gray-700 bg-gray-900'
      }`}
    >
      <div className="px-4 py-3 border-b border-gray-700">
        <div className="flex items-center justify-between">
          <h3 className="font-bold text-white">{team.name}</h3>
          <span className="text-sm text-gray-400">avg {team.avg_skill_score.toFixed(1)}</span>
        </div>
      </div>
      <div className="p-3 flex flex-col gap-2">
        {team.players.map(player => (
          <DraggablePlayer key={player.id} player={player} teamId={team.id} />
        ))}
        {team.players.length === 0 && (
          <p className="text-gray-600 text-sm text-center py-4">Drop players here</p>
        )}
      </div>
    </div>
  )
}

export default function TeamView() {
  const { tournament, benchPlayerIds, movePlayer } = useTournamentStore()
  const [activePlayer, setActivePlayer] = useState<{ player: Player; teamId: string } | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
  )

  if (!tournament || tournament.teams.length === 0) return null

  const benchPlayers = tournament.players.filter(p => benchPlayerIds.includes(p.id))

  const handleDragStart = (event: DragStartEvent) => {
    const { player, teamId } = event.active.data.current as { player: Player; teamId: string }
    setActivePlayer({ player, teamId })
  }

  const handleDragEnd = (event: DragEndEvent) => {
    setActivePlayer(null)
    const { over, active } = event
    if (!over) return

    const { player, teamId: fromTeamId } = active.data.current as { player: Player; teamId: string }
    const toTeamId = over.id as string

    if (fromTeamId === toTeamId) return
    movePlayer(player.id, fromTeamId, toTeamId)
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider">
          Teams ({tournament.teams.length})
        </h2>
        <span className="text-xs text-gray-600">drag players between teams to adjust</span>
      </div>

      <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
        <div className="flex gap-4 overflow-x-auto pb-2">
          {tournament.teams.map(team => (
            <DroppableTeam key={team.id} team={team} />
          ))}
        </div>

        <DragOverlay>
          {activePlayer && (
            <div className="opacity-90 shadow-2xl rotate-1">
              <PlayerCard player={activePlayer.player} compact />
            </div>
          )}
        </DragOverlay>
      </DndContext>

      {benchPlayers.length > 0 && (
        <div className="mt-4 p-4 rounded-xl border border-dashed border-gray-700 bg-gray-900/50">
          <h3 className="text-sm font-semibold text-gray-500 mb-2">
            Bench ({benchPlayers.length}) — not enough for a full team
          </h3>
          <div className="flex flex-wrap gap-2">
            {benchPlayers.map(p => (
              <span key={p.id} className="text-xs px-2 py-1 rounded bg-gray-800 text-gray-400">
                {p.steam_name} {p.skill_score !== undefined ? `(${p.skill_score?.toFixed(1)})` : ''}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
