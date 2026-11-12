import { useState, type ReactNode } from 'react'
import { copyText } from '../lib/clipboard'

interface Props {
  text: string | (() => string)
  children: ReactNode
  title?: string
  className?: string
}

/** A button that copies text and briefly confirms it. */
export default function CopyButton({ text, children, title, className = '' }: Props) {
  const [copied, setCopied] = useState(false)

  const handleClick = async () => {
    if (await copyText(typeof text === 'function' ? text() : text)) {
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    }
  }

  return (
    <button type="button" onClick={handleClick} title={title} className={className}>
      {copied ? <span className="text-emerald-400">Copied</span> : children}
    </button>
  )
}
