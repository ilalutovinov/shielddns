export const token = {
  get: () => (typeof window !== 'undefined' ? localStorage.getItem('sd_token') : null),
  set: (t: string) => localStorage.setItem('sd_token', t),
  clear: () => localStorage.removeItem('sd_token'),
}

type Opts = { method?: string; body?: any; raw?: boolean }

export async function api<T = any>(path: string, opts: Opts = {}): Promise<T> {
  const t = token.get()
  const res = await fetch('/api' + path, {
    method: opts.method || 'GET',
    headers: { ...(opts.body ? { 'Content-Type': 'application/json' } : {}), ...(t ? { Authorization: 'Bearer ' + t } : {}) },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  })
  if (res.status === 401 && !path.startsWith('/auth')) {
    token.clear()
    if (typeof window !== 'undefined') location.href = '/login'
    throw new Error('Требуется вход')
  }
  if (!res.ok) {
    let msg = 'Ошибка запроса'
    try { msg = (await res.json()).error || msg } catch {}
    throw new Error(msg)
  }
  return (opts.raw ? res : await res.json()) as T
}

export const fmt = (n: number) => new Intl.NumberFormat('ru-RU').format(n ?? 0)
