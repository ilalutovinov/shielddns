'use client'
import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { CreditCard, Filter, LayoutDashboard, LogOut, ShieldCheck, Smartphone } from 'lucide-react'
import { api, token } from '@/lib/api'
import ThemeToggle from '@/components/Theme'
import Overview from '@/components/dashboard/Overview'
import Filters from '@/components/dashboard/Filters'
import Setup from '@/components/dashboard/Setup'
import Billing from '@/components/dashboard/Billing'

const tabs = [
  { id: 'overview', t: 'Обзор', i: LayoutDashboard },
  { id: 'filters', t: 'Фильтры', i: Filter },
  { id: 'setup', t: 'Настройка', i: Smartphone },
  { id: 'billing', t: 'Подписка', i: CreditCard },
]

export default function Dashboard() {
  const r = useRouter()
  const [me, setMe] = useState<any>(null)
  const [tab, setTab] = useState('overview')

  const load = useCallback(async () => { try { setMe(await api('/me')) } catch {} }, [])
  useEffect(() => {
    if (!token.get()) { r.replace('/login'); return }
    load()
  }, [load, r])

  if (!me) return <main className="flex min-h-screen items-center justify-center"><span className="muted">Загрузка…</span></main>

  return (
    <div className="min-h-screen">
      <header className="border-b border-zinc-200 dark:border-zinc-800">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
          <Link href="/" className="flex items-center gap-2 font-semibold"><ShieldCheck className="h-5 w-5 text-emerald-500" />ShieldDNS</Link>
          <div className="flex items-center gap-2">
            <span className={`pill ${me.plan === 'pro' ? 'bg-emerald-500/15 text-emerald-500' : 'bg-zinc-200 dark:bg-zinc-800'}`}>{me.plan.toUpperCase()}</span>
            <span className="muted hidden text-sm sm:block">{me.email}</span>
            <ThemeToggle />
            <button className="btn-ghost btn-sm" onClick={() => { token.clear(); r.push('/') }}><LogOut className="h-3.5 w-3.5" />Выйти</button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-6xl space-y-6 px-4 py-6">
        <nav className="flex flex-wrap gap-1">
          {tabs.map(({ id, t, i: Icon }) => (
            <button key={id} onClick={() => setTab(id)} className={`tab flex items-center gap-1.5 ${tab === id ? 'tab-active' : ''}`}><Icon className="h-4 w-4" />{t}</button>
          ))}
        </nav>
        {tab === 'overview' && <Overview me={me} />}
        {tab === 'filters' && <Filters me={me} reload={load} />}
        {tab === 'setup' && <Setup me={me} />}
        {tab === 'billing' && <Billing me={me} reload={load} />}
      </main>
    </div>
  )
}
