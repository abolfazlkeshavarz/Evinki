/** Colours are CSS variables (see index.css) so light/dark and the brand colour change in one place. */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  darkMode: ['class', '[data-theme="dark"]'],
  theme: {
    extend: {
      fontFamily: { sans: ['Vazirmatn', 'system-ui', 'sans-serif'] },
      colors: {
        bg: 'rgb(var(--bg) / <alpha-value>)',
        surface: 'rgb(var(--surface) / <alpha-value>)',
        muted: 'rgb(var(--muted) / <alpha-value>)',
        line: 'rgb(var(--line) / <alpha-value>)',
        ink: 'rgb(var(--ink) / <alpha-value>)',
        soft: 'rgb(var(--soft) / <alpha-value>)',
        brand: { DEFAULT: 'rgb(var(--brand) / <alpha-value>)', ink: 'rgb(var(--brand-ink) / <alpha-value>)', dark: 'rgb(var(--brand-dark) / <alpha-value>)', tint: 'rgb(var(--brand-tint) / <alpha-value>)' },
        ok: 'rgb(var(--ok) / <alpha-value>)',
        warn: 'rgb(var(--warn) / <alpha-value>)',
        bad: 'rgb(var(--bad) / <alpha-value>)',
      },
      borderRadius: { xl: '0.9rem', '2xl': '1.2rem' },
      boxShadow: { card: '0 1px 2px rgb(0 0 0 / .04), 0 4px 16px rgb(0 0 0 / .05)' },
    },
  },
  plugins: [],
}
