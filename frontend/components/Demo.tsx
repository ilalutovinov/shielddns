'use client'
import { useEffect, useState } from 'react'
import { ShieldBan, ShieldCheck } from 'lucide-react'

const BLOCKED = ['doubleclick.net', 'googlesyndication.com', 'ads.facebook.com', 'app-measurement.com', 'adservice.google.com', 'tracking.example.net', 'metrics.mixpanel.com']
const ALLOWED = ['wikipedia.org', 'github.com', 'news.ycombinator.com', 'cloudflare.com', 'duckduckgo.com']

function check(d: string) {
  const x = d.trim().toLowerCase().replace(/^https?:\/\//, '').split('/')[0]
  if (!x) return null
  return BLOCKED.some((b) => x === b || x.endsWith('.' + b)) || /(^|\.)(ads?|track(er|ing)?|metrics|analytics|telemetry)\./.test(x)
}

export default function Demo() {
  const [feed, setFeed] = useState<{ d: string; b: boolean; k: number }[]>([])
  const [input, setInput] = useState('')
  const verdict = check(input)

  useEffect(() => {
    let k = 0
    const t = setInterval(() => {
      const b = Math.random() < 0.55
      const pool = b ? BLOCKED : ALLOWED
      const d = pool[Math.floor(Math.random() * pool.length)]
      setFeed((f) => [{ d, b, k: k++ }, ...f].slice(0, 6))
    }, 1100)
    return () => clearInterval(t)
  }, [])

  return (
    <div className="grid gap-6 md:grid-cols-2">
      <div className="card">
        <h3 className="mb-1 font-semibold">Проверь домен</h3>
        <p className="muted mb-3 text-sm">Так ShieldDNS решает, пускать запрос или нет (упрощённая демонстрация).</p>
        <input className="input" placeholder="например, doubleclick.net" value={input} onChange={(e) => setInput(e.target.value)} />
        <div className="mt-3 h-10">
          {verdict === true && <span className="pill gap-1 bg-red-500/15 text-red-500"><ShieldBan className="mr-1 h-3.5 w-3.5" />Заблокирован: реклама/трекер</span>}
          {verdict === false && <span className="pill gap-1 bg-emerald-500/15 text-emerald-500"><ShieldCheck className="mr-1 h-3.5 w-3.5" />Разрешён</span>}
        </div>
      </div>
      <div className="card">
        <h3 className="mb-3 font-semibold">Запросы в реальном времени</h3>
        <ul className="space-y-1.5">
          {feed.map((r) => (
            <li key={r.k} className="flex items-center justify-between rounded-md bg-zinc-100 px-3 py-1.5 text-sm dark:bg-zinc-800/60">
              <code className="truncate font-mono">{r.d}</code>
              <span className={`pill ${r.b ? 'bg-red-500/15 text-red-500' : 'bg-emerald-500/15 text-emerald-500'}`}>{r.b ? 'blocked' : 'allowed'}</span>
            </li>
          ))}
        </ul>
      </div>
    </div>
  )
}
