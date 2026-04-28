import type { Player } from '../types'

interface Props {
    player: Player
    onRemove?: () => void
    compact?: boolean
}

function FaceitBadge({ level, elo }: { level?: number; elo?: number }) {
    if (!level && !elo) return null
    const colors: Record<number, string> = {
        1: 'bg-gray-600', 2: 'bg-gray-500', 3: 'bg-gray-400',
        4: 'bg-green-700', 5: 'bg-green-600', 6: 'bg-orange-600',
        7: 'bg-orange-500', 8: 'bg-red-600', 9: 'bg-red-500', 10: 'bg-red-400',
    }
    const color = level ? (colors[level] ?? 'bg-gray-600') : 'bg-gray-600'
    return (
        <span className={`inline-flex items-center gap-1 text-xs font-semibold px-1.5 py-0.5 rounded ${color} text-white`}>
            FL{level ?? '?'}
            {elo && <span className="font-normal opacity-80">{elo.toLocaleString()}</span>}
        </span>
    )
}

function PremierBadge({ rating }: { rating?: number }) {
    if (!rating) return null
    return (
        <span className="inline-flex items-center gap-1 text-xs font-semibold px-1.5 py-0.5 rounded bg-blue-700 text-white">
            {rating.toLocaleString()}
            <span className="text-yellow-300">★</span>
        </span>
    )
}

function ScoreBar({ score }: { score: number }) {
    const color =
        score >= 70 ? 'bg-green-500' :
            score >= 45 ? 'bg-yellow-500' :
                'bg-red-500'
    return (
        <div className="flex items-center gap-2">
            <div className="flex-1 bg-gray-700 rounded-full h-1.5">
                <div className={`${color} h-1.5 rounded-full`} style={{ width: `${Math.min(score, 100)}%` }} />
            </div>
            <span className="text-sm font-bold text-white w-10 text-right">{score.toFixed(1)}</span>
        </div>
    )
}

export default function PlayerCard({ player, onRemove, compact = false }: Props) {
    const hasScore = player.skill_score != null && player.skill_score >= 0
    const isSteamOnly = player.privacy_mode === 'steam_only'
    // "private" means Leetify returned the profile but flagged it as private (score = -1)
    // "unknown" means the Leetify fetch itself failed — don't show "Private" in that case
    const isPrivate = !isSteamOnly && (player.privacy_mode === 'private' || (player.skill_score != null && player.skill_score < 0))
    const noData = !hasScore && !isPrivate && !isSteamOnly  // fetch failed or player not on Leetify

    return (
        <div className={`flex items-start gap-3 p-3 rounded-lg bg-gray-800 border border-gray-700 hover:border-gray-600 transition-colors ${compact ? 'py-2' : ''}`}>
            <img
                src={player.avatar_url || 'https://steamcommunity-a.akamaihd.net/public/images/avatars/fe/fef49e7fa7e1997310d705b2a6158ff8dc1cdfeb_full.jpg'}
                alt={player.steam_name}
                className="w-10 h-10 rounded flex-shrink-0"
            />

            <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2 flex-wrap">
                    <span className="font-semibold text-sm truncate">{player.steam_name}</span>
                    {player.is_banned && (
                        <span className="text-xs font-bold px-1.5 py-0.5 rounded bg-red-900 text-red-300 border border-red-700">
                            BANNED{player.ban_info?.length ? ` (${player.ban_info.map(b => b.platform).join(', ')})` : ''}
                        </span>
                    )}
                    {isPrivate && (
                        <span className="text-xs px-1.5 py-0.5 rounded bg-yellow-900 text-yellow-400">
                            Private
                        </span>
                    )}
                    {isSteamOnly && (
                        <span className="text-xs px-1.5 py-0.5 rounded bg-blue-900 text-blue-300">
                            Steam stats
                        </span>
                    )}
                    {noData && (
                        <span className="text-xs px-1.5 py-0.5 rounded bg-gray-700 text-gray-400">
                            No Leetify data
                        </span>
                    )}
                </div>

                {!compact && (
                    <div className="flex items-center gap-2 mt-1 flex-wrap">
                        <FaceitBadge level={player.faceit_level} elo={player.faceit_elo} />
                        <PremierBadge rating={player.premier_rating} />
                    </div>
                )}

                {hasScore ? (
                    <div className="mt-1.5">
                        <ScoreBar score={player.skill_score!} />
                        {!compact && player.rating && (
                            <div className="flex gap-3 mt-1 text-xs text-gray-400">
                                <span>Aim <span className="text-gray-200">{player.rating.aim.toFixed(0)}</span></span>
                                <span>Pos <span className="text-gray-200">{player.rating.positioning.toFixed(0)}</span></span>
                                <span>Util <span className="text-gray-200">{player.rating.utility.toFixed(0)}</span></span>
                                <span>Clutch <span className="text-gray-200">{player.rating.clutch.toFixed(0)}</span></span>
                            </div>
                        )}
                    </div>
                ) : (
                    <p className="text-xs text-gray-500 mt-1">
                        {isPrivate ? 'Stats unavailable — private Leetify profile' : 'No stats available'}
                    </p>
                )}
            </div>

            {onRemove && (
                <button
                    onClick={onRemove}
                    className="text-gray-600 hover:text-red-400 transition-colors text-lg leading-none flex-shrink-0"
                    title="Remove player"
                >
                    ×
                </button>
            )}
        </div>
    )
}
