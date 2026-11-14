'use client'
import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

export function CopyField({ label, value }: { label: string; value: string }) {
  const [ok, setOk] = useState(false)
  const copy = async () => {
    try { await navigator.clipboard.writeText(value) } catch {
      const el = document.createElement('textarea'); el.value = value; document.body.appendChild(el); el.select(); document.execCommand('copy'); el.remove()
    }
    setOk(true); setTimeout(() => setOk(false), 1500)
  }
  return (
    <div>
      <div className="muted mb-1 text-xs">{label}</div>
      <div className="flex items-center gap-2 rounded-lg border border-zinc-300 px-3 py-2 dark:border-zinc-700">
        <code className="min-w-0 flex-1 truncate font-mono text-sm">{value}</code>
        <button onClick={copy} className="btn-ghost btn-sm shrink-0">
          {ok ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
          {ok ? 'Скопировано' : 'Копировать'}
        </button>
      </div>
    </div>
  )
}
