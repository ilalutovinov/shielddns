import Link from 'next/link'
import { BarChart3, Check, Globe2, Lock, ShieldCheck, Smartphone, Zap } from 'lucide-react'
import ThemeToggle from '@/components/Theme'
import Demo from '@/components/Demo'

const features = [
  { i: Lock, t: 'DoH и DoT', d: 'Шифрованный DNS по RFC 8484 и порту 853 — провайдер не видит и не подменяет ваши запросы.' },
  { i: Zap, t: 'Быстро', d: 'Поиск по спискам за O(1) в памяти, кэш ответов в Redis, статистика не тормозит DNS.' },
  { i: Smartphone, t: 'Настройка в 1 клик', d: 'Готовый .mobileconfig для iOS и macOS, инструкции для Android, Windows и роутеров.' },
  { i: BarChart3, t: 'Аналитика', d: 'Графики, топ заблокированных доменов и живая лента с кнопкой «Разблокировать».' },
  { i: Globe2, t: 'Свой профиль', d: 'Персональный адрес вида a7f9k2.dns.example.com — настройки только ваши.' },
  { i: ShieldCheck, t: 'Категории и правила', d: 'Реклама, трекеры, вредоносные сайты, соцсети, 18+ плюс белый и чёрный списки.' },
]

const plans = [
  { n: 'Free', p: '0 ₽', f: ['До 50 000 запросов в месяц', 'Все категории фильтров', 'До 10 своих правил', 'Статистика и лента запросов'] },
  { n: 'Pro', p: '299 ₽/мес', hot: true, f: ['Безлимит запросов', 'Неограниченные белый/чёрный списки', 'Все категории фильтров', 'Приоритетная поддержка'] },
]

export default function Landing() {
  return (
    <div>
      <header className="sticky top-0 z-10 border-b border-zinc-200 bg-white/80 backdrop-blur dark:border-zinc-800 dark:bg-zinc-950/80">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-4">
          <Link href="/" className="flex items-center gap-2 font-semibold"><ShieldCheck className="h-5 w-5 text-emerald-500" />ShieldDNS</Link>
          <nav className="flex items-center gap-2 text-sm">
            <a href="#pricing" className="muted hidden px-2 hover:text-emerald-500 sm:block">Тарифы</a>
            <ThemeToggle />
            <Link href="/login" className="btn-ghost btn-sm">Войти</Link>
            <Link href="/register" className="btn-primary btn-sm">Начать бесплатно</Link>
          </nav>
        </div>
      </header>

      <section className="mx-auto max-w-4xl px-4 pb-16 pt-20 text-center">
        <span className="pill bg-emerald-500/15 text-emerald-500">DNS-фильтр нового поколения</span>
        <h1 className="mt-4 text-4xl font-bold tracking-tight sm:text-6xl">
          Интернет без рекламы и трекеров <span className="text-emerald-500">на всех устройствах</span>
        </h1>
        <p className="muted mx-auto mt-5 max-w-2xl text-lg">
          Укажите персональный адрес ShieldDNS в настройках телефона, компьютера или роутера — и реклама, трекеры и вредоносные домены блокируются ещё до загрузки.
        </p>
        <div className="mt-8 flex justify-center gap-3">
          <Link href="/register" className="btn-primary px-6 py-3">Создать профиль</Link>
          <a href="#how" className="btn-ghost px-6 py-3">Как это работает</a>
        </div>
      </section>

      <section id="how" className="mx-auto max-w-6xl px-4 py-12">
        <h2 className="mb-8 text-center text-2xl font-semibold">Как это работает</h2>
        <div className="grid gap-4 md:grid-cols-3">
          {['Регистрация — получаете адрес вида a7f9k2.dns.example.com', 'Вписываете его в настройки устройства (DoH, DoT или .mobileconfig)', 'Каждый запрос проверяется по вашим спискам: разрешён → апстрим, реклама → блок'].map((t, i) => (
            <div key={i} className="card"><div className="mb-2 flex h-8 w-8 items-center justify-center rounded-full bg-emerald-500 font-semibold text-white">{i + 1}</div><p>{t}</p></div>
          ))}
        </div>
      </section>

      <section className="mx-auto max-w-6xl px-4 py-12">
        <h2 className="mb-8 text-center text-2xl font-semibold">Попробуйте</h2>
        <Demo />
      </section>

      <section className="mx-auto max-w-6xl px-4 py-12">
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {features.map(({ i: Icon, t, d }) => (
            <div key={t} className="card"><Icon className="mb-3 h-6 w-6 text-emerald-500" /><h3 className="font-semibold">{t}</h3><p className="muted mt-1 text-sm">{d}</p></div>
          ))}
        </div>
      </section>

      <section id="pricing" className="mx-auto max-w-4xl px-4 py-16">
        <h2 className="mb-8 text-center text-2xl font-semibold">Тарифы</h2>
        <div className="grid gap-4 md:grid-cols-2">
          {plans.map((p) => (
            <div key={p.n} className={`card ${p.hot ? '!border-emerald-500' : ''}`}>
              <div className="flex items-baseline justify-between"><h3 className="text-lg font-semibold">{p.n}</h3><span className="text-2xl font-bold">{p.p}</span></div>
              <ul className="my-4 space-y-2 text-sm">{p.f.map((x) => <li key={x} className="flex gap-2"><Check className="h-4 w-4 shrink-0 text-emerald-500" />{x}</li>)}</ul>
              <Link href="/register" className={p.hot ? 'btn-primary w-full' : 'btn-ghost w-full'}>Выбрать {p.n}</Link>
            </div>
          ))}
        </div>
      </section>

      <footer className="border-t border-zinc-200 py-8 text-center text-sm dark:border-zinc-800"><span className="muted">© ShieldDNS — учебный MVP</span></footer>
    </div>
  )
}
