import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { CalendarDays, ChevronLeft, ChevronRight } from 'lucide-react'
import clsx from 'clsx'
import { isoToJalaliInput, jalaliParts, jalaliToISO, latin, parseJalali, todayISO } from '../lib/format'

const MONTHS = ['فروردین', 'اردیبهشت', 'خرداد', 'تیر', 'مرداد', 'شهریور', 'مهر', 'آبان', 'آذر', 'دی', 'بهمن', 'اسفند']
const WEEK = ['ش', 'ی', 'د', 'س', 'چ', 'پ', 'ج'] // Saturday first
const fa = (n: number | string) => String(n).replace(/\d/g, (d) => '۰۱۲۳۴۵۶۷۸۹'[+d])

function monthLength(y: number, m: number) {
  if (m <= 6) return 31
  if (m <= 11) return 30
  return jalaliToISO(y, 12, 30) ? 30 : 29 // Esfand has 30 days in leap years
}
/** Column of the first day of the month, Saturday = 0. */
function firstColumn(y: number, m: number) {
  const iso = jalaliToISO(y, m, 1)
  if (!iso) return 0
  return (new Date(iso + 'T12:00:00Z').getUTCDay() + 1) % 7
}
const partsOf = (iso: string) => jalaliParts(new Date(iso + 'T12:00:00Z'))

/**
 * Solar Hijri date field. Type the date (any digits, "1405/3/7" works) or pick it from the calendar.
 * Value in / out is an ISO date (YYYY-MM-DD) or '' for empty. The text you are typing is never rewritten
 * while the field has focus, so the cursor does not jump.
 */
export function JalaliPicker({ value, onChange, placeholder = '1405/01/01', clearable = true, min, max }: {
  value: string; onChange: (iso: string) => void; placeholder?: string; clearable?: boolean; min?: string; max?: string
}) {
  const [text, setText] = useState(isoToJalaliInput(value))
  const [bad, setBad] = useState(false)
  const [open, setOpen] = useState(false)
  const focused = useRef(false)
  const wrap = useRef<HTMLDivElement>(null)
  const input = useRef<HTMLInputElement>(null)

  // follow outside changes, but never while the user is typing
  useEffect(() => { if (!focused.current) { setText(isoToJalaliInput(value)); setBad(false) } }, [value])

  const commit = (t: string) => {
    const clean = latin(t.trim())
    if (!clean) { setBad(false); onChange(''); return }
    const iso = parseJalali(clean)
    if (!iso || (min && iso < min) || (max && iso > max)) { setBad(true); return }
    setBad(false)
    if (iso !== value) onChange(iso)
  }

  return (
    <div ref={wrap} className="relative" dir="ltr">
      <input ref={input} className={clsx('input num pe-10', bad && 'border-bad')} dir="ltr" inputMode="numeric" placeholder={placeholder} value={text}
        aria-invalid={bad} autoComplete="off"
        onFocus={() => { focused.current = true }}
        onBlur={() => { focused.current = false; commit(text); setText((t) => { const iso = parseJalali(latin(t)); return iso ? isoToJalaliInput(iso) : t }) }}
        onKeyDown={(e) => { if (e.key === 'ArrowDown' || (e.altKey && e.key === 'ArrowDown')) { e.preventDefault(); setOpen(true) } if (e.key === 'Enter') { commit(text) } }}
        onChange={(e) => {
          const t = latin(e.target.value).replace(/[^\d/.\-]/g, '')
          setText(t)
          const iso = parseJalali(t)
          setBad(!!t && !iso && t.length >= 8)
          if (iso && !(min && iso < min) && !(max && iso > max)) onChange(iso)
          if (!t) onChange('')
        }} />
      <button type="button" tabIndex={-1} aria-label="باز کردن تقویم" className="absolute end-1 top-1/2 -translate-y-1/2 rounded-lg p-2 text-soft hover:bg-muted"
        onMouseDown={(e) => e.preventDefault()} onClick={() => setOpen((o) => !o)}><CalendarDays className="h-4 w-4" /></button>
      {open && <Calendar anchor={wrap} value={value} min={min} max={max} clearable={clearable}
        onPick={(iso) => { onChange(iso); setText(isoToJalaliInput(iso)); setBad(false); setOpen(false); input.current?.focus() }}
        onClose={() => setOpen(false)} />}
    </div>
  )
}

function Calendar({ anchor, value, min, max, clearable, onPick, onClose }: {
  anchor: React.RefObject<HTMLDivElement | null>; value: string; min?: string; max?: string; clearable: boolean; onPick: (iso: string) => void; onClose: () => void
}) {
  const today = todayISO()
  const start = partsOf(value || today)
  const [view, setView] = useState({ y: start.y, m: start.m })
  const [pos, setPos] = useState<{ top: number; left: number; up: boolean } | null>(null)
  const box = useRef<HTMLDivElement>(null)

  // position under the field (or above when there is no room), as a fixed layer so dialogs do not clip it
  useLayoutEffect(() => {
    const place = () => {
      const r = anchor.current?.getBoundingClientRect()
      if (!r) return
      const h = 340, w = 288
      const up = r.bottom + h > window.innerHeight && r.top > h
      setPos({ top: up ? r.top - 6 : r.bottom + 6, left: Math.min(Math.max(8, r.left), window.innerWidth - w - 8), up })
    }
    place()
    window.addEventListener('resize', place); window.addEventListener('scroll', place, true)
    return () => { window.removeEventListener('resize', place); window.removeEventListener('scroll', place, true) }
  }, [anchor])
  useEffect(() => {
    const down = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node) && !anchor.current?.contains(e.target as Node)) onClose() }
    const key = (e: KeyboardEvent) => { if (e.key === 'Escape') { e.stopPropagation(); onClose() } }
    document.addEventListener('mousedown', down); document.addEventListener('keydown', key, true)
    return () => { document.removeEventListener('mousedown', down); document.removeEventListener('keydown', key, true) }
  }, [anchor, onClose])

  const shift = (n: number) => setView((v) => { const t = v.y * 12 + (v.m - 1) + n; return { y: Math.floor(t / 12), m: (t % 12) + 1 } })
  const col = firstColumn(view.y, view.m)
  const len = monthLength(view.y, view.m)
  const cells: (number | null)[] = [...Array(col).fill(null), ...Array.from({ length: len }, (_, i) => i + 1)]
  const sel = value ? partsOf(value) : null
  const tp = partsOf(today)
  const years = Array.from({ length: 81 }, (_, i) => tp.y - 60 + i)
  if (!pos) return null
  return createPortal(
    <div ref={box} role="dialog" aria-label="تقویم شمسی" dir="rtl" className="fixed z-[70] w-72 rounded-2xl border border-line bg-surface p-3 shadow-2xl"
      style={{ top: pos.top, left: pos.left, transform: pos.up ? 'translateY(-100%)' : undefined }}>
      <div className="mb-2 flex items-center gap-1">
        <button type="button" className="rounded-lg p-2 hover:bg-muted" onClick={() => shift(-1)} aria-label="ماه قبل"><ChevronRight className="h-4 w-4" /></button>
        <select className="min-h-0 flex-1 rounded-lg border border-line bg-surface px-2 py-1.5 text-sm font-semibold" value={view.m} onChange={(e) => setView({ ...view, m: +e.target.value })}>{MONTHS.map((n, i) => <option key={n} value={i + 1}>{n}</option>)}</select>
        <select className="min-h-0 w-[5.2rem] rounded-lg border border-line bg-surface px-2 py-1.5 text-sm font-semibold" value={view.y} onChange={(e) => setView({ ...view, y: +e.target.value })}>{(years.includes(view.y) ? years : [view.y, ...years]).map((y) => <option key={y} value={y}>{fa(y)}</option>)}</select>
        <button type="button" className="rounded-lg p-2 hover:bg-muted" onClick={() => shift(1)} aria-label="ماه بعد"><ChevronLeft className="h-4 w-4" /></button>
      </div>
      <div className="grid grid-cols-7 text-center text-xs text-soft">{WEEK.map((w, i) => <div key={w} className={clsx('py-1', i === 6 && 'text-bad')}>{w}</div>)}</div>
      <div className="grid grid-cols-7 gap-0.5 text-center">
        {cells.map((d, i) => {
          if (d === null) return <span key={'e' + i} />
          const iso = jalaliToISO(view.y, view.m, d) ?? ''
          const off = (!!min && iso < min) || (!!max && iso > max)
          const isSel = sel && sel.y === view.y && sel.m === view.m && sel.d === d
          const isToday = tp.y === view.y && tp.m === view.m && tp.d === d
          return <button key={d} type="button" disabled={off} onClick={() => onPick(iso)}
            className={clsx('grid h-9 place-items-center rounded-lg text-sm num transition', isSel ? 'bg-brand font-bold text-brand-ink' : isToday ? 'border border-brand font-bold text-brand' : 'hover:bg-muted', (i % 7 === 6) && !isSel && 'text-bad', off && 'pointer-events-none opacity-30')}>{fa(d)}</button>
        })}
      </div>
      <div className="mt-2 flex items-center justify-between border-t border-line pt-2 text-xs">
        <button type="button" className="rounded-lg px-2 py-1.5 font-semibold text-brand hover:bg-brand-tint" onClick={() => onPick(today)}>امروز</button>
        {clearable && <button type="button" className="rounded-lg px-2 py-1.5 text-soft hover:bg-muted" onClick={() => onPick('')}>پاک کردن</button>}
      </div>
    </div>, document.body)
}
