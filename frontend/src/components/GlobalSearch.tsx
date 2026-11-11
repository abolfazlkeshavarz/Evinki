import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search } from 'lucide-react'
import { get } from '../lib/api'

const groups: [string, string, (id: number, label: string) => string][] = [
  ['guarantees', 'گارانتی‌ها', (id) => `/admin/guarantees/${id}`],
  ['customers', 'مشتریان', (_, l) => `/admin/customers?q=${encodeURIComponent(l)}`],
  ['products', 'محصولات', () => '/admin/products'],
  ['repairs', 'تعمیرها', (id) => `/admin/repairs/${id}`],
  ['technicians', 'تکنسین‌ها', (id) => `/admin/technicians/${id}`],
]

/** One box in the admin header that looks in guarantees, customers, products, repairs and technicians. */
export function GlobalSearch() {
  const [q, setQ] = useState('')
  const [res, setRes] = useState<any>(null)
  const [open, setOpen] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const nav = useNavigate()

  useEffect(() => {
    if (q.trim().length < 2) { setRes(null); return }
    const t = setTimeout(async () => { try { setRes(await get('/admin/search?q=' + encodeURIComponent(q))) } catch { setRes(null) } }, 250)
    return () => clearTimeout(t)
  }, [q])
  useEffect(() => {
    const d = (e: MouseEvent) => { if (!box.current?.contains(e.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', d)
    return () => document.removeEventListener('mousedown', d)
  }, [])

  const any = res && groups.some(([k]) => res[k]?.length)
  return (
    <div ref={box} className="relative hidden w-full max-w-sm md:block">
      <Search className="pointer-events-none absolute end-3 top-1/2 h-4 w-4 -translate-y-1/2 text-soft" />
      <input className="input pe-9" placeholder="جستجو: کد گارانتی، مشتری، محصول، تعمیر…" value={q} aria-label="جستجوی سراسری"
        onFocus={() => setOpen(true)} onChange={(e) => { setQ(e.target.value); setOpen(true) }} onKeyDown={(e) => e.key === 'Escape' && setOpen(false)} />
      {open && q.trim().length >= 2 && (
        <div className="absolute inset-x-0 top-full z-50 mt-1 max-h-96 overflow-y-auto rounded-2xl border border-line bg-surface p-2 shadow-2xl">
          {!res ? <div className="p-3 text-sm text-soft">در حال جستجو…</div> : !any ? <div className="p-3 text-sm text-soft">موردی پیدا نشد</div> : groups.map(([k, title, to]) => res[k]?.length ? (
            <div key={k} className="mb-1">
              <div className="px-2 py-1 text-[11px] font-bold text-soft">{title}</div>
              {res[k].map((r: any) => (
                <button key={k + r.id} className="flex w-full items-center justify-between gap-3 rounded-lg px-2 py-2 text-start text-sm hover:bg-muted" onClick={() => { setOpen(false); setQ(''); nav(to(r.id, r.label)) }}>
                  <b dir="auto" className="truncate">{r.label}</b><span dir="auto" className="truncate text-xs text-soft">{r.sub}</span>
                </button>))}
            </div>) : null)}
        </div>)}
    </div>
  )
}
