'use client'
import { Moon, Sun } from 'lucide-react'

export default function ThemeToggle() {
  const toggle = () => {
    const dark = document.documentElement.classList.toggle('dark')
    localStorage.setItem('sd_theme', dark ? 'dark' : 'light')
  }
  return (
    <button onClick={toggle} aria-label="Сменить тему" className="btn-ghost btn-sm !p-2">
      <Sun className="hidden h-4 w-4 dark:block" />
      <Moon className="h-4 w-4 dark:hidden" />
    </button>
  )
}
