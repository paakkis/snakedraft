import type { Draft, DraftSettings, ScoreSettings } from '../types'

/** The full model. Must match services.DefaultDraftSettings on the backend. */
export const DEFAULT_SETTINGS: DraftSettings = {
  use_faceit: true,
  use_premier: true,
  use_steam: true,
  weight_rank: 0.55,
  weight_aim: 0.15,
  weight_positioning: 0.10,
  weight_utility: 0.04,
  weight_opening: 0.08,
  weight_form: 0.08,
  scale_by_confidence: true,
  adjust_for_performance: true,
  adjust_for_mechanics: true,
  max_spread: 2,
  honor_requests: true,
  fallback_to_full_model: true,
}

/** The listed rank and nothing else. */
const RANK_ONLY: ScoreSettings = {
  use_faceit: true,
  use_premier: true,
  use_steam: false,
  weight_rank: 1,
  weight_aim: 0,
  weight_positioning: 0,
  weight_utility: 0,
  weight_opening: 0,
  weight_form: 0,
  scale_by_confidence: false,
  adjust_for_performance: false,
  adjust_for_mechanics: false,
}

export interface Preset {
  id: string
  label: string
  hint: string
  score: ScoreSettings
}

const scoreOf = (s: DraftSettings): ScoreSettings => {
  const { max_spread: _m, honor_requests: _h, fallback_to_full_model: _f, ...score } = s
  return score
}

export const PRESETS: Preset[] = [
  {
    id: 'full',
    label: 'Full model',
    hint: 'Rank, Leetify ratings, recent form and the rank corrections. What the player list shows.',
    score: scoreOf(DEFAULT_SETTINGS),
  },
  {
    id: 'ranks',
    label: 'Faceit + Premier',
    hint: 'The better of Faceit ELO and Premier rating, as listed. Nothing else.',
    score: RANK_ONLY,
  },
  {
    id: 'faceit',
    label: 'Faceit only',
    hint: 'Faceit ELO as listed. Nothing else.',
    score: { ...RANK_ONLY, use_premier: false },
  },
  {
    id: 'premier',
    label: 'Premier only',
    hint: 'Premier rating as listed. Nothing else.',
    score: { ...RANK_ONLY, use_faceit: false },
  },
]

export const WEIGHTS: { key: keyof ScoreSettings & `weight_${string}`; label: string; hint: string }[] = [
  { key: 'weight_rank', label: 'Rank', hint: 'The better of the ranks switched on above' },
  { key: 'weight_aim', label: 'Aim', hint: 'Leetify aim rating' },
  { key: 'weight_positioning', label: 'Positioning', hint: 'Leetify positioning rating' },
  { key: 'weight_utility', label: 'Utility', hint: 'Leetify utility rating' },
  { key: 'weight_opening', label: 'Opening duels', hint: 'Opening duel win %, CT and T averaged' },
  { key: 'weight_form', label: 'Recent form', hint: 'Leetify rating over the last 10 matches, or recent Faceit K/D and ADR for players without Leetify' },
]

const weightSum = (s: ScoreSettings) => WEIGHTS.reduce((sum, w) => sum + s[w.key], 0)

/** Share of the score a weight gets when every component is present, 0–100. */
export function weightShare(s: ScoreSettings, key: (typeof WEIGHTS)[number]['key']): number {
  const sum = weightSum(s)
  return sum > 0 ? (s[key] / sum) * 100 : 0
}

const near = (a: number, b: number) => Math.abs(a - b) < 1e-9

/** Same scores for every player? Weights are compared by ratio. */
function sameScore(a: ScoreSettings, b: ScoreSettings): boolean {
  if (
    a.use_faceit !== b.use_faceit || a.use_premier !== b.use_premier || a.use_steam !== b.use_steam
    || a.scale_by_confidence !== b.scale_by_confidence
    || a.adjust_for_performance !== b.adjust_for_performance
    || a.adjust_for_mechanics !== b.adjust_for_mechanics
  ) return false
  const sa = weightSum(a), sb = weightSum(b)
  if (sa === 0 || sb === 0) return sa === sb
  return WEIGHTS.every(w => near(a[w.key] / sa, b[w.key] / sb))
}

/** The preset these settings score like, if any. */
export const matchPreset = (s: ScoreSettings): Preset | undefined =>
  PRESETS.find(p => sameScore(p.score, s))

/** Do these settings give every player the score the player list is loaded with? */
export const isFullModel = (s: ScoreSettings): boolean => matchPreset(s)?.id === 'full'

/** Can these settings score anyone at all? */
export function canScore(s: ScoreSettings): boolean {
  const ranked = (s.use_faceit || s.use_premier) && s.weight_rank > 0
  const leetify = s.weight_aim + s.weight_positioning + s.weight_utility + s.weight_opening + s.weight_form > 0
  return ranked || leetify || s.use_steam
}

/** Short labels saying how a draft was made, most important first. */
export function describeSettings(s: DraftSettings): string[] {
  const out: string[] = []
  const preset = matchPreset(s)
  if (preset) {
    out.push(preset.label)
  } else {
    out.push(
      s.weight_rank > 0
        ? s.use_faceit && s.use_premier ? 'Faceit + Premier' : s.use_faceit ? 'Faceit' : s.use_premier ? 'Premier' : 'No rank'
        : 'No rank',
    )
    const parts = WEIGHTS
      .filter(w => s[w.key] > 0 && (w.key !== 'weight_rank' || s.use_faceit || s.use_premier))
      .map(w => `${w.label.toLowerCase()} ${Math.round(weightShare(s, w.key))}%`)
    if (parts.length > 0) out.push(parts.join(', '))
    if (!s.use_steam) out.push('no Steam stats')
    const off = [
      !s.scale_by_confidence && 'confidence',
      !s.adjust_for_performance && 'lobby performance',
      !s.adjust_for_mechanics && 'mechanics',
    ].filter(Boolean)
    if (off.length === 3) out.push('ranks as listed')
    else if (off.length > 0) out.push(`no ${off.join(' or ')} correction`)
  }
  if (!near(s.max_spread, DEFAULT_SETTINGS.max_spread)) out.push(`max spread ${s.max_spread}`)
  if (!s.honor_requests) out.push('requests ignored')
  if (!s.fallback_to_full_model) out.push('unrated players left out')
  return out
}

/** A name for a new draft that no existing draft has yet. */
export function freeDraftName(base: string, drafts: Draft[]): string {
  const taken = new Set(drafts.map(d => d.name))
  if (!taken.has(base)) return base
  for (let n = 2; ; n++) {
    if (!taken.has(`${base} ${n}`)) return `${base} ${n}`
  }
}
