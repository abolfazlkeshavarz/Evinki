import { createContext, useCallback, useContext, useEffect, useId, useRef, useState, type ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Info, Loader2, X } from 'lucide-react'
import clsx from 'clsx'
import { Link } from 'react-router-dom'
import { ApiError, upload } from '../lib/api'
import { isoToJalaliInput, parseJalali, latin, parseMoney, fa } from '../lib/format'
import type { Tone } from '../lib/labels'

// ───── badges & small displays ─────
const tones: Record<Tone, string> = {
  ok: 'bg-ok/12 text-ok', warn: 'bg-warn/15 text-warn', bad: 'bg-bad/12 text-bad', info: 'bg-brand-tint text-brand', mute: 'bg-muted text-soft',
}
export function Badge({ tone = 'mute', children }: { tone?: Tone; children: ReactNode }) {
  return <span className={clsx('badge', tones[tone])}>{children}</span>
}
export function StatusBadge({ map, value }: { map: Record<string, [string, Tone]>; value?: string | null }) {
  const [label, tone] = map[value ?? ''] ?? [value || '—', 'mute']
  return <Badge tone={tone}>{label}</Badge>
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={clsx('animate-spin text-soft', className ?? 'h-5 w-5')} aria-label="در حال بارگذاری" />
}
export function Loading() {
  return <div className="flex justify-center py-16"><Spinner className="h-7 w-7" /></div>
}
export function Empty({ title = 'موردی پیدا نشد', hint, action }: { title?: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-2 py-14 text-center">
      <div className="rounded-full bg-muted p-3 text-soft"><Info className="h-6 w-6" /></div>
      <div className="font-semibold">{title}</div>
      {hint && <div className="max-w-sm text-sm text-soft">{hint}</div>}
      {action}
    </div>
  )
}
export function ErrorBox({ error, retry }: { error: unknown; retry?: () => void }) {
  const msg = error instanceof ApiError ? error.message : 'خطای ناشناخته'
  return (
    <div className="m-4 flex items-start gap-3 rounded-xl border border-bad/30 bg-bad/10 p-4 text-sm text-bad">
      <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" />
      <div className="flex-1">{msg}</div>
      {retry && <button className="font-semibold underline" onClick={retry}>تلاش دوباره</button>}
    </div>
  )
}

export function PageHeader({ title, subtitle, actions }: { title: string; subtitle?: string; actions?: ReactNode }) {
  return (
    <div className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-xl font-bold md:text-2xl">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-soft">{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

export function Stat({ label, value, hint, tone, to }: { label: string; value: ReactNode; hint?: string; tone?: Tone; to?: string }) {
  const inner = (
    <div className="card p-4 transition hover:border-brand/50">
      <div className="text-xs font-medium text-soft">{label}</div>
      <div className={clsx('mt-1 text-2xl font-bold num', tone && tones[tone].split(' ')[1])}>{value}</div>
      {hint && <div className="mt-0.5 text-xs text-soft">{hint}</div>}
    </div>
  )
  return to ? <Link to={to}>{inner}</Link> : inner
}

// ───── layout helpers ─────
export const Card = ({ children, className }: { children: ReactNode; className?: string }) => <div className={clsx('card', className)}>{children}</div>

export function Tabs<T extends string>({ value, onChange, items }: { value: T; onChange: (v: T) => void; items: { key: T; label: string; count?: number }[] }) {
  return (
    <div className="-mx-1 mb-4 flex gap-1 overflow-x-auto px-1 pb-1" role="tablist">
      {items.map((t) => (
        <button key={t.key} role="tab" aria-selected={value === t.key} onClick={() => onChange(t.key)}
          className={clsx('whitespace-nowrap rounded-xl px-3.5 py-2 text-sm font-semibold transition', value === t.key ? 'bg-brand text-brand-ink' : 'bg-surface border border-line text-soft hover:text-ink')}>
          {t.label}{t.count !== undefined && <span className="ms-1.5 rounded-full bg-black/10 px-1.5 text-xs num">{fa(t.count)}</span>}
        </button>
      ))}
    </div>
  )
}

// ───── forms ─────
export function Field({ label, hint, children, className }: { label: string; hint?: string; children: ReactNode; className?: string }) {
  const id = useId()
  return (
    <div className={className}>
      <label className="label" htmlFor={id}>{label}</label>
      <div id={id}>{children}</div>
      {hint && <p className="mt-1 text-xs text-soft">{hint}</p>}
    </div>
  )
}

export function MoneyInput({ value, onChange, ...rest }: { value: number; onChange: (n: number) => void } & Omit<React.InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'>) {
  return <input {...rest} className="input num" inputMode="numeric" dir="ltr" value={value ? new Intl.NumberFormat('en-US').format(value) : ''} onChange={(e) => onChange(parseMoney(e.target.value))} />
}

/** Digits typed in any script are accepted; value is ASCII. */
export function NumInput({ value, onChange, ...rest }: { value: string | number; onChange: (s: string) => void } & Omit<React.InputHTMLAttributes<HTMLInputElement>, 'value' | 'onChange'>) {
  return <input {...rest} className={clsx('input num', rest.className)} inputMode="numeric" dir="ltr" value={value} onChange={(e) => onChange(latin(e.target.value))} />
}

/** Solar Hijri date field: shows 1405/03/17, stores ISO (YYYY-MM-DD). */
export function JalaliInput({ value, onChange, placeholder = '۱۴۰۵/۰۱/۰۱' }: { value: string; onChange: (iso: string) => void; placeholder?: string }) {
  const [text, setText] = useState(isoToJalaliInput(value))
  const [bad, setBad] = useState(false)
  useEffect(() => { setText(isoToJalaliInput(value)) }, [value])
  return (
    <input className={clsx('input num', bad && 'border-bad')} dir="ltr" inputMode="numeric" placeholder={placeholder} value={text}
      onChange={(e) => {
        const t = latin(e.target.value)
        setText(t)
        const iso = parseJalali(t)
        setBad(!!t && !iso && t.length >= 8)
        if (iso) onChange(iso)
        if (!t) onChange('')
      }} />
  )
}

export function ImageUpload({ value, onChange, purpose = 'image' }: { value?: string; onChange: (url: string) => void; purpose?: string }) {
  const [busy, setBusy] = useState(false)
  const toast = useToast()
  return (
    <div className="flex items-center gap-3">
      {value && /\.(jpg|png|webp)$|api\/v1\/files/.test(value) && <img src={value} alt="" className="h-14 w-14 rounded-xl border border-line object-cover" />}
      <label className="btn-ghost btn-sm cursor-pointer">
        {busy ? <Spinner className="h-4 w-4" /> : 'انتخاب فایل'}
        <input type="file" accept="image/*,application/pdf" className="hidden" onChange={async (e) => {
          const f = e.target.files?.[0]
          if (!f) return
          setBusy(true)
          try { onChange((await upload(f, purpose)).url) } catch (err) { toast.error(err) } finally { setBusy(false) }
        }} />
      </label>
      {value && <button type="button" className="text-xs text-bad" onClick={() => onChange('')}>حذف</button>}
    </div>
  )
}

// ───── dialogs ─────
export function Modal({ open, onClose, title, children, footer, wide }: { open: boolean; onClose: () => void; title: string; children: ReactNode; footer?: ReactNode; wide?: boolean }) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!open) return
    const prev = document.activeElement as HTMLElement | null
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    ref.current?.focus()
    return () => { document.removeEventListener('keydown', onKey); document.body.style.overflow = ''; prev?.focus() }
  }, [open, onClose])
  if (!open) return null
  return (
    <div className="fixed inset-0 z-50 flex items-end justify-center bg-black/50 p-0 sm:items-center sm:p-4" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={ref} tabIndex={-1} role="dialog" aria-modal="true" aria-label={title}
        className={clsx('flex max-h-[92vh] w-full flex-col rounded-t-2xl bg-surface shadow-2xl outline-none sm:rounded-2xl', wide ? 'sm:max-w-3xl' : 'sm:max-w-lg')}>
        <div className="flex items-center justify-between border-b border-line px-5 py-3.5">
          <h2 className="font-bold">{title}</h2>
          <button onClick={onClose} className="rounded-lg p-1.5 text-soft hover:bg-muted" aria-label="بستن"><X className="h-5 w-5" /></button>
        </div>
        <div className="flex-1 overflow-y-auto p-5">{children}</div>
        {footer && <div className="flex flex-wrap justify-end gap-2 border-t border-line px-5 py-3.5 pb-[max(0.875rem,env(safe-area-inset-bottom))]">{footer}</div>}
      </div>
    </div>
  )
}

// ───── toasts ─────
type T = { id: number; kind: 'ok' | 'err' | 'info'; text: string }
const ToastCtx = createContext<{ ok: (s: string) => void; error: (e: unknown) => void; info: (s: string) => void }>({ ok() {}, error() {}, info() {} })
export const useToast = () => useContext(ToastCtx)
export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<T[]>([])
  const push = useCallback((kind: T['kind'], text: string) => {
    const id = Date.now() + Math.random()
    setItems((x) => [...x, { id, kind, text }])
    setTimeout(() => setItems((x) => x.filter((i) => i.id !== id)), kind === 'err' ? 6000 : 3500)
  }, [])
  const api = {
    ok: (s: string) => push('ok', s),
    info: (s: string) => push('info', s),
    error: (e: unknown) => push('err', e instanceof ApiError ? e.message : e instanceof Error ? e.message : String(e)),
  }
  return (
    <ToastCtx.Provider value={api}>
      {children}
      <div className="pointer-events-none fixed inset-x-0 bottom-4 z-[60] flex flex-col items-center gap-2 px-4" aria-live="polite">
        {items.map((t) => (
          <div key={t.id} className={clsx('pointer-events-auto flex max-w-md items-center gap-2 rounded-xl px-4 py-3 text-sm font-medium shadow-lg',
            t.kind === 'ok' ? 'bg-ok text-white' : t.kind === 'err' ? 'bg-bad text-white' : 'bg-ink text-bg')}>
            {t.kind === 'ok' ? <CheckCircle2 className="h-4 w-4" /> : t.kind === 'err' ? <AlertTriangle className="h-4 w-4" /> : <Info className="h-4 w-4" />}
            {t.text}
          </div>
        ))}
      </div>
    </ToastCtx.Provider>
  )
}

/** A button that runs an async action, shows a spinner and reports errors. */
export function ActionButton({ onClick, children, className = 'btn-primary', confirm, success, disabled }: {
  onClick: () => Promise<unknown>; children: ReactNode; className?: string; confirm?: string; success?: string; disabled?: boolean
}) {
  const [busy, setBusy] = useState(false)
  const toast = useToast()
  return (
    <button className={className} disabled={busy || disabled} onClick={async () => {
      if (confirm && !window.confirm(confirm)) return
      setBusy(true)
      try { await onClick(); success && toast.ok(success) } catch (e) { toast.error(e) } finally { setBusy(false) }
    }}>
      {busy && <Spinner className="h-4 w-4 text-current" />}{children}
    </button>
  )
}

// ───── table ─────
export interface Col<T> { head: string; cell: (r: T) => ReactNode; className?: string }
export function DataTable<T>({ rows, cols, onRow, rowKey }: { rows: T[]; cols: Col<T>[]; onRow?: (r: T) => void; rowKey: (r: T) => string | number }) {
  return (
    <>
      <div className="hidden overflow-x-auto md:block">
        <table className="w-full">
          <thead className="border-b border-line bg-muted/50"><tr>{cols.map((c, i) => <th key={i} className="th">{c.head}</th>)}</tr></thead>
          <tbody className="divide-y divide-line">
            {rows.map((r) => (
              <tr key={rowKey(r)} onClick={() => onRow?.(r)} className={clsx(onRow && 'cursor-pointer hover:bg-muted/50')}>
                {cols.map((c, i) => <td key={i} className={clsx('td', c.className)}>{c.cell(r)}</td>)}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {/* phones: each row becomes a card, so nothing needs horizontal scrolling */}
      <div className="divide-y divide-line md:hidden">
        {rows.map((r) => (
          <div key={rowKey(r)} onClick={() => onRow?.(r)} className={clsx('space-y-1.5 p-4', onRow && 'active:bg-muted')}>
            {cols.map((c, i) => (
              <div key={i} className="flex items-start justify-between gap-3 text-sm">
                <span className="shrink-0 text-xs text-soft">{c.head}</span>
                <span className="text-end">{c.cell(r)}</span>
              </div>
            ))}
          </div>
        ))}
      </div>
    </>
  )
}

export function Pager({ page, total, limit, onPage }: { page: number; total: number; limit: number; onPage: (p: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / limit))
  if (pages <= 1) return null
  return (
    <div className="flex items-center justify-between border-t border-line px-4 py-3 text-sm">
      <span className="text-soft">{fa(total)} مورد</span>
      <div className="flex items-center gap-2">
        <button className="btn-ghost btn-sm" disabled={page <= 1} onClick={() => onPage(page - 1)}>قبلی</button>
        <span className="num">{fa(page)} / {fa(pages)}</span>
        <button className="btn-ghost btn-sm" disabled={page >= pages} onClick={() => onPage(page + 1)}>بعدی</button>
      </div>
    </div>
  )
}

export function SearchBox({ value, onChange, placeholder = 'جستجو…' }: { value: string; onChange: (s: string) => void; placeholder?: string }) {
  const [v, setV] = useState(value)
  useEffect(() => { const t = setTimeout(() => onChange(v), 300); return () => clearTimeout(t) }, [v])
  return <input className="input max-w-xs" type="search" value={v} onChange={(e) => setV(e.target.value)} placeholder={placeholder} aria-label={placeholder} />
}

export const KV = ({ k, v }: { k: string; v: ReactNode }) => (
  <div className="flex justify-between gap-3 border-b border-line/60 py-2 text-sm last:border-0"><span className="text-soft">{k}</span><span className="text-end font-medium">{v ?? '—'}</span></div>
)
