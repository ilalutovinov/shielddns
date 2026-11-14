'use client'
import { useState } from 'react'
import { Download, Laptop, Router, Smartphone, Apple, Monitor } from 'lucide-react'
import { api } from '@/lib/api'
import { CopyField } from '../Copy'

const platforms = [
  { id: 'ios', t: 'iOS / iPadOS', i: Apple },
  { id: 'android', t: 'Android', i: Smartphone },
  { id: 'windows', t: 'Windows', i: Monitor },
  { id: 'macos', t: 'macOS', i: Laptop },
  { id: 'router', t: 'Роутеры', i: Router },
]

function Steps({ items }: { items: React.ReactNode[] }) {
  return (
    <ol className="space-y-3">{items.map((s, i) => (
      <li key={i} className="flex gap-3"><span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-emerald-500 text-xs font-semibold text-white">{i + 1}</span><div className="text-sm">{s}</div></li>
    ))}</ol>
  )
}

// Схематичный «экран настроек» вместо скриншотов: показывает, куда что вписывать.
function Mock({ title, rows }: { title: string; rows: [string, string][] }) {
  return (
    <div className="mt-2 max-w-sm overflow-hidden rounded-xl border border-zinc-300 text-sm dark:border-zinc-700">
      <div className="bg-zinc-100 px-4 py-2 text-center font-medium dark:bg-zinc-800">{title}</div>
      {rows.map(([k, v]) => (
        <div key={k} className="flex justify-between gap-3 border-t border-zinc-200 px-4 py-2 dark:border-zinc-800"><span className="muted">{k}</span><code className="truncate font-mono text-emerald-600 dark:text-emerald-400">{v}</code></div>
      ))}
    </div>
  )
}

export default function Setup({ me }: { me: any }) {
  const [p, setP] = useState('ios')
  const [ip, setIp] = useState('')
  const [err, setErr] = useState('')
  const { doh, dot } = me.endpoints
  const base = me.endpoints.baseDomain

  async function dl(proto: 'https' | 'tls') {
    try {
      const res: Response = await api('/profile/mobileconfig?proto=' + proto, { raw: true })
      const a = document.createElement('a')
      a.href = URL.createObjectURL(await res.blob())
      a.download = `shielddns-${proto === 'https' ? 'doh' : 'dot'}.mobileconfig`
      a.click()
    } catch (e: any) { setErr(e.message) }
  }
  async function linkIp() {
    try { setIp((await api('/profile/link-ip', { method: 'POST' })).ip) } catch (e: any) { setErr(e.message) }
  }

  const profileBtns = (
    <div className="flex flex-wrap gap-2">
      <button className="btn-primary" onClick={() => dl('https')}><Download className="h-4 w-4" />Скачать профиль (DoH)</button>
      <button className="btn-ghost" onClick={() => dl('tls')}><Download className="h-4 w-4" />Скачать профиль (DoT)</button>
    </div>
  )

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap gap-1">{platforms.map(({ id, t, i: Icon }) => (
        <button key={id} onClick={() => setP(id)} className={`tab flex items-center gap-1.5 ${p === id ? 'tab-active' : ''}`}><Icon className="h-4 w-4" />{t}</button>
      ))}</div>
      {err && <p className="text-sm text-red-500">{err}</p>}

      <div className="card space-y-5">
        {p === 'ios' && <>
          <h3 className="font-semibold">iPhone и iPad (iOS 14+)</h3>
          {profileBtns}
          <Steps items={[
            'Откройте эту страницу в Safari на iPhone и нажмите «Скачать профиль» → «Разрешить».',
            <>Откройте <b>Настройки → Основные → VPN и управление устройством</b> (или «Профиль загружен» вверху настроек) и нажмите «Установить».</>,
            'Введите код-пароль. Профиль не подписан сертификатом — iOS покажет предупреждение, это нормально для личного профиля.',
            <>Проверьте: <b>Настройки → Основные → VPN и управление устройством → DNS</b> — выбран ShieldDNS.</>,
          ]} />
          <Mock title="DNS" rows={[['Тип', 'Зашифрованный DNS'], ['Сервер', doh.replace('https://', '')]]} />
        </>}

        {p === 'android' && <>
          <h3 className="font-semibold">Android 9+ (Приватный DNS)</h3>
          <CopyField label="Имя хоста поставщика частного DNS" value={dot} />
          <Steps items={[
            <>Откройте <b>Настройки → Сеть и интернет → Частный DNS</b> (на некоторых прошивках: Подключения → Другие настройки → Частный DNS).</>,
            <>Выберите <b>«Имя хоста поставщика частного DNS»</b>.</>,
            'Вставьте имя хоста выше и нажмите «Сохранить».',
            'Готово: Android использует DNS-over-TLS на порту 853 для Wi-Fi и мобильной сети.',
          ]} />
          <Mock title="Частный DNS" rows={[['Режим', 'Имя хоста поставщика'], ['Хост', dot]]} />
        </>}

        {p === 'windows' && <>
          <h3 className="font-semibold">Windows 11 (DNS-over-HTTPS)</h3>
          <CopyField label="Шаблон DoH" value={doh} />
          <Steps items={[
            <>Выполните в PowerShell (от администратора), подставив имя адаптера: <code className="font-mono">Get-NetAdapter</code>.</>,
            <>Укажите DNS-серверы 1.1.1.1 и зарегистрируйте шаблон DoH: <code className="block overflow-x-auto rounded bg-zinc-100 p-2 font-mono text-xs dark:bg-zinc-800">Add-DnsClientDohServerAddress -ServerAddress 1.1.1.1 -DohTemplate {doh} -AllowFallbackToUdp $false -AutoUpgrade $true</code></>,
            <>Или через интерфейс: <b>Параметры → Сеть и Интернет → Wi-Fi/Ethernet → Свойства → Назначение DNS-сервера → Вручную</b>. Введите IP DNS-сервера, включите «Шифрование DNS» и вставьте шаблон выше.</>,
          ]} />
          <p className="muted text-xs">Windows 11 привязывает шаблон DoH к IP-адресу известного DNS-сервера. Если у вас публичный IP сервера ShieldDNS, укажите его в качестве DNS-адреса.</p>
        </>}

        {p === 'macos' && <>
          <h3 className="font-semibold">macOS 11+</h3>
          {profileBtns}
          <Steps items={[
            'Скачайте профиль и дважды щёлкните по файлу.',
            <>Откройте <b>Системные настройки → Конфиденциальность и безопасность → Профили</b> и нажмите «Установить».</>,
            'Подтвердите паролем. DNS-запросы всей системы пойдут через ShieldDNS.',
          ]} />
        </>}

        {p === 'router' && <>
          <h3 className="font-semibold">Роутеры и домашняя сеть</h3>
          <p className="text-sm">Вариант 1 — DoT/DoH прямо на роутере (OpenWrt, Keenetic, Mikrotik, pfSense):</p>
          <CopyField label="DoT (stubby / «DNS через TLS»): хост и порт 853" value={dot} />
          <CopyField label="DoH (https-dns-proxy): шаблон" value={doh} />
          <pre className="overflow-x-auto rounded-lg bg-zinc-100 p-3 font-mono text-xs dark:bg-zinc-800">{`# OpenWrt: /etc/config/https-dns-proxy
config https-dns-proxy
  option resolver_url '${doh}'
  option listen_addr '127.0.0.1'
  option listen_port '5053'`}</pre>
          <p className="text-sm">Вариант 2 — обычный DNS (порт 53). Роутер не умеет DoH/DoT, но у вас статический внешний IP: привяжите его к профилю и укажите IP сервера ShieldDNS в настройках DHCP/WAN.</p>
          <button className="btn-ghost" onClick={linkIp}>Привязать мой текущий IP к профилю</button>
          {ip && <p className="text-sm">Привязан IP <code className="font-mono">{ip}</code> (30 дней). Запросы с него на порт 53 сервера <code className="font-mono">{base}</code> будут фильтроваться вашим профилем.</p>}
          <p className="muted text-xs">Привязка по IP работает для всех устройств за одним NAT. Если IP меняется, нажмите кнопку снова.</p>
        </>}
      </div>
    </div>
  )
}
