import { useTournamentStore } from '../store/tournament'
import PlayerCard from './PlayerCard'

export default function PlayerList() {
  const { tournament, removePlayer } = useTournamentStore()
  if (!tournament) return null

  const players = tournament.players

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider">
          Players ({players.length})
        </h2>
        <span className="text-xs text-gray-600">sorted by skill score</span>
      </div>

      {players.length === 0 ? (
        <p className="text-gray-500 text-sm py-8 text-center border border-dashed border-gray-800 rounded-lg">
          No players yet. Add players using the form above.
        </p>
      ) : (
        <div className="flex flex-col gap-2">
          {players.map(player => (
            <PlayerCard
              key={player.id}
              player={player}
              onRemove={() => removePlayer(player.id)}
            />
          ))}
        </div>
      )}
    </div>
  )
}
