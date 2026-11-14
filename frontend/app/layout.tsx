import './globals.css'
import type { Metadata } from 'next'

export const metadata: Metadata = {
  title: 'ShieldDNS — DNS-фильтр рекламы и трекеров',
  description: 'Облачный DNS с блокировкой рекламы, DoH и DoT, личным кабинетом и аналитикой.',
  icons: { icon: '/favicon.svg' },
}

const themeScript = `try{var t=localStorage.getItem('sd_theme');if(t==='dark'||(!t&&matchMedia('(prefers-color-scheme: dark)').matches))document.documentElement.classList.add('dark')}catch(e){}`

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru" suppressHydrationWarning>
      <head><script dangerouslySetInnerHTML={{ __html: themeScript }} /></head>
      <body>{children}</body>
    </html>
  )
}
