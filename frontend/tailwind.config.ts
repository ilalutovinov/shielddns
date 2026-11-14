import type { Config } from 'tailwindcss'
export default {
  darkMode: 'class',
  content: ['./app/**/*.{ts,tsx}', './components/**/*.{ts,tsx}'],
  theme: { extend: { fontFamily: { mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'] } } },
  plugins: [],
} satisfies Config
