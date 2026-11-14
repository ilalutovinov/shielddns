'use client'
import { useState } from 'react'
import { Check } from 'lucide-react'
import { api } from '@/lib/api'

const plans = [
  { id: 'free', n: 'Free', p: '0 ₽', f: ['До 50 000 запросов в месяц', 'До 10 своих правил', 'Все категории фильтров'] },
  { id: 'pro', n: 'Pro', p: '299 ₽/мес', f: ['Безлимит запросов', 'Неограниченные правила', 'Все категории фильтров'] },
]

export default function Billing({ me, reload }: { me: any; reload: () => void }) {
  const [busy, setBusy] = useState('')
  async function choose(id: string) {
    setBusy(id)
    try { await api('/subscription', { method: 'POST', body: { plan: id } }); await reload() } finally { setBusy('') }
  }
  return (
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2">
        {plans.map((p) => {
          const cur = me.plan === p.id
          return (
            <div key={p.id} className={`card ${cur ? '!border-emerald-500' : ''}`}>
              <div className="flex items-baseline justify-between"><h3 className="text-lg font-semibold">{p.n}</h3><span className="text-2xl font-bold">{p.p}</span></div>
              <ul className="my-4 space-y-2 text-sm">{p.f.map((x) => <li key={x} className="flex gap-2"><Check className="h-4 w-4 text-emerald-500" />{x}</li>)}</ul>
              <button disabled={cur || !!busy} onClick={() => choose(p.id)} className={cur ? 'btn-ghost w-full' : 'btn-primary w-full'}>
                {cur ? 'Текущий тариф' : busy === p.id ? '…' : 'Перейти на ' + p.n}
              </button>
            </div>
          )
        })}
      </div>
      <p className="muted text-sm">Это заглушка оплаты для MVP: переключение тарифа происходит сразу, без списания денег.
        {me.planExpires && <> Pro активен до {new Date(me.planExpires).toLocaleDateString('ru-RU')}.</>}</p>
    </div>
  )
}
