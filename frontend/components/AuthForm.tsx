'use client'
import { useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { ShieldCheck } from 'lucide-react'
import { api, token } from '@/lib/api'
import ThemeToggle from './Theme'

export default function AuthForm({ mode }: { mode: 'login' | 'register' }) {
  const r = useRouter()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)
  const reg = mode === 'register'

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true); setErr('')
    try {
      const { token: t } = await api<{ token: string }>(reg ? '/auth/register' : '/auth/login', { method: 'POST', body: { email, password } })
      token.set(t)
      r.push('/dashboard')
    } catch (e: any) { setErr(e.message) } finally { setBusy(false) }
  }

  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <div className="absolute right-4 top-4"><ThemeToggle /></div>
      <form onSubmit={submit} className="card w-full max-w-sm space-y-4">
        <Link href="/" className="flex items-center gap-2 font-semibold"><ShieldCheck className="h-5 w-5 text-emerald-500" />ShieldDNS</Link>
        <h1 className="text-xl font-semibold">{reg ? 'Создать аккаунт' : 'Вход'}</h1>
        <input className="input" type="email" required placeholder="Email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" />
        <input className="input" type="password" required minLength={8} placeholder="Пароль (от 8 символов)" value={password}
          onChange={(e) => setPassword(e.target.value)} autoComplete={reg ? 'new-password' : 'current-password'} />
        {err && <p className="text-sm text-red-500">{err}</p>}
        <button className="btn-primary w-full" disabled={busy}>{busy ? '…' : reg ? 'Зарегистрироваться' : 'Войти'}</button>
        <p className="muted text-center text-sm">
          {reg ? <>Уже есть аккаунт? <Link className="text-emerald-500" href="/login">Войти</Link></>
               : <>Нет аккаунта? <Link className="text-emerald-500" href="/register">Регистрация</Link></>}
        </p>
      </form>
    </main>
  )
}
