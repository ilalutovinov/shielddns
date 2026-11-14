'use client'
import { useCallback, useEffect, useState } from 'react'
import { Area, AreaChart, CartesianGrid, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { Activity, Ban, Percent } from 'lucide-react'
import { api, fmt } from '@/lib/api'
import { CopyField } from '../Copy'

export default function Overview({ me }: { me: any }) {
  const [range, setRange] = useState('24h')
  const [stats, setStats] = useState<any>(null)
  const [logs, setLogs] = useState<any[]>([])
  const [done, setDone] = useState<Record<string, string>>({})

  const loadStats = useCallback(async () => { try { setStats(await api('/stats?range=' + range)) } catch {} }, [range])
  const loadLogs = useCallback(async () => { try { setLogs(await api('/logs?limit=40')) } catch {} }, [])
  useEffect(() => { loadStats(); const t = setInterval(loadStats, 15000); return () => clearInterval(t) }, [loadStats])
  useEffect(() => { loadLogs(); const t = setInterval(loadLogs, 3000); return () => clearInterval(t) }, [loadLogs])

  async function act(domain: string, action: 'allow' | 'block') {
    try {
      await api('/profile/rules', { method: 'POST', body: { domain, action } })
      setDone((d) => ({ ...d, [domain]: action === 'allow' ? 'Добавлен в белый список' : 'Добавлен в чёрный список' }))
    } catch (e: any) { setDone((d) => ({ ...d, [domain]: e.message })) }
  }

  const t = stats?.totals
  const tick = (v: string) => { const d = new Date(v); return range === '24h' ? d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' }) : d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' }) }
  const cards = [
    { l: 'Всего запросов', v: fmt(t?.total), i: Activity },
    { l: 'Заблокировано', v: fmt(t?.blocked), i: Ban },
    { l: 'Доля блокировок', v: (t?.percent ?? 0).toFixed(1) + '%', i: Percent },
  ]

  return (
    <div className="space-y-6">
      <div className="card grid gap-4 md:grid-cols-2">
        <CopyField label="DNS-over-HTTPS (DoH)" value={me.endpoints.doh} />
        <CopyField label="DNS-over-TLS (DoT) — имя хоста" value={me.endpoints.dot} />
        <div className="muted text-xs md:col-span-2">Ваш ID профиля: <code className="font-mono">{me.profile.clientId}</code>. Инструкции для устройств — во вкладке «Настройка».</div>
      </div>

      {me.limit && (
        <div className="card">
          <div className="mb-2 flex justify-between text-sm"><span>Использовано в этом месяце (Free)</span><span>{fmt(me.usage)} / {fmt(me.limit)}</span></div>
          <div className="h-2 overflow-hidden rounded-full bg-zinc-200 dark:bg-zinc-800"><div className="h-full bg-emerald-500" style={{ width: Math.min(100, (me.usage / me.limit) * 100) + '%' }} /></div>
        </div>
      )}

      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Статистика</h2>
        <div className="flex gap-1">{['24h', '7d', '30d'].map((r) => <button key={r} onClick={() => setRange(r)} className={`tab ${range === r ? 'tab-active' : ''}`}>{r}</button>)}</div>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        {cards.map(({ l, v, i: Icon }) => (
          <div key={l} className="card flex items-center gap-4"><Icon className="h-8 w-8 text-emerald-500" /><div><div className="muted text-sm">{l}</div><div className="text-2xl font-bold">{v}</div></div></div>
        ))}
      </div>

      <div className="card">
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <AreaChart data={stats?.series || []}>
              <CartesianGrid strokeDasharray="3 3" stroke="#71717a33" />
              <XAxis dataKey="t" tickFormatter={tick} fontSize={12} stroke="#71717a" />
              <YAxis fontSize={12} stroke="#71717a" allowDecimals={false} />
              <Tooltip labelFormatter={(v) => tick(v as string)} contentStyle={{ borderRadius: 8 }} />
              <Area type="monotone" dataKey="total" name="Всего" stroke="#10b981" fill="#10b98133" />
              <Area type="monotone" dataKey="blocked" name="Заблокировано" stroke="#ef4444" fill="#ef444433" />
            </AreaChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="card">
          <h3 className="mb-3 font-semibold">Топ заблокированных доменов</h3>
          {stats?.topBlocked?.length ? (
            <ul className="space-y-1.5 text-sm">{stats.topBlocked.map((d: any) => (
              <li key={d.domain} className="flex justify-between gap-2"><code className="truncate font-mono">{d.domain}</code><span className="muted">{fmt(d.hits)}</span></li>
            ))}</ul>
          ) : <p className="muted text-sm">Пока нет данных — данные появятся после первых запросов.</p>}
        </div>
        <div className="card">
          <h3 className="mb-3 font-semibold">Топ разрешённых доменов</h3>
          {stats?.topAllowed?.length ? (
            <ul className="space-y-1.5 text-sm">{stats.topAllowed.map((d: any) => (
              <li key={d.domain} className="flex justify-between gap-2"><code className="truncate font-mono">{d.domain}</code><span className="muted">{fmt(d.hits)}</span></li>
            ))}</ul>
          ) : <p className="muted text-sm">Пока нет данных.</p>}
        </div>
      </div>

      <div className="card">
        <h3 className="mb-3 font-semibold">Лента запросов <span className="muted text-xs font-normal">(обновляется каждые 3 секунды)</span></h3>
        {!me.profile.logging ? <p className="muted text-sm">Логирование выключено (вкладка «Фильтры»).</p> : logs.length === 0 ? <p className="muted text-sm">Запросов пока нет.</p> : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <tbody>{logs.map((l, i) => (
                <tr key={i} className="border-t border-zinc-200 dark:border-zinc-800">
                  <td className="muted whitespace-nowrap py-2 pr-3">{new Date(l.t).toLocaleTimeString('ru-RU')}</td>
                  <td className="py-2 pr-3"><code className="font-mono">{l.domain}</code> <span className="muted text-xs">{l.type}</span></td>
                  <td className="py-2 pr-3"><span className={`pill ${l.status === 'blocked' ? 'bg-red-500/15 text-red-500' : 'bg-emerald-500/15 text-emerald-500'}`}>{l.status === 'blocked' ? 'заблокирован' : 'разрешён'}</span> <span className="muted text-xs">{l.reason}</span></td>
                  <td className="py-2 text-right">
                    {done[l.domain] ? <span className="muted text-xs">{done[l.domain]}</span> : l.status === 'blocked'
                      ? <button className="btn-ghost btn-sm" onClick={() => act(l.domain, 'allow')}>Разблокировать</button>
                      : <button className="btn-ghost btn-sm" onClick={() => act(l.domain, 'block')}>Заблокировать</button>}
                  </td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
