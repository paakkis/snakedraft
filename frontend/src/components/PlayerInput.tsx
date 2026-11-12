import { useState, type FormEvent } from 'react'
import { useTournamentStore } from '../store/tournament'

/** Accepts one or many Steam profile URLs / Steam64 IDs, separated by spaces, commas or new lines. */
export default function PlayerInput() {
  const [text, setText] = useState('')
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [failed, setFailed] = useState<{ input: string; reason: string }[]>([])
  const { addPlayer, clearError } = useTournamentStore()

  const inputs = text.split(/[\s,]+/).filter(Boolean)
  const busy = progress !== null

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (inputs.length === 0 || busy) return
    const failures: { input: string; reason: string }[] = []
    setFailed([])
    for (let i = 0; i < inputs.length; i++) {
      setProgress({ done: i, total: inputs.length })
      const ok = await addPlayer(inputs[i])
      if (!ok) {
        failures.push({ input: inputs[i], reason: useTournamentStore.getState().error ?? 'failed' })
        clearError()
      }
    }
    setProgress(null)
    setFailed(failures)
    // Keep what failed in the box so it can be corrected and retried
    setText(failures.map(f => f.input).join('\n'))
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-2">
      <textarea
        value={text}
        onChange={e => setText(e.target.value)}
        onKeyDown={e => {
          if (e.key === 'Enter' && !e.shiftKey && inputs.length <= 1) handleSubmit(e)
        }}
        rows={Math.min(Math.max(text.split('\n').length, 1), 8)}
        placeholder="Steam profile URLs or Steam64 IDs — paste several at once, one per line"
        className="field w-full font-mono text-xs leading-5 resize-y"
        disabled={busy}
      />
      <div className="flex items-center gap-3">
        <button type="submit" disabled={busy || inputs.length === 0} className="btn-primary">
          {busy
            ? `Fetching ${progress.done + 1} / ${progress.total}…`
            : inputs.length > 1 ? `Add ${inputs.length} players` : 'Add player'}
        </button>
        <span className="text-xs text-zinc-500">
          steamcommunity.com/id/name · steamcommunity.com/profiles/7656… · 7656…
        </span>
      </div>
      {failed.length > 0 && (
        <ul className="notice-error space-y-0.5">
          {failed.map(f => (
            <li key={f.input}>
              <span className="font-mono text-xs">{f.input}</span> — {f.reason}
            </li>
          ))}
        </ul>
      )}
    </form>
  )
}
