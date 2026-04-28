import { useDraggable } from '@dnd-kit/core'
import type { Player } from '../types'
import PlayerCard from './PlayerCard'

interface Props {
  player: Player
  teamId: string
}

export default function DraggablePlayer({ player, teamId }: Props) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: player.id,
    data: { player, teamId },
  })

  const style = transform
    ? { transform: `translate(${transform.x}px, ${transform.y}px)` }
    : undefined

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...listeners}
      {...attributes}
      className={`cursor-grab active:cursor-grabbing transition-opacity ${isDragging ? 'opacity-50' : ''}`}
    >
      <PlayerCard player={player} compact />
    </div>
  )
}
