// Level colours follow Faceit's own scheme: 1 grey, 2–3 green, 4–7 yellow, 8–9 orange, 10 red.
const colorOf = (level: number) =>
  level >= 10 ? '#fe1f00' : level >= 8 ? '#ff6309' : level >= 4 ? '#ffc800' : level >= 2 ? '#1ce400' : '#d4d4d8'

/** Faceit skill level badge: a ring that fills up with the level, around the number. */
export default function FaceitLevel({ level, size = 20 }: { level: number; size?: number }) {
  const lvl = Math.min(Math.max(Math.round(level), 1), 10)
  const color = colorOf(lvl)
  // The ring is a 270° gauge, open at the bottom
  const r = 9.5
  const gauge = 2 * Math.PI * r * 0.75
  return (
    <svg
      width={size} height={size} viewBox="0 0 24 24"
      role="img" aria-label={`Faceit level ${lvl}`}
      className="inline-block flex-shrink-0 align-middle"
    >
      <title>{`Faceit level ${lvl}`}</title>
      <circle cx="12" cy="12" r="12" fill="#1f1f22" />
      <g transform="rotate(135 12 12)" fill="none" strokeWidth="2.2" strokeLinecap="round">
        <circle cx="12" cy="12" r={r} stroke="#3f3f46" strokeDasharray={`${gauge} 100`} />
        <circle cx="12" cy="12" r={r} stroke={color} strokeDasharray={`${gauge * (lvl / 10)} 100`} />
      </g>
      <text
        x="12" y="12.5" textAnchor="middle" dominantBaseline="middle"
        fontSize={lvl >= 10 ? 9.5 : 11} fontWeight="700" fill={color}
        fontFamily="ui-sans-serif, system-ui, sans-serif"
      >
        {lvl}
      </text>
    </svg>
  )
}
