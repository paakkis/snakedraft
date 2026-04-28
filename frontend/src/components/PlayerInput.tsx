import { useState, type FormEvent } from 'react'
import { useTournamentStore } from '../store/tournament'

export default function PlayerInput() {
  const [url, setUrl] = useState('')
  const { addingPlayer, error, addPlayer, clearError } = useTournamentStore()

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!url.trim()) return
    clearError()
    await addPlayer(url.trim())
    setUrl('')
  }

  return (
    <div>
      <form onSubmit={handleSubmit} className="flex gap-2">
        <input
          type="text"
          value={url}
          onChange={e => setUrl(e.target.value)}
          placeholder="steamcommunity.com/id/username  or  /profiles/76561..."
          className="flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm text-gray-100 placeholder-gray-500 focus:outline-none focus:border-blue-500 transition-colors"
          disabled={addingPlayer}
        />
        <button
          type="submit"
          disabled={addingPlayer || !url.trim()}
          className="px-4 py-2 bg-blue-600 hover:bg-blue-500 disabled:bg-gray-700 disabled:text-gray-500 text-white font-semibold rounded-lg text-sm transition-colors flex items-center gap-2"
        >
          {addingPlayer ? (
            <>
              <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
              </svg>
              Fetching...
            </>
          ) : (
            '+ Add'
          )}
        </button>
      </form>

      {error && (
        <p className="mt-2 text-sm text-red-400 bg-red-950 border border-red-800 rounded px-3 py-2">
          {error}
        </p>
      )}
    </div>
  )
}
