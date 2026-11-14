'use client'
import { useCallback, useEffect, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { api, fmt } from '@/lib/api'

function Switch({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button role="switch" aria-checked={on} onClick={() => onChange(!on)}
      className={`relative h-6 w-11 shrink-0 rounded-full transition ${on ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-700'}`}>
      <span className={`absolute top-0.5 h-5 w-5 rounded-full bg-white transition-all ${on ? 'left-[22px]' : 'left-0.5'}`} />
    </button>
  )
}

function RuleColumn({ title, action, rules, onAdd, onDel }: any) {
  const [v, setV] = useState('')
  const [err, setErr] = useState('')
  const list = rules.filter((r: any) => r.action === action)
  async function add(e: React.FormEvent) {
    e.preventDefault(); setErr('')
    try { await onAdd(v, action); setV('') } catch (e: any) { setErr(e.message) }
  }
  return (
    <div className="card">
      <h3 className="mb-3 font-semibold">{title}</h3>
      <form onSubmit={add} className="flex gap-2"><input className="input" placeholder="example.com" value={v} onChange={(e) => setV(e.target.value)} /><button className="btn-primary">Добавить</button></form>
      {err && <p className="mt-2 text-sm text-red-500">{err}</p>}
      <ul className="mt-3 space-y-1.5">{list.map((r: any) => (
        <li key={r.id} className="flex items-center justify-between rounded-md bg-zinc-100 px-3 py-1.5 text-sm dark:bg-zinc-800/60">
          <code className="truncate font-mono">{r.domain}</code>
          <button onClick={() => onDel(r.id)} aria-label="Удалить" className="muted hover:text-red-500"><Trash2 className="h-4 w-4" /></button>
        </li>
      ))}{list.length === 0 && <li className="muted text-sm">Список пуст. Правило действует и на все поддомены.</li>}</ul>
    </div>
  )
}

export default function Filters({ me, reload }: { me: any; reload: () => void }) {
  const [filters, setFilters] = useState<any[]>([])
  const [rules, setRules] = useState<any[]>([])
  const [mode, setMode] = useState(me.profile.blockMode)
  const [logging, setLogging] = useState(me.profile.logging)

  const load = useCallback(async () => {
    setFilters(await api('/profile/filters')); setRules(await api('/profile/rules'))
  }, [])
  useEffect(() => { load() }, [load])

  async function toggle(slug: string, on: boolean) {
    const next = filters.map((f) => (f.slug === slug ? { ...f, enabled: on } : f))
    setFilters(next)
    await api('/profile/filters', { method: 'PUT', body: { lists: next.filter((f) => f.enabled).map((f) => f.slug) } })
  }
  async function patch(body: any) { await api('/profile', { method: 'PATCH', body }); reload() }
  async function addRule(domain: string, action: string) { await api('/profile/rules', { method: 'POST', body: { domain, action } }); setRules(await api('/profile/rules')) }
  async function delRule(id: number) { await api('/profile/rules/' + id, { method: 'DELETE' }); setRules(await api('/profile/rules')) }

  return (
    <div className="space-y-6">
      <div className="card">
        <h3 className="mb-3 font-semibold">Готовые списки</h3>
        <ul className="divide-y divide-zinc-200 dark:divide-zinc-800">{filters.map((f) => (
          <li key={f.slug} className="flex items-center justify-between gap-4 py-3">
            <div><div className="font-medium">{f.title}</div><div className="muted text-xs">{f.domains ? fmt(f.domains) + ' доменов' : 'список загружается'}</div></div>
            <Switch on={f.enabled} onChange={(v) => toggle(f.slug, v)} />
          </li>
        ))}</ul>
      </div>

      <div className="card space-y-4">
        <h3 className="font-semibold">Параметры профиля</h3>
        <div className="flex items-center justify-between gap-4">
          <div><div className="font-medium">Ответ на заблокированный домен</div><div className="muted text-xs">0.0.0.0 / :: или NXDOMAIN</div></div>
          <select className="input !w-auto" value={mode} onChange={(e) => { setMode(e.target.value); patch({ blockMode: e.target.value }) }}>
            <option value="null_ip">0.0.0.0 / ::</option><option value="nxdomain">NXDOMAIN</option>
          </select>
        </div>
        <div className="flex items-center justify-between gap-4">
          <div><div className="font-medium">Сохранять журнал запросов</div><div className="muted text-xs">Лента и топ доменов. Счётчики запросов ведутся всегда.</div></div>
          <Switch on={logging} onChange={(v) => { setLogging(v); patch({ logging: v }) }} />
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <RuleColumn title="Белый список (всегда разрешать)" action="allow" rules={rules} onAdd={addRule} onDel={delRule} />
        <RuleColumn title="Чёрный список (всегда блокировать)" action="block" rules={rules} onAdd={addRule} onDel={delRule} />
      </div>
      {me.plan === 'free' && <p className="muted text-sm">На тарифе Free доступно до 10 правил. Whitelist имеет приоритет над blacklist.</p>}
    </div>
  )
}
