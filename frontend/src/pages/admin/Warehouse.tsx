import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import QRCode from 'qrcode'
import { Download, Minus, Plus, Printer, Trash2 } from 'lucide-react'
import clsx from 'clsx'
import { ActionButton, Badge, Card, DataTable, Empty, ErrorBox, Field, KV, Loading, Modal, NumInput, PageHeader, Pager, SearchBox, StatusBadge, Tabs, useToast } from '../../components/ui'
import { Scanner } from '../../components/Scanner'
import { ApiError, del, download, get, post, put } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { dateTime, fa } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { docStatus } from '../../lib/labels'

function useMeta() { return useGet<any>('/wh/meta') }

export function Stock() {
  const [sp, setSp] = useSearchParams()
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [wh, setWh] = useState(0)
  const low = sp.get('low') === '1'
  const meta = useMeta()
  const d = useGet<any>('/wh/items' + qs({ q, page, limit: 30, low: low ? 1 : undefined, warehouse_id: wh || undefined }))
  const can = useCan()
  return (
    <div>
      <PageHeader title="موجودی انبار" actions={<>{can('wh.manage') && <button className="btn-ghost btn-sm" onClick={() => download('/wh/exports/movements.csv', 'movements.csv')}><Download className="h-4 w-4" />خروجی حرکات (CSV)</button>}</>} />
      <Card>
        <div className="flex flex-wrap items-center gap-3 border-b border-line p-3">
          <SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} placeholder="نام، کد یا شماره قطعه" />
          <select className="input w-auto" value={wh} onChange={(e) => { setWh(+e.target.value); setPage(1) }} aria-label="انبار"><option value={0}>انبار مرکزی</option>{meta.data?.warehouses.filter((w: any) => w.kind !== 'central').map((w: any) => <option key={w.id} value={w.id}>{w.title}</option>)}</select>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={low} onChange={(e) => { const n = new URLSearchParams(sp); e.target.checked ? n.set('low', '1') : n.delete('low'); setSp(n); setPage(1) }} />فقط کمبود موجودی</label>
        </div>
        {d.isLoading ? <Loading /> : d.error ? <ErrorBox error={d.error} /> : !d.data.items.length ? <Empty title="قطعه‌ای پیدا نشد" /> : (
          <>
            <DataTable rows={d.data.items} rowKey={(r: any) => r.id} cols={[
              { head: 'قطعه', cell: (r: any) => <div className="flex items-center gap-3">{r.image && <img src={r.image} alt="" className="h-10 w-10 rounded-lg object-cover" />}<div><b>{r.name}</b>{r.variant && <span className="text-soft"> — {r.variant}</span>}<div className="text-xs text-soft" dir="ltr">{r.code}</div></div></div> },
              { head: 'محل', cell: (r: any) => r.location || '—' }, { head: 'حداقل', cell: (r: any) => fa(r.min_stock) },
              { head: 'موجودی', cell: (r: any) => <span className={clsx('text-base font-bold num', r.low && 'text-bad')}>{fa(r.stock)} {r.low && <Badge tone="bad">کمبود</Badge>}</span> }]} />
            <Pager page={page} total={d.data.total} limit={d.data.limit} onPage={setPage} />
          </>
        )}
      </Card>
    </div>
  )
}

type Line = { part_id: number; code: string; name: string; variant?: string; qty: number; stock: number }

export function Scan() {
  const meta = useMeta()
  const can = useCan()
  const toast = useToast()
  const inv = useInvalidate()
  const [dir, setDir] = useState<'in' | 'out'>('out')
  const [reason, setReason] = useState('to_worker')
  const [dest, setDest] = useState(0)
  const [worker, setWorker] = useState('')
  const [note, setNote] = useState('')
  const [lines, setLines] = useState<Line[]>([])
  const [manual, setManual] = useState('')
  const [done, setDone] = useState<any>(null)
  const workers = useGet<string[]>('/wh/workers')
  const reasons = (meta.data?.reasons ?? []).filter((r: any) => r.direction === dir)
  useEffect(() => { if (reasons.length && !reasons.find((r: any) => r.key === reason)) setReason(reasons[0].key) }, [dir, meta.data])
  const needsDest = reasons.find((r: any) => r.key === reason)?.needs_dest
  const scrap = meta.data?.warehouses.find((w: any) => w.kind === 'scrap')

  const add = async (raw: string) => {
    const code = raw.trim()
    if (!code) return
    try {
      const p = await get<any>('/wh/items/scan/' + encodeURIComponent(code))
      setLines((ls) => {
        const i = ls.findIndex((l) => l.part_id === p.id)
        if (i >= 0) return ls.map((l, j) => (j === i ? { ...l, qty: l.qty + 1 } : l))
        return [...ls, { part_id: p.id, code: p.code, name: p.name, variant: p.variant, qty: 1, stock: p.stock }]
      })
      toast.info(`${p.name} اضافه شد`)
    } catch (e) { toast.error(e instanceof ApiError ? e : new Error('قطعه پیدا نشد')) }
  }
  const submit = async () => {
    const body: any = { direction: dir, reason, worker_name: worker, note, client_ref: crypto.randomUUID(), lines: lines.map((l) => ({ part_id: l.part_id, qty: l.qty })) }
    if (needsDest) body.dest_warehouse_id = dest || scrap?.id
    const r = await post('/wh/documents', body)
    setDone(r.document)
    setLines([])
    inv('/wh')
  }
  if (done) {
    return (
      <div className="mx-auto max-w-md space-y-4 text-center">
        <Card className="p-8"><div className="text-5xl">{done.status === 'pending' ? '⏳' : '✅'}</div><h2 className="mt-3 text-lg font-bold">سند شماره {fa(done.doc_no)} ثبت شد</h2>
          <p className="mt-1 text-sm text-soft">{done.status === 'pending' ? 'موجودی پس از تایید مدیر تغییر می‌کند.' : 'موجودی به‌روز شد.'}</p></Card>
        <button className="btn-primary w-full" onClick={() => setDone(null)}>ثبت حرکت بعدی</button>
      </div>
    )
  }
  return (
    <div className="mx-auto max-w-2xl space-y-4">
      <PageHeader title="ثبت حرکت انبار" subtitle="کد QR قطعه را اسکن کنید یا کد را وارد کنید." />
      <div className="grid grid-cols-2 gap-2">
        {([['out', 'خروج از انبار'], ['in', 'ورود به انبار']] as const).map(([k, l]) => <button key={k} onClick={() => setDir(k)} className={clsx('btn border text-base', dir === k ? 'border-brand bg-brand text-brand-ink' : 'border-line bg-surface')}>{l}</button>)}
      </div>
      <Card className="grid gap-3 p-4 sm:grid-cols-2">
        <Field label="علت"><select className="input" value={reason} onChange={(e) => setReason(e.target.value)}>{reasons.map((r: any) => <option key={r.key} value={r.key}>{r.label}</option>)}</select></Field>
        {needsDest && <Field label="انبار مقصد"><select className="input" value={dest || scrap?.id || 0} onChange={(e) => setDest(+e.target.value)}>{meta.data?.warehouses.map((w: any) => <option key={w.id} value={w.id}>{w.title}</option>)}</select></Field>}
        {dir === 'out' && !needsDest && <Field label="نام کارگر / تحویل‌گیرنده"><input className="input" list="workers" value={worker} onChange={(e) => setWorker(e.target.value)} /><datalist id="workers">{workers.data?.map((w) => <option key={w} value={w} />)}</datalist></Field>}
      </Card>
      <Scanner onCode={add} />
      <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); add(manual); setManual('') }}>
        <input className="input" dir="ltr" placeholder="کد قطعه یا QR را وارد کنید" value={manual} onChange={(e) => setManual(e.target.value)} aria-label="کد قطعه" /><button className="btn-ghost">افزودن</button>
      </form>
      <Card>
        {!lines.length ? <Empty title="هنوز قطعه‌ای اضافه نشده" hint="اسکن کنید یا کد را تایپ کنید." /> : lines.map((l) => (
          <div key={l.part_id} className="flex items-center gap-3 border-b border-line/60 p-3 last:border-0">
            <div className="min-w-0 flex-1"><div className="truncate font-semibold">{l.name}{l.variant && ` — ${l.variant}`}</div><div className="text-xs text-soft">موجودی فعلی: {fa(l.stock)} {dir === 'out' && l.qty > l.stock && meta.data?.settings.check_stock && <span className="font-bold text-bad">— بیشتر از موجودی!</span>}</div></div>
            <div className="flex items-center rounded-xl border border-line">
              <button className="p-3" aria-label="کم" onClick={() => setLines(lines.map((x) => (x === l ? { ...x, qty: Math.max(1, x.qty - 1) } : x)))}><Minus className="h-4 w-4" /></button>
              <input className="w-12 bg-transparent text-center font-bold num" inputMode="numeric" value={l.qty} onChange={(e) => setLines(lines.map((x) => (x === l ? { ...x, qty: Math.max(1, +e.target.value.replace(/\D/g, '') || 1) } : x)))} aria-label="تعداد" />
              <button className="p-3" aria-label="زیاد" onClick={() => setLines(lines.map((x) => (x === l ? { ...x, qty: x.qty + 1 } : x)))}><Plus className="h-4 w-4" /></button>
            </div>
            <button className="p-2 text-bad" aria-label="حذف" onClick={() => setLines(lines.filter((x) => x !== l))}><Trash2 className="h-5 w-5" /></button>
          </div>
        ))}
      </Card>
      <Field label="توضیح"><input className="input" value={note} onChange={(e) => setNote(e.target.value)} /></Field>
      {dir === 'out' && !can('wh.write') && meta.data?.settings.approval_out && <p className="rounded-xl bg-warn/10 p-3 text-sm text-warn">خروج‌ها پس از تایید مدیر اعمال می‌شوند.</p>}
      <ActionButton className="btn-primary w-full text-base" disabled={!lines.length} onClick={submit}>{dir === 'out' ? 'ثبت خروج' : 'ثبت ورود'} ({fa(lines.reduce((a, l) => a + l.qty, 0))} عدد)</ActionButton>
    </div>
  )
}

export function Documents() {
  const [status, setStatus] = useState('pending')
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [view, setView] = useState<number | null>(null)
  const [note, setNote] = useState('')
  const can = useCan()
  const inv = useInvalidate()
  const list = useGet<any>('/wh/documents' + qs({ status, q, page, limit: 20 }))
  const one = useGet<any>(view ? `/wh/documents/${view}` : null)
  const reasonLabel = useMeta().data?.reasons
  const lab = (k: string) => reasonLabel?.find((r: any) => r.key === k)?.label ?? ({ tech_delivery: 'تحویل به تکنسین', repair_use: 'مصرف در تعمیر', return_old: 'دریافت قطعه قدیمی' } as any)[k] ?? k
  const doc = one.data?.document
  return (
    <div>
      <PageHeader title="اسناد انبار" />
      <Tabs value={status} onChange={(s) => { setStatus(s); setPage(1) }} items={[{ key: 'pending', label: 'منتظر تایید', count: list.data?.pending }, { key: 'approved', label: 'تایید شده' }, { key: 'rejected', label: 'رد شده' }, { key: 'cancelled', label: 'لغو شده' }, { key: '', label: 'همه' }]} />
      <Card>
        <div className="border-b border-line p-3"><SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} placeholder="شماره سند، کارگر یا قطعه" /></div>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data.items.length ? <Empty title="سندی نیست" /> : (
          <>
            <DataTable rows={list.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => { setView(r.id); setNote('') }} cols={[
              { head: 'سند', cell: (r: any) => <b className="num">{fa(r.doc_no)}</b> }, { head: 'نوع', cell: (r: any) => <span><Badge tone={r.direction === 'in' ? 'ok' : 'warn'}>{r.direction === 'in' ? 'ورود' : 'خروج'}</Badge> <span className="text-xs text-soft">{lab(r.reason)}</span></span> },
              { head: 'اقلام', cell: (r: any) => `${fa(r.line_count)} قلم / ${fa(r.total_qty)} عدد` }, { head: 'ثبت‌کننده', cell: (r: any) => r.created_by_name || 'سیستم' }, { head: 'زمان', cell: (r: any) => <span className="text-xs">{dateTime(r.created_at)}</span> },
              { head: 'وضعیت', cell: (r: any) => <StatusBadge map={docStatus} value={r.status} /> }]} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} />
          </>
        )}
      </Card>
      <Modal open={!!view} onClose={() => setView(null)} wide title={doc ? `سند ${fa(doc.doc_no)}` : 'سند'}
        footer={doc && <>
          {doc.status === 'pending' && can('wh.approve') && <><ActionButton className="btn-ghost text-bad" onClick={async () => { await post(`/wh/documents/${view}/reject`, { note }); inv('/wh'); setView(null) }}>رد</ActionButton><ActionButton onClick={async () => { await post(`/wh/documents/${view}/approve`, { note }); inv('/wh'); setView(null) }} success="تایید شد و موجودی به‌روز شد">تایید</ActionButton></>}
          {doc.status === 'pending' && !can('wh.approve') && <ActionButton className="btn-ghost" onClick={async () => { await post(`/wh/documents/${view}/cancel`); inv('/wh'); setView(null) }}>لغو سند</ActionButton>}
          {can('wh.delete') && doc.status !== 'cancelled' && doc.source_type === 'manual' && <ActionButton className="btn-ghost text-bad" confirm="سند حذف و اثر آن بر موجودی برگردانده شود؟ (دلیل را در کادر یادداشت بنویسید)" onClick={async () => { await del(`/wh/documents/${view}`, { note: note || 'حذف' }); inv('/wh'); setView(null) }}>حذف سند</ActionButton>}
        </>}>
        {one.isLoading || !doc ? <Loading /> : <>
          <KV k="نوع" v={`${doc.direction === 'in' ? 'ورود' : 'خروج'} — ${lab(doc.reason)}`} /><KV k="انبار" v={doc.warehouse + (doc.dest_warehouse ? ` ← ${doc.dest_warehouse}` : '')} />
          {doc.worker_name && <KV k="کارگر" v={doc.worker_name} />}<KV k="ثبت‌کننده" v={doc.created_by_name || 'سیستم'} /><KV k="زمان ثبت" v={dateTime(doc.created_at)} />{doc.decided_by_name && <KV k="تصمیم‌گیرنده" v={`${doc.decided_by_name} — ${dateTime(doc.decided_at)}`} />}
          {doc.note && <KV k="توضیح" v={doc.note} />}{doc.decision_note && <KV k="یادداشت تصمیم" v={doc.decision_note} />}
          <table className="mt-4 w-full text-sm"><thead><tr className="text-soft"><th className="th">قطعه</th><th className="th">تعداد</th><th className="th">قبل</th><th className="th">بعد</th></tr></thead><tbody>
            {one.data.lines.map((l: any) => <tr key={l.id} className="border-t border-line/60"><td className="td">{l.name}{l.variant && ` — ${l.variant}`}<div className="text-xs text-soft" dir="ltr">{l.code}</div></td><td className="td font-bold num">{fa(l.qty)}</td><td className="td num">{l.stock_before ?? '—'}</td><td className="td num">{l.stock_after ?? '—'}</td></tr>)}</tbody></table>
          {(doc.status === 'pending' || can('wh.delete')) && <Field label="یادداشت (برای رد یا حذف الزامی است)" className="mt-4"><input className="input" value={note} onChange={(e) => setNote(e.target.value)} /></Field>}
        </>}
      </Modal>
    </div>
  )
}

export function Counts() {
  const list = useGet<any[]>('/wh/counts')
  const [open, setOpen] = useState<number | null>(null)
  const inv = useInvalidate()
  const nav = useNavigate()
  return (
    <div>
      <PageHeader title="انبارگردانی" actions={<ActionButton onClick={async () => { const r = await post('/wh/counts', { title: 'انبارگردانی ' + new Date().toLocaleDateString('fa-IR') }); inv('/wh/counts'); setOpen(r.id) }}>شروع انبارگردانی جدید</ActionButton>} />
      <Card>{list.isLoading ? <Loading /> : !list.data?.length ? <Empty title="انبارگردانی ثبت نشده" /> : list.data.map((c) => (
        <button key={c.id} onClick={() => setOpen(c.id)} className="flex w-full items-center justify-between gap-3 border-b border-line/60 p-4 text-start last:border-0 hover:bg-muted/40"><div><b>{c.title || `#${c.id}`}</b><div className="text-xs text-soft">{dateTime(c.created_at)} · {fa(c.lines)} قلم شمرده شده</div></div><Badge tone={{ open: 'info', submitted: 'warn', applied: 'ok', rejected: 'bad' }[c.status as string] as any}>{{ open: 'در حال شمارش', submitted: 'منتظر تایید', applied: 'اعمال شد', rejected: 'رد شد' }[c.status as string]}</Badge></button>
      ))}</Card>
      {open && <CountDetail id={open} onClose={() => setOpen(null)} />}
      <span className="hidden">{nav.name}</span>
    </div>
  )
}

function CountDetail({ id, onClose }: { id: number; onClose: () => void }) {
  const d = useGet<any>(`/wh/counts/${id}`)
  const can = useCan()
  const inv = useInvalidate()
  const [q, setQ] = useState('')
  const items = useGet<any>(q.length >= 2 ? '/wh/items' + qs({ q, limit: 8 }) : null)
  const [val, setVal] = useState<Record<number, string>>({})
  const c = d.data
  return (
    <Modal open onClose={onClose} wide title={c?.title || 'انبارگردانی'} footer={c && <>
      {c.status === 'open' && <ActionButton onClick={async () => { await post(`/wh/counts/${id}/submit`); inv('/wh/counts') }}>ارسال برای تایید</ActionButton>}
      {c.status === 'submitted' && can('wh.manage') && <><ActionButton className="btn-ghost text-bad" onClick={async () => { await post(`/wh/counts/${id}/reject`); inv('/wh/counts'); onClose() }}>رد</ActionButton><ActionButton confirm="اختلاف‌ها به‌صورت سند اصلاحی در موجودی اعمال شود؟" onClick={async () => { await post(`/wh/counts/${id}/apply`); inv('/wh'); onClose() }} success="موجودی اصلاح شد">اعمال بر موجودی</ActionButton></>}
    </>}>
      {d.isLoading || !c ? <Loading /> : <>
        {c.status === 'open' && <>
          <input className="input mb-2" placeholder="جستجوی قطعه برای ثبت شمارش…" value={q} onChange={(e) => setQ(e.target.value)} />
          {items.data?.items.map((p: any) => (
            <div key={p.id} className="flex items-center gap-2 border-b border-line/60 py-2 text-sm"><span className="flex-1">{p.name} <span className="text-xs text-soft" dir="ltr">{p.code}</span></span>
              <div className="w-24"><NumInput value={val[p.id] ?? ''} onChange={(v) => setVal({ ...val, [p.id]: v })} placeholder="شمارش" /></div>
              <ActionButton className="btn-soft btn-sm" onClick={async () => { await put(`/wh/counts/${id}/lines`, { part_id: p.id, counted: +(val[p.id] || 0) }); inv(`/wh/counts/${id}`) }}>ثبت</ActionButton></div>
          ))}
        </>}
        <table className="mt-4 w-full text-sm"><thead><tr className="text-soft"><th className="th">قطعه</th><th className="th">شمارش</th>{c.status !== 'open' || can('wh.manage') ? <th className="th">سیستم</th> : null}</tr></thead><tbody>
          {c.lines.map((l: any) => <tr key={l.part_id} className="border-t border-line/60"><td className="td">{l.name}</td><td className="td font-bold num">{fa(l.counted)}</td>{l.expected !== null && <td className={clsx('td num', l.expected !== l.counted && 'font-bold text-bad')}>{fa(l.expected)}</td>}</tr>)}</tbody></table>
        {c.result && <p className="mt-3 text-sm text-soft">{c.result}</p>}
      </>}
    </Modal>
  )
}

export function Labels() {
  const [q, setQ] = useState('')
  const [sel, setSel] = useState<Record<number, any>>({})
  const [size, setSize] = useState<'s' | 'm' | 'l'>('m')
  const d = useGet<any>('/wh/items' + qs({ q, limit: 100 }))
  const chosen = Object.values(sel)
  const [urls, setUrls] = useState<Record<number, string>>({})
  useEffect(() => { chosen.forEach(async (p: any) => { if (!urls[p.id]) { const u = await QRCode.toDataURL('WH:' + p.qr_token, { margin: 0, width: 240, errorCorrectionLevel: 'M' }); setUrls((x) => ({ ...x, [p.id]: u })) } }) }, [sel])
  const dim = { s: 'w-[30mm] h-[22mm]', m: 'w-[45mm] h-[30mm]', l: 'w-[70mm] h-[40mm]' }[size]
  const rows = useMemo(() => d.data?.items ?? [], [d.data])
  return (
    <div>
      <div className="no-print">
        <PageHeader title="چاپ برچسب QR" subtitle="QR فقط یک شناسه یکتا دارد؛ با ویرایش کد قطعه برچسب‌های چاپ‌شده معتبر می‌مانند." actions={<><select className="input w-auto" value={size} onChange={(e) => setSize(e.target.value as any)}><option value="s">کوچک ۳۰×۲۲</option><option value="m">متوسط ۴۵×۳۰</option><option value="l">بزرگ ۷۰×۴۰</option></select><button className="btn-primary" disabled={!chosen.length} onClick={() => window.print()}><Printer className="h-4 w-4" />چاپ ({fa(chosen.length)})</button></>} />
        <Card>
          <div className="flex items-center gap-3 border-b border-line p-3"><SearchBox value={q} onChange={setQ} /><button className="btn-ghost btn-sm" onClick={() => setSel(Object.fromEntries(rows.map((p: any) => [p.id, p])))}>انتخاب همه نتایج</button><button className="btn-ghost btn-sm" onClick={() => setSel({})}>پاک کردن</button></div>
          <div className="max-h-[50vh] overflow-y-auto">{rows.map((p: any) => <label key={p.id} className="flex items-center gap-3 border-b border-line/60 px-4 py-2.5 text-sm"><input type="checkbox" checked={!!sel[p.id]} onChange={(e) => { const n = { ...sel }; e.target.checked ? (n[p.id] = p) : delete n[p.id]; setSel(n) }} />{p.name} {p.variant && `— ${p.variant}`} <span className="text-xs text-soft" dir="ltr">{p.code}</span></label>)}</div>
        </Card>
      </div>
      <div className="mt-6 flex flex-wrap gap-2 print:mt-0">
        {chosen.map((p: any) => (
          <div key={p.id} className={clsx('flex items-center gap-2 break-inside-avoid border border-dashed border-line bg-white p-1.5 text-black', dim)}>
            {urls[p.id] && <img src={urls[p.id]} alt="" className="aspect-square h-full" />}
            <div className="min-w-0 text-[9px] leading-tight"><div className="font-bold" dir="ltr">{p.code}</div><div className="line-clamp-3">{p.name}{p.variant && ` ${p.variant}`}</div></div>
          </div>
        ))}
      </div>
    </div>
  )
}

void Link
