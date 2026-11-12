import { useRef, useState, type ChangeEvent, type FormEvent } from 'react'
import { useTournamentStore } from '../store/tournament'
import { readPlayerFile } from '../lib/importPlayers'

/** Adds players from pasted Steam URLs / Steam64 IDs or from a CSV / Excel file. */
export default function PlayerInput() {
  const [text, setText] = useState('')
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null)
  const [failed, setFailed] = useState<{ input: string; reason: string }[]>([])
  const [importNote, setImportNote] = useState<{ text: string; error?: boolean } | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const { tournament, addPlayer, clearError } = useTournamentStore()

  const inputs = text.split(/[\s,]+/).filter(Boolean)
  const busy = progress !== null

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (inputs.length === 0 || busy) return
    const failures: { input: string; reason: string }[] = []
    setFailed([])
    setImportNote(null)
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

  const handleFile = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = '' // allow picking the same file again
    if (!file) return
    setFailed([])
    try {
      const { inputs: found, warnings } = await readPlayerFile(file)

      // Skip players who are already in the tournament
      const ids = new Set(tournament?.players.map(p => p.steam64_id))
      const vanities = new Set(tournament?.players.map(p => p.vanity_url?.toLowerCase()).filter(Boolean))
      const fresh = found.filter(input => {
        const vanity = input.match(/\/id\/(.+)$/)?.[1].toLowerCase()
        return vanity ? !vanities.has(vanity) : !ids.has(input)
      })
      const skipped = found.length - fresh.length

      if (found.length === 0) {
        setImportNote({ text: `No Steam profiles found in ${file.name}.`, error: true })
        return
      }
      setText(prev => [...prev.split(/[\s,]+/).filter(Boolean), ...fresh].join('\n'))
      setImportNote({
        text: [
          `${file.name}: ${fresh.length} new player${fresh.length === 1 ? '' : 's'}`
            + (skipped ? `, ${skipped} already added` : '') + '.',
          ...warnings,
        ].filter(Boolean).join(' '),
        error: warnings.length > 0,
      })
    } catch (err) {
      setImportNote({ text: (err as Error).message, error: true })
    }
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
        placeholder="Steam profile URLs or Steam64 IDs, one per line"
        className="field w-full font-mono text-xs leading-5 resize-y"
        disabled={busy}
      />
      <div className="flex items-center gap-2 flex-wrap">
        <button type="submit" disabled={busy || inputs.length === 0} className="btn-primary">
          {busy
            ? `Fetching ${progress.done + 1} / ${progress.total}`
            : inputs.length > 1 ? `Add ${inputs.length} players` : 'Add player'}
        </button>
        <button type="button" onClick={() => fileInput.current?.click()} disabled={busy} className="btn">
          Import CSV / Excel
        </button>
        <input
          ref={fileInput}
          type="file"
          accept=".csv,.txt,.tsv,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
          onChange={handleFile}
          className="hidden"
        />
        {importNote && (
          <span className={`text-xs ${importNote.error ? 'text-amber-400' : 'text-zinc-400'}`}>{importNote.text}</span>
        )}
      </div>
      {failed.length > 0 && (
        <ul className="notice-error space-y-0.5">
          {failed.map(f => (
            <li key={f.input}>
              <span className="font-mono text-xs">{f.input}</span>: {f.reason}
            </li>
          ))}
        </ul>
      )}
    </form>
  )
}
