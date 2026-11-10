import { useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { AlertTriangle, Camera, CheckCircle2, ChevronLeft, Minus, PackagePlus, Phone, Plus, Search, Send, Trash2, Wrench } from 'lucide-react'
import clsx from 'clsx'
import { ActionButton, Badge, Card, Empty, ErrorBox, Field, ImageUpload, KV, Loading, Modal, NumInput, PageHeader, StatusBadge, Tabs, useToast } from '../../components/ui'
import { ApiError, del, get, patch, post, upload } from '../../lib/api'
import { useUser } from '../../lib/auth'
import { ago, date, dateTime, fa, toman } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { ledgerKind, method, repairStatus, requestStatus, returnState, shipmentStatus } from '../../lib/labels'

function Blocked({ error }: { error: unknown }) {
  if (error instanceof ApiError && error.code === 'pending_approval')
    return <Card className="m-2 p-8 text-center"><div className="text-4xl">⏳</div><h2 className="mt-3 font-bold">درخواست شما در انتظار تایید است</h2><p className="mt-2 text-sm text-soft">پس از تایید دفتر مرکزی، می‌توانید کار را شروع کنید.</p></Card>
  return <ErrorBox error={error} />
}

function Big({ to, icon: Icon, label, n, tone }: { to: string; icon: any; label: string; n?: number; tone?: string }) {
  return (
    <Link to={to} className="card flex items-center gap-3 p-4 active:scale-[.99]">
      <div className={clsx('grid h-12 w-12 shrink-0 place-items-center rounded-xl', tone ?? 'bg-brand-tint text-brand')}><Icon className="h-6 w-6" /></div>
      <div className="flex-1 font-semibold">{label}</div>
      {n !== undefined && <span className="text-2xl font-extrabold num">{fa(n)}</span>}
      <ChevronLeft className="h-5 w-5 text-soft" />
    </Link>
  )
}

export function TechHome() {
  const u = useUser()
  const d = useGet<any>('/tech/dashboard', { refetchInterval: 60_000 })
  if (d.isLoading) return <Loading />
  if (d.error) return <Blocked error={d.error} />
  const x = d.data
  return (
    <div className="space-y-5">
      <div><h1 className="text-xl font-bold">سلام {u?.full_name?.split(' ')[0]} 👋</h1><p className="text-sm text-soft">امروز چه کاری دارید؟</p></div>
      <Link to="/tech/jobs/new" className="btn-primary min-h-[56px] w-full text-base"><Plus className="h-5 w-5" />ثبت تعمیر جدید</Link>
      <div className="space-y-2.5">
        {x.to_start > 0 && <Big to="/tech/jobs" icon={Wrench} label="تعمیر محول‌شده برای شروع" n={x.to_start} tone="bg-warn/15 text-warn" />}
        {x.rejected > 0 && <Big to="/tech/jobs" icon={AlertTriangle} label="گزارش ردشده (نیاز به اصلاح)" n={x.rejected} tone="bg-bad/10 text-bad" />}
        <Big to="/tech/jobs" icon={Wrench} label="تعمیرهای در حال انجام" n={x.open_repairs} />
        {x.parts_to_send > 0 && <Big to="/tech/returns" icon={Send} label="قطعه قدیمی که باید بفرستید" n={x.parts_to_send} tone="bg-warn/15 text-warn" />}
        {x.parcels_in_transit > 0 && <Big to="/tech/returns" icon={Send} label="مرسوله در راه" n={x.parcels_in_transit} />}
      </div>
      <Card className="grid grid-cols-2 divide-x divide-x-reverse divide-line">
        <Link to="/tech/account" className="p-4"><div className="text-xs text-soft">مانده حساب شما</div><div className={clsx('text-xl font-extrabold num', x.balance > 0 && 'text-ok')}>{toman(x.balance)}</div></Link>
        <Link to="/tech/stock" className="p-4"><div className="text-xs text-soft">قطعات در انبار شما</div><div className="text-xl font-extrabold num">{fa(x.stock_units)} عدد</div></Link>
      </Card>
      {x.awaiting_review > 0 && <p className="text-center text-sm text-soft">{fa(x.awaiting_review)} گزارش منتظر بررسی دفتر مرکزی است.</p>}
    </div>
  )
}

export function Jobs() {
  const [tab, setTab] = useState('active')
  const d = useGet<any[]>('/tech/repairs')
  const groups: Record<string, string[]> = { active: ['assigned', 'in_progress', 'waiting_parts', 'rejected'], review: ['submitted'], done: ['approved', 'cancelled'] }
  const list = (d.data ?? []).filter((r) => groups[tab].includes(r.status))
  return (
    <div>
      <PageHeader title="تعمیرها" actions={<Link to="/tech/jobs/new" className="btn-primary btn-sm"><Plus className="h-4 w-4" />جدید</Link>} />
      <Tabs value={tab} onChange={setTab} items={[{ key: 'active', label: 'فعال' }, { key: 'review', label: 'در بررسی' }, { key: 'done', label: 'پایان‌یافته' }]} />
      {d.isLoading ? <Loading /> : d.error ? <Blocked error={d.error} /> : !list.length ? <Empty title="موردی نیست" /> : (
        <div className="space-y-2.5">{list.map((r) => (
          <Link key={r.id} to={`/tech/jobs/${r.id}`} className="card block p-4 active:scale-[.99]">
            <div className="flex items-start justify-between gap-2"><div><b>{r.customer_name}</b> <span className="text-xs text-soft">{r.customer_city}</span></div><StatusBadge map={repairStatus} value={r.status} /></div>
            <div className="mt-1 text-sm text-soft">{r.product_name} · <span className="num">{r.number}</span></div>
            {r.complaint && <p className="mt-1 line-clamp-1 text-sm">{r.complaint}</p>}
            <div className="mt-2 flex flex-wrap gap-1.5">{r.guarantee_was_expired && <Badge tone="warn">خارج از گارانتی</Badge>}{r.has_parts_to_return && <Badge tone="warn">قطعه قدیمی برای ارسال</Badge>}</div>
          </Link>))}</div>
      )}
    </div>
  )
}

export function NewJob() {
  const nav = useNavigate()
  const toast = useToast()
  const inv = useInvalidate()
  const [code, setCode] = useState('')
  const [lk, setLk] = useState<any>(null)
  const [manual, setManual] = useState(false)
  const [complaint, setComplaint] = useState('')
  const [c, setC] = useState({ full_name: '', phone: '', city: '', address: '' })
  const [product, setProduct] = useState(0)
  const products = useGet<any[]>(manual ? '/lookup/products' : null)
  const find = async () => { try { setLk(await get('/tech/guarantees/lookup' + qs({ code }))) } catch (e) { toast.error(e) } }
  const create = async (body: any) => { const r = await post('/tech/repairs', body); inv('/tech'); nav(`/tech/jobs/${r.id}`, { replace: true }) }
  return (
    <div className="space-y-4">
      <PageHeader title="تعمیر جدید" />
      {!manual && (
        <>
          <Card className="space-y-3 p-4">
            <Field label="کد گارانتی دستگاه"><div className="flex gap-2"><input className="input text-lg" dir="ltr" autoFocus value={code} onChange={(e) => { setCode(e.target.value); setLk(null) }} onKeyDown={(e) => e.key === 'Enter' && find()} /><button className="btn-primary" onClick={find} aria-label="جستجو"><Search className="h-5 w-5" /></button></div></Field>
          </Card>
          {lk && !lk.found && <Card className="space-y-3 p-4"><p className="text-sm text-bad">گارانتی با این کد پیدا نشد.</p><button className="btn-ghost w-full" onClick={() => setManual(true)}>ثبت تعمیر بدون گارانتی (دستی)</button></Card>}
          {lk?.found && (
            <Card className="space-y-3 p-4">
              <div className="flex items-center justify-between"><b>{lk.guarantee.customer_name}</b>{lk.guarantee.expired ? <Badge tone="warn">خارج از گارانتی</Badge> : <Badge tone="ok">داخل گارانتی ({fa(lk.guarantee.days_remaining)} روز)</Badge>}</div>
              <KV k="محصول" v={lk.guarantee.product_name} /><KV k="موبایل" v={<a className="text-brand" href={`tel:${lk.guarantee.customer_phone}`} dir="ltr">{lk.guarantee.customer_phone}</a>} /><KV k="شهر" v={lk.guarantee.customer_city} />
              {lk.guarantee.previous_repairs > 0 && <p className="rounded-lg bg-warn/10 p-2 text-xs text-warn">این دستگاه قبلاً {fa(lk.guarantee.previous_repairs)} بار تعمیر شده است.</p>}
              {lk.guarantee.expired && <p className="rounded-lg bg-warn/10 p-2 text-xs text-warn">هزینه تعمیر از مشتری دریافت می‌شود؛ می‌توانید ادامه دهید.</p>}
              {lk.guarantee.status === 'pending' && <p className="rounded-lg bg-bad/10 p-2 text-xs text-bad">این گارانتی هنوز تایید نشده است.</p>}
              <Field label="مشکل دستگاه"><textarea className="input" rows={3} value={complaint} onChange={(e) => setComplaint(e.target.value)} /></Field>
              <ActionButton className="btn-primary min-h-[52px] w-full text-base" onClick={() => create({ guarantee_code: code, complaint })}>شروع تعمیر</ActionButton>
            </Card>
          )}
        </>
      )}
      {manual && (
        <Card className="space-y-3 p-4">
          <p className="text-sm text-soft">دستگاه بدون گارانتی — هزینه از مشتری دریافت می‌شود.</p>
          <Field label="نام مشتری"><input className="input" value={c.full_name} onChange={(e) => setC({ ...c, full_name: e.target.value })} /></Field>
          <Field label="موبایل"><NumInput value={c.phone} onChange={(v) => setC({ ...c, phone: v })} /></Field>
          <Field label="شهر"><input className="input" value={c.city} onChange={(e) => setC({ ...c, city: e.target.value })} /></Field>
          <Field label="محصول"><select className="input" value={product} onChange={(e) => setProduct(+e.target.value)}><option value={0}>انتخاب…</option>{products.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></Field>
          <Field label="مشکل دستگاه"><textarea className="input" rows={3} value={complaint} onChange={(e) => setComplaint(e.target.value)} /></Field>
          <ActionButton className="btn-primary w-full" onClick={() => create({ customer: c, product_id: product, complaint })}>شروع تعمیر</ActionButton>
          <button className="w-full text-sm text-soft" onClick={() => setManual(false)}>بازگشت به جستجوی گارانتی</button>
        </Card>
      )}
    </div>
  )
}

export function Job() {
  const { id } = useParams()
  const q = useGet<any>(`/tech/repairs/${id}`)
  const inv = useInvalidate()
  const toast = useToast()
  const [pick, setPick] = useState<'part' | 'service' | 'request' | null>(null)
  const [report, setReport] = useState<string | null>(null)
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} />
  const r = q.data
  const edit = ['assigned', 'in_progress', 'waiting_parts'].includes(r.status)
  const refresh = () => inv('/tech')
  const run = (fn: () => Promise<any>, ok?: string) => async () => { try { await fn(); refresh(); ok && toast.ok(ok) } catch (e) { toast.error(e) } }
  const toReturn = r.parts.filter((p: any) => p.return_state === 'pending')
  return (
    <div className="space-y-4 pb-24">
      <div className="flex items-start justify-between gap-2"><div><h1 className="text-xl font-bold">{r.customer_name}</h1><div className="text-sm text-soft">{r.product_name} · <span className="num">{r.number}</span></div></div><div className="flex flex-col items-end gap-1"><StatusBadge map={repairStatus} value={r.status} />{r.guarantee_was_expired && <Badge tone="warn">خارج از گارانتی</Badge>}</div></div>
      <Card className="flex items-center justify-between gap-3 p-4"><div className="text-sm"><div>{r.customer_city} {r.customer_address}</div><div className="num text-soft" dir="ltr">{r.customer_phone}</div></div><a href={`tel:${r.customer_phone}`} className="btn-primary" aria-label="تماس"><Phone className="h-5 w-5" /></a></Card>
      {r.status === 'rejected' && <Card className="space-y-2 border-bad/40 bg-bad/5 p-4"><b className="text-bad">گزارش شما رد شد</b><p className="text-sm">{r.review_notes}</p><ActionButton className="btn-primary w-full" onClick={run(() => post(`/tech/repairs/${id}/reopen`))}>اصلاح و ارسال دوباره</ActionButton></Card>}
      {r.status === 'assigned' && <ActionButton className="btn-primary min-h-[52px] w-full text-base" onClick={run(() => post(`/tech/repairs/${id}/start`))}>شروع تعمیر</ActionButton>}
      <Card className="p-4"><div className="label">شکایت مشتری</div><p className="text-sm">{r.complaint || '—'}</p></Card>

      <Section title="قطعات مصرف‌شده" action={edit && <button className="btn-soft btn-sm" onClick={() => setPick('part')}><Plus className="h-4 w-4" />قطعه</button>}>
        {!r.parts.length ? <p className="p-4 text-sm text-soft">قطعه‌ای ثبت نشده است.</p> : r.parts.map((p: any) => (
          <div key={p.id} className="flex items-center justify-between gap-3 border-t border-line/60 p-4 text-sm">
            <div><b>{p.name}</b> × {fa(p.qty)}<div className="mt-1"><StatusBadge map={returnState} value={p.return_state} /></div></div>
            {edit && !p.shipment_id && <button className="p-2 text-bad" aria-label="حذف" onClick={run(() => del(`/tech/repairs/${id}/parts/${p.id}`))}><Trash2 className="h-5 w-5" /></button>}
          </div>))}
      </Section>
      <Section title="خدمات / اجرت" action={edit && <button className="btn-soft btn-sm" onClick={() => setPick('service')}><Plus className="h-4 w-4" />خدمت</button>}>
        {!r.services.length ? <p className="p-4 text-sm text-soft">خدمتی ثبت نشده است.</p> : r.services.map((s: any) => (
          <div key={s.id} className="flex items-center justify-between gap-3 border-t border-line/60 p-4 text-sm"><span>{s.name} × {fa(s.qty)}</span><span className="flex items-center gap-2 num">{toman(s.fee * s.qty)}{edit && <button className="p-1 text-bad" aria-label="حذف" onClick={run(() => del(`/tech/repairs/${id}/services/${s.id}`))}><Trash2 className="h-4 w-4" /></button>}</span></div>))}
      </Section>
      <Section title="درخواست قطعه از دفتر" action={edit && <button className="btn-soft btn-sm" onClick={() => setPick('request')}><PackagePlus className="h-4 w-4" />درخواست</button>}>
        {!r.requests.length ? <p className="p-4 text-sm text-soft">درخواستی ثبت نشده است.</p> : r.requests.map((x: any) => (
          <div key={x.id} className="border-t border-line/60 p-4 text-sm"><div className="flex items-center justify-between"><b className="num">{x.number}</b><StatusBadge map={requestStatus} value={x.status} /></div><div className="text-soft">{x.items.map((i: any) => `${i.name} × ${i.qty}`).join('، ')}</div>{x.review_notes && <div className="text-xs text-bad">{x.review_notes}</div>}</div>))}
      </Section>
      <Section title="عکس‌ها" action={edit && <label className="btn-soft btn-sm cursor-pointer"><Camera className="h-4 w-4" />عکس<input type="file" accept="image/*" capture="environment" className="hidden" onChange={async (e) => { const f = e.target.files?.[0]; if (!f) return; try { const u = await upload(f, 'repair'); await post(`/tech/repairs/${id}/photos`, { file: u.url }); refresh() } catch (x) { toast.error(x) } }} /></label>}>
        {!!r.photos.length && <div className="flex flex-wrap gap-2 border-t border-line/60 p-4">{r.photos.map((p: any) => <img key={p.id} src={p.file} alt="" className="h-20 w-20 rounded-xl object-cover" />)}</div>}
      </Section>
      <Card className="p-4">
        <Field label="گزارش تعمیر">
          <textarea className="input" rows={4} disabled={!edit} placeholder="چه مشکلی بود و چه کاری انجام دادید؟" value={report ?? r.report} onChange={(e) => setReport(e.target.value)} onBlur={async () => { if (report !== null && report !== r.report) { await patch(`/tech/repairs/${id}`, { report }).catch((x) => toast.error(x)); refresh() } }} />
        </Field>
      </Card>

      {edit && (
        <div className="fixed inset-x-0 bottom-[calc(58px+env(safe-area-inset-bottom))] z-20 border-t border-line bg-surface/95 p-3 backdrop-blur"><div className="mx-auto max-w-2xl">
          {toReturn.length > 0 && <p className="mb-2 text-center text-xs text-soft">{fa(toReturn.length)} قطعه قدیمی پس از تایید باید به دفتر ارسال شود.</p>}
          <ActionButton className="btn-primary min-h-[52px] w-full text-base" confirm="گزارش برای دفتر ارسال شود؟ قطعات مصرف‌شده از انبار شما کسر می‌شود." onClick={async () => { if (report !== null && report !== r.report) await patch(`/tech/repairs/${id}`, { report }); await post(`/tech/repairs/${id}/submit`); refresh() }} success="گزارش ارسال شد"><CheckCircle2 className="h-5 w-5" />ارسال گزارش به دفتر</ActionButton>
        </div></div>
      )}
      {r.status === 'submitted' && <Card className="p-4 text-center text-sm text-soft">گزارش شما برای بررسی ارسال شد. پس از تایید می‌توانید قطعات قدیمی را بفرستید.</Card>}
      {['approved'].includes(r.status) && toReturn.length > 0 && <Link to="/tech/returns" className="btn-primary w-full">ارسال {fa(toReturn.length)} قطعه قدیمی</Link>}

      <PartPicker open={pick === 'part'} onClose={() => setPick(null)} repairId={+id!} onAdded={refresh} />
      <ServicePicker open={pick === 'service'} onClose={() => setPick(null)} repairId={+id!} onAdded={refresh} />
      <RequestDialog open={pick === 'request'} onClose={() => setPick(null)} repairId={+id!} onDone={refresh} product={r.product_id} />
    </div>
  )
}

function Section({ title, action, children }: { title: string; action?: React.ReactNode; children?: React.ReactNode }) {
  return <Card><div className="flex items-center justify-between p-4"><h2 className="font-bold">{title}</h2>{action}</div>{children}</Card>
}

function PartPicker({ open, onClose, repairId, onAdded }: { open: boolean; onClose: () => void; repairId: number; onAdded: () => void }) {
  const [q, setQ] = useState('')
  const toast = useToast()
  const stock = useGet<any[]>(open ? '/tech/stock' : null)
  const mine = useMemo(() => new Map((stock.data ?? []).map((s) => [s.part_id, s.qty])), [stock.data])
  const parts = useGet<any[]>(open ? '/lookup/parts' + qs({ q }) : null)
  const [custom, setCustom] = useState('')
  const [reqFor, setReqFor] = useState<any>(null)
  const add = async (body: any) => {
    try { await post(`/tech/repairs/${repairId}/parts`, body); onAdded(); onClose(); toast.ok('قطعه ثبت شد') }
    catch (e) { if (e instanceof ApiError && e.code === 'not_in_stock') setReqFor({ id: body.part_id, msg: e.message }); else toast.error(e) }
  }
  const list = [...(parts.data ?? [])].sort((a, b) => (mine.get(b.id) ? 1 : 0) - (mine.get(a.id) ? 1 : 0))
  return (
    <Modal open={open} onClose={onClose} title="کدام قطعه تعویض شد؟">
      <input className="input mb-3" autoFocus placeholder="جستجوی قطعه…" value={q} onChange={(e) => setQ(e.target.value)} />
      {reqFor && <div className="mb-3 rounded-xl bg-warn/10 p-3 text-sm text-warn">{reqFor.msg}<ActionButton className="btn-primary btn-sm mt-2 w-full" onClick={async () => { await post('/tech/part-requests', { repair_id: repairId, items: [{ kind: 'part', part_id: reqFor.id, qty: 1 }] }); setReqFor(null); onAdded(); onClose() }} success="درخواست برای دفتر ارسال شد">همین قطعه را درخواست کن</ActionButton></div>}
      <div className="max-h-[45vh] space-y-1 overflow-y-auto">
        {parts.isLoading ? <Loading /> : list.map((p) => {
          const n = mine.get(p.id) ?? 0
          return <button key={p.id} className="flex w-full items-center justify-between gap-3 rounded-xl p-3 text-start hover:bg-muted active:bg-muted" onClick={() => add({ part_id: p.id, qty: 1 })}>
            <span><b>{p.name}</b>{p.variant && <span className="text-soft"> — {p.variant}</span>}<span className="block text-xs text-soft" dir="ltr">{p.code}</span></span>
            {n > 0 ? <Badge tone="ok">{fa(n)} عدد دارید</Badge> : <Badge>ندارید</Badge>}</button>
        })}
      </div>
      <div className="mt-4 flex gap-2 border-t border-line pt-4"><input className="input" placeholder="قطعه‌ای که در فهرست نیست…" value={custom} onChange={(e) => setCustom(e.target.value)} /><button className="btn-ghost" disabled={!custom.trim()} onClick={() => add({ custom_name: custom, qty: 1 })}>ثبت</button></div>
    </Modal>
  )
}

function ServicePicker({ open, onClose, repairId, onAdded }: { open: boolean; onClose: () => void; repairId: number; onAdded: () => void }) {
  const s = useGet<any[]>(open ? '/lookup/service-items' : null)
  const toast = useToast()
  return (
    <Modal open={open} onClose={onClose} title="چه خدمتی انجام شد؟">
      {s.isLoading ? <Loading /> : s.data?.map((x) => (
        <button key={x.id} className="flex w-full items-center justify-between rounded-xl p-3 text-start hover:bg-muted" onClick={async () => { try { await post(`/tech/repairs/${repairId}/services`, { service_id: x.id, qty: 1 }); onAdded(); onClose() } catch (e) { toast.error(e) } }}><b>{x.name}</b><span className="text-sm text-soft num">{x.fee ? toman(x.fee) : ''}</span></button>
      ))}
    </Modal>
  )
}

function RequestDialog({ open, onClose, repairId, onDone, product }: { open: boolean; onClose: () => void; repairId: number | null; onDone: () => void; product?: number }) {
  const [q, setQ] = useState('')
  const [items, setItems] = useState<{ part_id?: number; name: string; qty: number; kind: string; custom_name?: string }[]>([])
  const [notes, setNotes] = useState('')
  const parts = useGet<any[]>(open ? '/lookup/parts' + qs({ q, product_id: q ? undefined : product }) : null)
  const [custom, setCustom] = useState('')
  const add = (p: any) => setItems((x) => (x.find((i) => i.part_id === p.id) ? x.map((i) => (i.part_id === p.id ? { ...i, qty: i.qty + 1 } : i)) : [...x, { part_id: p.id, name: p.name, qty: 1, kind: 'part' }]))
  return (
    <Modal open={open} onClose={onClose} title="درخواست قطعه از دفتر" footer={<ActionButton disabled={!items.length} onClick={async () => { await post('/tech/part-requests', { repair_id: repairId, notes, items: items.map((i) => ({ kind: i.kind, part_id: i.part_id, custom_name: i.custom_name, qty: i.qty })) }); setItems([]); setNotes(''); onDone(); onClose() }} success="درخواست ارسال شد">ارسال درخواست</ActionButton>}>
      {!!items.length && <div className="mb-3 space-y-1">{items.map((i, k) => <div key={k} className="flex items-center gap-2 rounded-xl bg-brand-tint p-2 text-sm"><span className="flex-1 font-medium">{i.name}</span><button className="p-1.5" onClick={() => setItems(items.map((x, j) => (j === k ? { ...x, qty: Math.max(1, x.qty - 1) } : x)))}><Minus className="h-4 w-4" /></button><b className="w-5 text-center num">{fa(i.qty)}</b><button className="p-1.5" onClick={() => setItems(items.map((x, j) => (j === k ? { ...x, qty: x.qty + 1 } : x)))}><Plus className="h-4 w-4" /></button><button className="p-1.5 text-bad" onClick={() => setItems(items.filter((_, j) => j !== k))}><Trash2 className="h-4 w-4" /></button></div>)}</div>}
      <input className="input mb-2" placeholder={product ? 'قطعات سازگار با این دستگاه — جستجو…' : 'جستجوی قطعه…'} value={q} onChange={(e) => setQ(e.target.value)} />
      <div className="max-h-48 space-y-1 overflow-y-auto">{parts.data?.map((p) => <button key={p.id} className="flex w-full items-center justify-between rounded-xl p-2.5 text-start text-sm hover:bg-muted" onClick={() => add(p)}><span>{p.name}{p.variant && ` — ${p.variant}`}</span><Plus className="h-4 w-4 text-brand" /></button>)}</div>
      <div className="mt-3 flex gap-2"><input className="input" placeholder="قطعه خارج از فهرست…" value={custom} onChange={(e) => setCustom(e.target.value)} /><button className="btn-ghost" disabled={!custom.trim()} onClick={() => { setItems([...items, { name: custom, qty: 1, kind: 'custom', custom_name: custom }]); setCustom('') }}>افزودن</button></div>
      <Field label="توضیح" className="mt-3"><input className="input" value={notes} onChange={(e) => setNotes(e.target.value)} /></Field>
    </Modal>
  )
}

export function TechStock() {
  const s = useGet<any[]>('/tech/stock')
  const reqs = useGet<any[]>('/tech/part-requests')
  const [dlg, setDlg] = useState(false)
  const inv = useInvalidate()
  const toast = useToast()
  return (
    <div className="space-y-4">
      <PageHeader title="انبار من" subtitle="قطعاتی که دفتر به شما تحویل داده است" actions={<button className="btn-primary btn-sm" onClick={() => setDlg(true)}><PackagePlus className="h-4 w-4" />درخواست قطعه</button>} />
      <Card>{s.isLoading ? <Loading /> : s.error ? <Blocked error={s.error} /> : !s.data?.length ? <Empty title="انبار شما خالی است" hint="با «درخواست قطعه» از دفتر قطعه بگیرید." /> : s.data.map((p) => (
        <div key={p.part_id} className="flex items-center justify-between border-b border-line/60 p-4 last:border-0"><div><b>{p.name}</b>{p.variant && <span className="text-soft"> — {p.variant}</span>}<div className="text-xs text-soft" dir="ltr">{p.code}</div></div><span className="text-xl font-extrabold num">{fa(p.qty)}</span></div>))}</Card>
      <h2 className="font-bold">درخواست‌های من</h2>
      <div className="space-y-2">{reqs.data?.slice(0, 15).map((x) => (
        <Card key={x.id} className="p-4 text-sm"><div className="flex items-center justify-between"><b className="num">{x.number}</b><StatusBadge map={requestStatus} value={x.status} /></div><div className="text-soft">{x.items.map((i: any) => `${i.name} × ${i.qty}`).join('، ')}</div>
          {['pending', 'approved'].includes(x.status) && <button className="mt-2 text-xs text-bad" onClick={async () => { if (confirm('درخواست لغو شود؟')) { try { await post(`/tech/part-requests/${x.id}/cancel`); inv('/tech') } catch (e) { toast.error(e) } } }}>لغو درخواست</button>}</Card>))}</div>
      <RequestDialog open={dlg} onClose={() => setDlg(false)} repairId={null} onDone={() => inv('/tech')} />
    </div>
  )
}

export function Returns() {
  const [tab, setTab] = useState<'send' | 'sent'>('send')
  const ret = useGet<any[]>('/tech/returns')
  const ships = useGet<any[]>('/tech/shipments')
  const dash = useGet<any>('/tech/dashboard')
  const [sel, setSel] = useState<number[]>([])
  const [dlg, setDlg] = useState(false)
  const [f, setF] = useState({ method: 'post', tracking_code: '', notes: '' })
  const [view, setView] = useState<number | null>(null)
  const one = useGet<any>(view ? `/tech/shipments/${view}` : null)
  const inv = useInvalidate()
  const all = ret.data?.map((r) => r.id) ?? []
  const chosen = sel.length ? sel : []
  return (
    <div className="space-y-4">
      <PageHeader title="ارسال قطعات قدیمی" subtitle="قطعه‌ای که تعویض کردید را برای دفتر بفرستید؛ بعد از دریافت، پولش به حساب شما می‌نشیند." />
      {dash.data && <div className="grid grid-cols-3 gap-2"><Card className="p-3 text-center"><div className="text-xl font-extrabold num">{fa(dash.data.parts_to_send)}</div><div className="text-[11px] text-soft">باید بفرستید</div></Card><Card className="p-3 text-center"><div className="text-xl font-extrabold num">{fa(dash.data.parcels_in_transit)}</div><div className="text-[11px] text-soft">در راه</div></Card><Card className="p-3 text-center"><div className="text-lg font-extrabold text-warn num">{toman(dash.data.awaiting_payment)}</div><div className="text-[11px] text-soft">در انتظار پرداخت</div></Card></div>}
      <Tabs value={tab} onChange={setTab} items={[{ key: 'send', label: 'آماده ارسال', count: ret.data?.length }, { key: 'sent', label: 'مرسوله‌ها' }]} />
      {tab === 'send' ? (
        <>
          {ret.isLoading ? <Loading /> : ret.error ? <Blocked error={ret.error} /> : !ret.data?.length ? <Empty title="قطعه‌ای برای ارسال ندارید 🎉" hint="قطعات پس از ارسال گزارش تعمیر اینجا ظاهر می‌شوند." /> : (
            <>
              <button className="text-sm font-semibold text-brand" onClick={() => setSel(sel.length === all.length ? [] : all)}>{sel.length === all.length ? 'لغو انتخاب همه' : 'انتخاب همه'}</button>
              <div className="space-y-2 pb-24">{ret.data.map((r) => (
                <label key={r.id} className={clsx('card flex cursor-pointer items-center gap-3 p-4', chosen.includes(r.id) && 'border-brand ring-1 ring-brand')}>
                  <input type="checkbox" className="h-5 w-5" checked={chosen.includes(r.id)} onChange={(e) => setSel(e.target.checked ? [...sel, r.id] : sel.filter((x) => x !== r.id))} />
                  <div className="flex-1 text-sm"><b>{r.name}</b> × {fa(r.qty)}<div className="text-xs text-soft">{r.customer_name} · {r.repair_number}</div></div>
                  {r.buyback_price > 0 && <span className="text-xs text-ok num">{toman(r.buyback_price * r.qty)}</span>}
                </label>))}</div>
              {chosen.length > 0 && <div className="fixed inset-x-0 bottom-[calc(58px+env(safe-area-inset-bottom))] z-20 border-t border-line bg-surface/95 p-3"><div className="mx-auto max-w-2xl"><button className="btn-primary min-h-[52px] w-full text-base" onClick={() => setDlg(true)}><Send className="h-5 w-5" />ارسال {fa(chosen.length)} قطعه</button></div></div>}
            </>
          )}
        </>
      ) : (
        <div className="space-y-2">{ships.isLoading ? <Loading /> : !ships.data?.length ? <Empty title="مرسوله‌ای ثبت نشده" /> : ships.data.map((s) => (
          <button key={s.id} className="card block w-full p-4 text-start" onClick={() => setView(s.id)}><div className="flex items-center justify-between"><b className="num">{s.number}</b><StatusBadge map={shipmentStatus} value={s.status} /></div><div className="mt-1 text-sm text-soft">{fa(s.part_count)} قطعه · {method[s.method]} · {date(s.sent_on)}</div>{s.invoice_total > 0 && <div className="mt-1 text-sm font-bold num">{toman(s.invoice_total)}</div>}</button>))}</div>
      )}
      <Modal open={dlg} onClose={() => setDlg(false)} title="ثبت مرسوله" footer={<ActionButton disabled={['post', 'courier'].includes(f.method) && !f.tracking_code.trim()} onClick={async () => { await post('/tech/shipments', { repair_part_ids: chosen, ...f }); setSel([]); setDlg(false); setTab('sent'); inv('/tech') }} success="مرسوله ثبت شد">ثبت</ActionButton>}>
        <div className="space-y-3"><Field label="روش ارسال"><div className="grid grid-cols-4 gap-1.5">{Object.entries(method).map(([k, l]) => <button key={k} className={clsx('rounded-xl border py-2.5 text-sm font-semibold', f.method === k ? 'border-brand bg-brand-tint text-brand' : 'border-line')} onClick={() => setF({ ...f, method: k })}>{l}</button>)}</div></Field>
          {['post', 'courier'].includes(f.method) && <Field label="کد رهگیری"><input className="input" dir="ltr" value={f.tracking_code} onChange={(e) => setF({ ...f, tracking_code: e.target.value })} /></Field>}
          <Field label="توضیح"><input className="input" value={f.notes} onChange={(e) => setF({ ...f, notes: e.target.value })} /></Field></div>
      </Modal>
      <Modal open={!!view} onClose={() => setView(null)} title={one.data ? `مرسوله ${one.data.number}` : 'مرسوله'} footer={one.data?.status === 'sent' ? <ActionButton className="btn-ghost text-bad" confirm="مرسوله لغو شود؟" onClick={async () => { await post(`/tech/shipments/${view}/cancel`); inv('/tech'); setView(null) }}>لغو مرسوله</ActionButton> : undefined}>
        {one.isLoading || !one.data ? <Loading /> : <><div className="mb-3"><StatusBadge map={shipmentStatus} value={one.data.status} /></div>
          {one.data.parts.map((p: any) => <div key={p.id} className="flex justify-between border-b border-line/60 py-2 text-sm"><span>{p.name} × {fa(p.qty)}</span><span className="num">{p.buyback_price ? toman(p.buyback_price * p.qty) : '—'}</span></div>)}
          {one.data.invoice_total > 0 && <div className="mt-3 flex justify-between font-bold"><span>جمع</span><span className="num">{toman(one.data.invoice_total)}</span></div>}
          {one.data.review_notes && <p className="mt-3 rounded-lg bg-bad/10 p-2 text-sm text-bad">{one.data.review_notes}</p>}</>}
      </Modal>
    </div>
  )
}

export function TechAccount() {
  const led = useGet<any>('/tech/ledger')
  const prof = useGet<any>('/tech/profile')
  const [pw, setPw] = useState({ cur: '', np: '' })
  return (
    <div className="space-y-4">
      <PageHeader title="حساب من" />
      <Card className="p-5 text-center"><div className="text-xs text-soft">مانده حساب (طلب شما از اوینکی)</div><div className={clsx('text-3xl font-extrabold num', (led.data?.balance ?? 0) > 0 && 'text-ok')}>{toman(led.data?.balance ?? 0)}</div></Card>
      <Card>{led.isLoading ? <Loading /> : !led.data?.entries?.length ? <Empty title="تراکنشی نیست" /> : led.data.entries.map((e: any) => (
        <div key={e.id} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0"><div><b>{ledgerKind[e.kind]}</b><div className="text-xs text-soft">{e.note} · {ago(e.created_at)}</div></div><b className={clsx('num', e.amount < 0 ? 'text-warn' : 'text-ok')} dir="ltr">{e.amount > 0 ? '+' : ''}{fa(e.amount)}</b></div>))}</Card>
      {prof.data && <Card className="p-4"><KV k="نام" v={prof.data.full_name} /><KV k="موبایل" v={<span dir="ltr">{prof.data.phone}</span>} /><KV k="شهر" v={`${prof.data.province} ${prof.data.city}`} /></Card>}
      <Card className="space-y-3 p-4"><b>تغییر گذرواژه</b><input className="input" type="password" placeholder="گذرواژه فعلی" value={pw.cur} onChange={(e) => setPw({ ...pw, cur: e.target.value })} /><input className="input" type="password" placeholder="گذرواژه جدید" value={pw.np} onChange={(e) => setPw({ ...pw, np: e.target.value })} />
        <ActionButton className="btn-soft w-full" success="گذرواژه تغییر کرد؛ دوباره وارد شوید" onClick={async () => { await post('/auth/change-password', { current_password: pw.cur, new_password: pw.np }); setTimeout(() => (location.href = '/login'), 1200) }}>ذخیره</ActionButton></Card>
    </div>
  )
}

void dateTime; void ImageUpload
