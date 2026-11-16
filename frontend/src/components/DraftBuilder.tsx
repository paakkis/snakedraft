import { useState } from 'react'
import type { DraftSettings } from '../types'
import { useTournamentStore } from '../store/tournament'
import {
  DEFAULT_SETTINGS, PRESETS, WEIGHTS,
  canScore, describeSettings, freeDraftName, matchPreset, weightShare,
} from '../lib/draftSettings'

function Check({ checked, onChange, label, hint, disabled }: {
  checked: boolean; onChange: (v: boolean) => void; label: string; hint: string; disabled?: boolean
}) {
  return (
    <label className={`flex items-start gap-2 ${disabled ? 'opacity-40' : 'cursor-pointer'}`} title={hint}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={e => onChange(e.target.checked)}
        className="mt-0.5 accent-amber-500"
      />
      <span>
        <span className="text-zinc-200">{label}</span>
        <span className="block text-xs text-zinc-500">{hint}</span>
      </span>
    </label>
  )
}

function Group({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <fieldset className="min-w-0">
      <legend className="panel-title mb-2">{title}</legend>
      <div className="flex flex-col gap-2 text-sm">{children}</div>
    </fieldset>
  )
}

// Weights are shown as whole points (55 for 0.55). Only their ratios matter.
const toPoints = (w: number) => +(w * 100).toFixed(2)
const fromPoints = (v: string) => Math.min(Math.max(Number(v) || 0, 0), 1000) / 100

/** Settings for a new draft. Every draft made here is kept next to the others. */
export default function DraftBuilder() {
  const { tournament, createDraft, loading, draftError } = useTournamentStore()
  const [settings, setSettings] = useState<DraftSettings>(DEFAULT_SETTINGS)
  const [name, setName] = useState('')
  const [open, setOpen] = useState(false)

  if (!tournament) return null

  const set = <K extends keyof DraftSettings>(key: K, value: DraftSettings[K]) =>
    setSettings(s => ({ ...s, [key]: value }))

  const preset = matchPreset(settings)
  const autoName = freeDraftName(preset?.label ?? 'Custom', tournament.drafts)
  const ranked = settings.use_faceit || settings.use_premier
  const usable = canScore(settings)

  // An estimate from the player list. The server has the final say, since
  // custom settings can score fewer players than the full model does.
  const scored = tournament.players.filter(p => p.skill_score != null && p.skill_score >= 0).length
  const size = tournament.team_size
  const teamsPossible = Math.floor(scored / size)
  const benched = scored - teamsPossible * size

  const create = async () => {
    if (await createDraft(name.trim() || autoName, settings)) setName('')
  }

  return (
    <div className="panel">
      <div className="px-3 py-2.5 flex items-center justify-between gap-x-4 gap-y-2 flex-wrap">
        <div className="flex items-center gap-3 flex-wrap">
          <h2 className="panel-title">New draft</h2>
          <div className="flex rounded border border-zinc-700 overflow-hidden text-xs" role="group" aria-label="Preset">
            {PRESETS.map(p => (
              <button
                key={p.id}
                type="button"
                title={p.hint}
                aria-pressed={preset?.id === p.id}
                onClick={() => setSettings(s => ({ ...s, ...p.score }))}
                className={`px-2.5 py-1 border-r border-zinc-700 last:border-r-0 ${
                  preset?.id === p.id ? 'bg-zinc-700 text-zinc-50' : 'text-zinc-400 hover:text-zinc-100'
                }`}
              >
                {p.label}
              </button>
            ))}
            {!preset && <span className="px-2.5 py-1 bg-amber-500/20 text-amber-300">Custom</span>}
          </div>
          <button type="button" onClick={() => setOpen(o => !o)} className="btn-quiet" aria-expanded={open}>
            {open ? 'Hide settings ▴' : 'All settings ▾'}
          </button>
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          <span className="text-sm text-zinc-400">
            {teamsPossible >= 2 ? (
              <>
                <span className="text-zinc-100 font-medium">{teamsPossible} teams</span> of {size}
                {benched > 0 && <span className="text-amber-400"> · {benched} benched</span>}
              </>
            ) : (
              <>Need {size * 2} scored players, have {scored}</>
            )}
          </span>
          <input
            type="text"
            value={name}
            onChange={e => setName(e.target.value)}
            placeholder={autoName}
            maxLength={60}
            aria-label="Draft name"
            className="field w-40"
          />
          <button
            onClick={create}
            disabled={loading || teamsPossible < 2 || !usable}
            title={usable ? undefined : 'These settings leave nothing to score players on'}
            className="btn-primary"
          >
            {loading ? 'Drafting…' : 'Create draft'}
          </button>
        </div>
      </div>

      <p className="px-3 pb-2.5 -mt-1 text-xs text-zinc-500">
        {describeSettings(settings).join(' · ')}
        {tournament.drafts.length > 0 && ' · existing drafts are kept, so you can compare'}
      </p>

      {draftError && <p className="notice-error mx-3 mb-3">{draftError}</p>}

      {open && (
        <div className="border-t border-zinc-800 p-3 grid gap-x-8 gap-y-5 sm:grid-cols-2 xl:grid-cols-4">
          <Group title="Sources">
            <Check
              checked={settings.use_faceit} onChange={v => set('use_faceit', v)}
              label="Faceit ELO" hint="With both ranks on, the better one counts"
            />
            <Check
              checked={settings.use_premier} onChange={v => set('use_premier', v)}
              label="Premier rating" hint="With both ranks on, the better one counts"
            />
            <Check
              checked={settings.use_steam} onChange={v => set('use_steam', v)}
              label="Steam lifetime stats" hint="5–10% of the score, and all of it for players with nothing else"
            />
          </Group>

          <Group title="Weights">
            {WEIGHTS.map(w => {
              const unused = w.key === 'weight_rank' && !ranked
              return (
                <label key={w.key} className={`flex items-center gap-2 ${unused ? 'opacity-40' : ''}`} title={w.hint}>
                  <span className="flex-1 text-zinc-200">{w.label}</span>
                  <input
                    type="number" min={0} max={1000} step={1}
                    value={toPoints(settings[w.key])}
                    onChange={e => set(w.key, fromPoints(e.target.value))}
                    className="field w-16 text-right tabular-nums !py-1"
                  />
                  <span className="w-10 text-right text-xs text-zinc-500 tabular-nums">
                    {Math.round(weightShare(settings, w.key))}%
                  </span>
                </label>
              )
            })}
            <p className="text-xs text-zinc-500">
              Only the ratios matter. A component a player has no data for is left out for them.
            </p>
          </Group>

          <Group title="Rank corrections">
            <Check
              checked={settings.scale_by_confidence} onChange={v => set('scale_by_confidence', v)}
              disabled={!ranked}
              label="Trust thin or old ranks less"
              hint="A rank with few matches behind it, or not played for in a long time, weighs less"
            />
            <Check
              checked={settings.adjust_for_performance} onChange={v => set('adjust_for_performance', v)}
              disabled={!settings.use_faceit}
              label="Faceit performance against the lobby"
              hint="Moves the ELO towards the one the player performed like in recent matches"
            />
            <Check
              checked={settings.adjust_for_mechanics} onChange={v => set('adjust_for_mechanics', v)}
              disabled={!ranked}
              label="Mechanics above the rank"
              hint="Raises a rank whose player aims and positions better than is typical for it"
            />
          </Group>

          <Group title="Teams">
            <label className="flex items-center gap-2" title="Largest gap between the strongest and weakest team average that still counts as fair when keeping requested teammates together">
              <span className="flex-1 text-zinc-200">Max spread for requests</span>
              <input
                type="number" min={0.1} max={100} step={0.5}
                value={settings.max_spread}
                onChange={e => set('max_spread', Math.min(Math.max(Number(e.target.value) || 0, 0), 100))}
                disabled={!settings.honor_requests}
                className="field w-16 text-right tabular-nums !py-1 disabled:opacity-40"
              />
            </label>
            <Check
              checked={settings.honor_requests} onChange={v => set('honor_requests', v)}
              label="Keep requested teammates together"
              hint="As long as the spread stays within the limit"
            />
            <Check
              checked={settings.fallback_to_full_model} onChange={v => set('fallback_to_full_model', v)}
              label="Draft players these settings cannot rate"
              hint="They are scored with the full model and marked ~. Otherwise they are left out"
            />
            <button type="button" onClick={() => setSettings(DEFAULT_SETTINGS)} className="btn self-start mt-1">
              Reset to full model
            </button>
          </Group>
        </div>
      )}
    </div>
  )
}
