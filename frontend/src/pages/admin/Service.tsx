import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { AlertTriangle, Plus } from 'lucide-react'
import { ActionButton, Badge, Card, DataTable, Empty, ErrorBox, Field, KV, Loading, Modal, MoneyInput, NumInput, PageHeader, SearchBox, StatusBadge, Tabs } from '../../components/ui'
import { patch, post } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { ago, date, dateTime, fa, toman } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { ledgerKind, method, repairStatus, requestStatus, returnState, shipmentStatus, stageLabel, stageOrder, techStatus } from '../../lib/labels'

// ───────── pipeline: every case, in the stage it is waiting in ─────────
export function Pipeline() {
  const [stage, setStage] = useState('')
  const [q, setQ] = useState('')
  const nav = useNavigate()
  const d = useGet<any>('/admin/pipeline' + qs({ stage, q }), { refetchInterval: 60_000 })
  const counts = d.data?.counts ?? {}
  const open = stageOrder.filter((s) => !['complete', 'closed'].includes(s)).reduce((a, s) => a + (counts[s] ?? 0), 0)
  return (
    <div>
      <PageHeader title="پیگیری پرونده‌های خدمات" subtitle={`${fa(open)} پرونده باز — هر پرونده در مرحله‌ای که منتظر آن است نمایش داده می‌شود`} />
      <Tabs value={stage} onChange={setStage} items={[{ key: '', label: 'همه' }, ...stageOrder.map((s) => ({ key: s, label: stageLabel[s], count: counts[s] }))]} />
      <Card>
        <div className="border-b border-line p-3"><SearchBox value={q} onChange={setQ} placeholder="شماره پرونده، مشتری یا کد گارانتی" /></div>
        {d.isLoading ? <Loading /> : d.error ? <ErrorBox error={d.error} /> : !d.data.items.length ? <Empty title="پرونده‌ای در این مرحله نیست" /> : (
          <DataTable rows={d.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => nav(`/admin/repairs/${r.id}`)} cols={[
            { head: 'پرونده', cell: (r: any) => <div><b className="num">{r.number}</b>{r.guarantee_was_expired && <Badge tone="warn">خارج از گارانتی</Badge>}</div> },
            { head: 'مشتری', cell: (r: any) => <div>{r.customer_name}<div className="text-xs text-soft">{r.customer_city}</div></div> }, { head: 'محصول', cell: (r: any) => r.product_name || '—' },
            { head: 'تکنسین', cell: (r: any) => r.technician_name || <Badge tone="warn">تعیین نشده</Badge> },
            { head: 'مرحله', cell: (r: any) => <div><Badge tone={['complete'].includes(r.stage) ? 'ok' : r.stage === 'closed' ? 'mute' : 'info'}>{stageLabel[r.stage]}</Badge>{r.needs_attention && <span className="ms-1 inline-flex items-center gap-1 text-xs text-bad"><AlertTriangle className="h-3.5 w-3.5" />{fa(r.days_in_stage)} روز</span>}</div> },
            { head: 'آخرین تغییر', cell: (r: any) => <span className="text-xs text-soft">{ago(r.stage_since)}</span> }]} />
        )}
      </Card>
    </div>
  )
}

export function RepairDetail() {
  const { id } = useParams()
  const q = useGet<any>(`/admin/repairs/${id}`)
  const can = useCan()
  const inv = useInvalidate()
  const [review, setReview] = useState<'approve' | 'reject' | null>(null)
  const [notes, setNotes] = useState('')
  const [assign, setAssign] = useState(false)
  const [tech, setTech] = useState(0)
  const techs = useGet<any[]>(assign ? '/admin/technicians?status=approved' : null)
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} />
  const r = q.data
  const manage = can('service.manage')
  return (
    <div className="space-y-4">
      <PageHeader title={`پرونده ${r.number}`} subtitle={`ثبت: ${dateTime(r.created_at)}`} actions={<><StatusBadge map={repairStatus} value={r.status} />{r.guarantee_was_expired && <Badge tone="warn">خارج از گارانتی</Badge>}<Link to="/admin/service" className="btn-ghost btn-sm">بازگشت</Link></>} />
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="p-4 lg:col-span-1"><KV k="مشتری" v={r.customer_name} /><KV k="موبایل" v={<a href={`tel:${r.customer_phone}`} dir="ltr" className="num text-brand">{r.customer_phone}</a>} /><KV k="شهر" v={r.customer_city} /><KV k="نشانی" v={r.customer_address} /><KV k="محصول" v={r.product_name} />
          <KV k="گارانتی" v={r.guarantee_id ? <Link className="text-brand" to={`/admin/guarantees/${r.guarantee_id}`} dir="ltr">{r.guarantee_code}</Link> : 'بدون گارانتی'} />{r.guarantee_expiry && <KV k="پایان گارانتی" v={date(r.guarantee_expiry)} />}
          <KV k="تکنسین" v={r.technician_id ? <Link className="text-brand" to={`/admin/technicians/${r.technician_id}`}>{r.technician_name}</Link> : '—'} /></Card>
        <Card className="space-y-3 p-4 lg:col-span-2">
          <div><div className="label">شکایت مشتری</div><p className="text-sm">{r.complaint || '—'}</p></div>
          {r.diagnosis && <div><div className="label">عیب‌یابی</div><p className="text-sm">{r.diagnosis}</p></div>}
          <div><div className="label">گزارش تکنسین</div><p className="whitespace-pre-line text-sm">{r.report || '—'}</p></div>
          {r.customer_charge > 0 && <div className="rounded-xl bg-warn/10 p-3 text-sm"><b>هزینه قابل دریافت از مشتری: {toman(r.customer_charge)}</b> {r.customer_paid ? <Badge tone="ok">دریافت شد</Badge> : manage && <ActionButton className="btn-soft btn-sm ms-2" onClick={async () => { await post(`/admin/repairs/${id}/customer-paid`); inv('/admin/repairs') }}>ثبت دریافت</ActionButton>}</div>}
          {r.review_notes && <div className="rounded-xl bg-muted p-3 text-sm"><b>نتیجه بررسی:</b> {r.review_notes}</div>}
        </Card>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card><div className="border-b border-line p-4 font-bold">قطعات مصرفی</div>
          {!r.parts.length ? <Empty title="قطعه‌ای ثبت نشده" /> : r.parts.map((p: any) => (
            <div key={p.id} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0"><div><b>{p.name}</b> × {fa(p.qty)}{p.report && <div className="text-xs text-soft">{p.report}</div>}</div><div className="flex flex-col items-end gap-1"><StatusBadge map={returnState} value={p.return_state} />{p.shipment_id && <Link className="text-xs text-brand" to={`/admin/shipments/${p.shipment_id}`}>مرسوله</Link>}</div></div>
          ))}
        </Card>
        <Card><div className="border-b border-line p-4 font-bold">خدمات / اجرت</div>
          {!r.services.length ? <Empty title="خدمتی ثبت نشده" /> : r.services.map((s: any) => <div key={s.id} className="flex justify-between border-b border-line/60 p-4 text-sm last:border-0"><span>{s.name} × {fa(s.qty)}</span><span className="num">{toman(s.fee * s.qty)}</span></div>)}
        </Card>
      </div>
      {!!r.requests.length && <Card><div className="border-b border-line p-4 font-bold">درخواست‌های قطعه</div>{r.requests.map((x: any) => <Link key={x.id} to={`/admin/part-requests?open=${x.id}`} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0"><span><b className="num">{x.number}</b> {x.items.map((i: any) => `${i.name}×${i.qty}`).join('، ')}</span><StatusBadge map={requestStatus} value={x.status} /></Link>)}</Card>}
      {!!r.photos.length && <Card className="flex flex-wrap gap-2 p-4">{r.photos.map((p: any) => <a key={p.id} href={p.file} target="_blank"><img src={p.file} alt={p.caption} className="h-24 w-24 rounded-xl object-cover" /></a>)}</Card>}
      {manage && (
        <Card className="flex flex-wrap gap-2 p-4">
          {r.status === 'submitted' && <><button className="btn-primary" onClick={() => setReview('approve')}>تایید گزارش</button><button className="btn-ghost text-bad" onClick={() => setReview('reject')}>رد گزارش</button></>}
          {['requested', 'assigned', 'in_progress', 'waiting_parts'].includes(r.status) && <button className="btn-soft" onClick={() => setAssign(true)}>{r.technician_id ? 'تغییر تکنسین' : 'تخصیص تکنسین'}</button>}
        </Card>
      )}
      <Modal open={!!review} onClose={() => setReview(null)} title={review === 'approve' ? 'تایید گزارش تعمیر' : 'رد گزارش تعمیر'} footer={<ActionButton className={review === 'approve' ? 'btn-primary' : 'btn-danger'} onClick={async () => { await post(`/admin/repairs/${id}/review`, { decision: review, notes }); inv('/admin'); setReview(null) }}>{review === 'approve' ? 'تایید' : 'رد'}</ActionButton>}>
        {review === 'approve' && <p className="mb-3 text-sm text-soft">با تایید، اجرت تکنسین (در تعمیرات داخل گارانتی) به حساب او اضافه و نظرسنجی برای مشتری ارسال می‌شود.</p>}
        <Field label={review === 'reject' ? 'دلیل رد (الزامی)' : 'توضیح (اختیاری)'}><textarea className="input" rows={3} value={notes} onChange={(e) => setNotes(e.target.value)} /></Field>
      </Modal>
      <Modal open={assign} onClose={() => setAssign(false)} title="تخصیص تکنسین" footer={<ActionButton disabled={!tech} onClick={async () => { await post(`/admin/repairs/${id}/assign`, { technician_id: tech }); inv('/admin'); setAssign(false) }}>تخصیص</ActionButton>}>
        <select className="input" value={tech} onChange={(e) => setTech(+e.target.value)}><option value={0}>انتخاب…</option>{techs.data?.map((t) => <option key={t.id} value={t.id}>{t.full_name} — {t.city} ({fa(t.open_repairs)} تعمیر باز)</option>)}</select>
      </Modal>
    </div>
  )
}

// ───────── part requests ─────────
export function PartRequests() {
  const [status, setStatus] = useState('pending')
  const [view, setView] = useState<number | null>(null)
  const list = useGet<any[]>('/admin/part-requests' + qs({ status }))
  const one = useGet<any>(view ? `/admin/part-requests/${view}` : null)
  const can = useCan()
  const inv = useInvalidate()
  const [notes, setNotes] = useState('')
  return (
    <div>
      <PageHeader title="درخواست قطعه" subtitle="تحویل، قطعه را از انبار مرکزی به انبار شخصی تکنسین منتقل می‌کند." />
      <Tabs value={status} onChange={setStatus} items={[{ key: 'pending', label: 'در انتظار' }, { key: 'delivered', label: 'تحویل شده' }, { key: 'not_delivered', label: 'تحویل نشد' }, { key: 'cancelled', label: 'لغو شده' }, { key: '', label: 'همه' }]} />
      <Card>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data?.length ? <Empty title="درخواستی نیست" /> : (
          <DataTable rows={list.data} rowKey={(r: any) => r.id} onRow={(r: any) => setView(r.id)} cols={[{ head: 'شماره', cell: (r: any) => <b className="num">{r.number}</b> }, { head: 'تکنسین', cell: (r: any) => r.technician_name }, { head: 'پرونده', cell: (r: any) => r.repair_number || '—' },
            { head: 'اقلام', cell: (r: any) => fa(r.item_count) }, { head: 'تاریخ', cell: (r: any) => ago(r.created_at) }, { head: 'وضعیت', cell: (r: any) => <div className="flex gap-1"><StatusBadge map={requestStatus} value={r.status} />{r.guarantee_was_expired && <Badge tone="warn">خارج از گارانتی</Badge>}</div> }]} />
        )}
      </Card>
      <Modal open={!!view} onClose={() => setView(null)} title={one.data ? `درخواست ${one.data.number}` : 'درخواست'} wide
        footer={one.data && can('service.manage') && ['pending', 'approved', 'not_delivered'].includes(one.data.status) ? <>
          <ActionButton className="btn-ghost text-bad" onClick={async () => { await post(`/admin/part-requests/${view}/not-delivered`, { notes }); inv('/admin/part-requests'); setView(null) }}>تحویل نمی‌شود</ActionButton>
          <ActionButton onClick={async () => { await post(`/admin/part-requests/${view}/deliver`, { notes }); inv('/admin'); setView(null) }} success="قطعات به انبار تکنسین منتقل شد">تحویل به تکنسین</ActionButton></> : undefined}>
        {one.isLoading || !one.data ? <Loading /> : <>
          <KV k="تکنسین" v={one.data.technician_name} /><KV k="پرونده" v={one.data.repair_number ? <Link className="text-brand" to={`/admin/repairs/${one.data.repair_id}`}>{one.data.repair_number}</Link> : '—'} /><KV k="وضعیت" v={<StatusBadge map={requestStatus} value={one.data.status} />} />
          {one.data.notes && <KV k="توضیح تکنسین" v={one.data.notes} />}
          <div className="mb-1 mt-4 text-sm font-bold">اقلام</div>
          {one.data.items.map((i: any) => (
            <div key={i.id} className="flex items-center justify-between border-b border-line/60 py-2 text-sm"><span>{i.name} × {fa(i.qty)} <span className="text-xs text-soft">({{ part: 'قطعه', service: 'خدمت', custom: 'سفارشی' }[i.kind as string]})</span></span>
              {i.kind === 'part' && <span className={i.central_stock >= i.qty ? 'text-xs text-ok' : 'text-xs font-bold text-bad'}>موجودی انبار: {fa(i.central_stock)}</span>}</div>
          ))}
          {['pending', 'approved'].includes(one.data.status) && <Field label="یادداشت" className="mt-4"><input className="input" value={notes} onChange={(e) => setNotes(e.target.value)} /></Field>}
        </>}
      </Modal>
    </div>
  )
}

// ───────── shipments of old parts, invoice, payment ─────────
export function Shipments() {
  const [status, setStatus] = useState('sent')
  const nav = useNavigate()
  const d = useGet<any>('/admin/shipments' + qs({ status }))
  return (
    <div>
      <PageHeader title="مرسوله قطعات قدیمی و پرداخت" />
      {d.data && <div className="mb-4 grid grid-cols-3 gap-3"><Card className="p-4"><div className="text-xs text-soft">منتظر دریافت</div><div className="text-2xl font-bold num">{fa(d.data.summary.to_receive)}</div></Card><Card className="p-4"><div className="text-xs text-soft">قابل پرداخت</div><div className="text-xl font-bold text-warn num">{toman(d.data.summary.payable)}</div></Card><Card className="p-4"><div className="text-xs text-soft">پرداخت‌شده</div><div className="text-xl font-bold num">{toman(d.data.summary.paid)}</div></Card></div>}
      <Tabs value={status} onChange={setStatus} items={[{ key: 'sent', label: 'ارسال شده' }, { key: 'invoiced', label: 'منتظر پرداخت' }, { key: 'received', label: 'دریافت شده' }, { key: 'paid', label: 'پرداخت شده' }, { key: 'rejected', label: 'رد شده' }, { key: '', label: 'همه' }]} />
      <Card>
        {d.isLoading ? <Loading /> : !d.data.items.length ? <Empty title="مرسوله‌ای نیست" /> : (
          <DataTable rows={d.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => nav(`/admin/shipments/${r.id}`)} cols={[{ head: 'شماره', cell: (r: any) => <b className="num">{r.number}</b> }, { head: 'تکنسین', cell: (r: any) => r.technician_name }, { head: 'روش', cell: (r: any) => `${method[r.method]}${r.tracking_code ? ' · ' + r.tracking_code : ''}` },
            { head: 'قطعه', cell: (r: any) => fa(r.part_count) }, { head: 'صورتحساب', cell: (r: any) => (r.invoice_total ? <span className="num">{toman(r.invoice_total)}</span> : '—') }, { head: 'وضعیت', cell: (r: any) => <StatusBadge map={shipmentStatus} value={r.status} /> }]} />
        )}
      </Card>
    </div>
  )
}

export function ShipmentDetail() {
  const { id } = useParams()
  const q = useGet<any>(`/admin/shipments/${id}`)
  const can = useCan()
  const inv = useInvalidate()
  const [got, setGot] = useState<number[] | null>(null)
  const [prices, setPrices] = useState<Record<number, number>>({})
  const [ref, setRef] = useState('')
  const [reason, setReason] = useState('')
  const [dlg, setDlg] = useState<'receive' | 'pay' | 'reject' | null>(null)
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} />
  const s = q.data
  const manage = can('service.manage')
  const sel = got ?? s.parts.map((p: any) => p.id)
  return (
    <div className="space-y-4">
      <PageHeader title={`مرسوله ${s.number}`} subtitle={`${s.technician_name} · ${method[s.method]} ${s.tracking_code ? '· ' + s.tracking_code : ''} · ${date(s.sent_on)}`} actions={<><StatusBadge map={shipmentStatus} value={s.status} /><Link to="/admin/shipments" className="btn-ghost btn-sm">بازگشت</Link></>} />
      <Card>
        {s.parts.map((p: any) => (
          <div key={p.id} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0">
            <div><b>{p.name}</b> × {fa(p.qty)}<div className="text-xs text-soft">پرونده {p.repair_number}{p.condition_note && ` · ${p.condition_note}`}</div></div>
            <div className="flex items-center gap-2"><StatusBadge map={returnState} value={p.return_state} /><span className="num">{p.buyback_price ? toman(p.buyback_price * p.qty) : 'بدون قیمت'}</span></div>
          </div>
        ))}
        <div className="flex justify-between p-4 font-bold"><span>جمع صورتحساب</span><span className="num">{s.invoice_total ? toman(s.invoice_total) : '—'}</span></div>
      </Card>
      {s.receive_notes && <Card className="p-4 text-sm"><b>یادداشت دریافت:</b> {s.receive_notes}</Card>}
      {manage && (
        <Card className="flex flex-wrap gap-2 p-4">
          {s.status === 'sent' && <><button className="btn-primary" onClick={() => { setGot(null); setDlg('receive') }}>دریافت و صدور صورتحساب</button><button className="btn-ghost text-bad" onClick={() => setDlg('reject')}>رد مرسوله</button></>}
          {s.status === 'received' && <ActionButton onClick={async () => { await post(`/admin/shipments/${id}/invoice`, {}); inv('/admin') }}>صدور صورتحساب</ActionButton>}
          {s.status === 'invoiced' && <button className="btn-primary" onClick={() => setDlg('pay')}>ثبت پرداخت {toman(s.invoice_total)}</button>}
        </Card>
      )}
      <Modal open={dlg === 'receive'} onClose={() => setDlg(null)} wide title="دریافت مرسوله" footer={<ActionButton disabled={!sel.length} onClick={async () => { await post(`/admin/shipments/${id}/receive`, { received_repair_part_ids: sel, prices: Object.fromEntries(Object.entries(prices).map(([k, v]) => [k, v])) }); inv('/admin'); setDlg(null) }} success="دریافت و صورتحساب ثبت شد">ثبت دریافت و صورتحساب</ActionButton>}>
        <p className="mb-3 text-sm text-soft">قطعاتی که واقعاً داخل بسته بوده را علامت بزنید. قیمت‌ها از فهرست قیمت قطعات پر شده؛ در صورت نیاز تغییر دهید. قطعات نرسیده دوباره در فهرست «ارسال» تکنسین قرار می‌گیرند.</p>
        {s.parts.map((p: any) => (
          <div key={p.id} className="flex items-center gap-3 border-b border-line/60 py-2.5 text-sm">
            <input type="checkbox" className="h-4 w-4" checked={sel.includes(p.id)} onChange={(e) => setGot(e.target.checked ? [...sel, p.id] : sel.filter((x: number) => x !== p.id))} />
            <span className="flex-1">{p.name} × {fa(p.qty)}</span>
            <div className="w-36"><MoneyInput value={prices[p.id] ?? p.buyback_price} onChange={(v) => setPrices({ ...prices, [p.id]: v })} aria-label="قیمت واحد" /></div>
          </div>
        ))}
      </Modal>
      <Modal open={dlg === 'pay'} onClose={() => setDlg(null)} title="ثبت پرداخت" footer={<ActionButton onClick={async () => { await post(`/admin/shipments/${id}/pay`, { reference: ref }); inv('/admin'); setDlg(null) }}>پرداخت شد</ActionButton>}>
        <Field label="شماره پیگیری / مرجع پرداخت"><input className="input" value={ref} onChange={(e) => setRef(e.target.value)} /></Field>
      </Modal>
      <Modal open={dlg === 'reject'} onClose={() => setDlg(null)} title="رد مرسوله" footer={<ActionButton className="btn-danger" onClick={async () => { await post(`/admin/shipments/${id}/reject`, { notes: reason }); inv('/admin'); setDlg(null) }}>رد</ActionButton>}>
        <Field label="دلیل (الزامی)"><textarea className="input" rows={3} value={reason} onChange={(e) => setReason(e.target.value)} /></Field>
      </Modal>
    </div>
  )
}

// ───────── technicians ─────────
export function Technicians() {
  const [status, setStatus] = useState('')
  const [q, setQ] = useState('')
  const [add, setAdd] = useState<any>(null)
  const nav = useNavigate()
  const can = useCan()
  const inv = useInvalidate()
  const d = useGet<any[]>('/admin/technicians' + qs({ status, q }))
  return (
    <div>
      <PageHeader title="تکنسین‌ها" actions={can('service.technicians') && <button className="btn-primary" onClick={() => setAdd({ full_name: '', phone: '', password: '', city: '', province: '' })}><Plus className="h-4 w-4" />تکنسین جدید</button>} />
      <Tabs value={status} onChange={setStatus} items={[{ key: '', label: 'همه' }, { key: 'pending', label: 'منتظر تایید' }, { key: 'approved', label: 'فعال' }, { key: 'rejected', label: 'رد شده' }]} />
      <Card>
        <div className="border-b border-line p-3"><SearchBox value={q} onChange={setQ} placeholder="نام، موبایل یا شهر" /></div>
        {d.isLoading ? <Loading /> : !d.data?.length ? <Empty /> : (
          <DataTable rows={d.data} rowKey={(r: any) => r.id} onRow={(r: any) => nav(`/admin/technicians/${r.id}`)} cols={[{ head: 'نام', cell: (r: any) => <b>{r.full_name}</b> }, { head: 'موبایل', cell: (r: any) => <span dir="ltr" className="num">{r.phone}</span> }, { head: 'شهر', cell: (r: any) => r.city || '—' },
            { head: 'تعمیر باز', cell: (r: any) => fa(r.open_repairs) }, { head: 'مانده حساب', cell: (r: any) => <span className={r.balance > 0 ? 'font-bold text-warn num' : 'num'}>{toman(r.balance)}</span> }, { head: 'وضعیت', cell: (r: any) => <StatusBadge map={techStatus} value={r.status} /> }]} />
        )}
      </Card>
      <Modal open={!!add} onClose={() => setAdd(null)} title="تکنسین جدید" footer={<ActionButton onClick={async () => { await post('/admin/technicians', add); inv('/admin/technicians'); setAdd(null) }}>ثبت</ActionButton>}>
        {add && <div className="grid gap-3 sm:grid-cols-2"><Field label="نام"><input className="input" value={add.full_name} onChange={(e) => setAdd({ ...add, full_name: e.target.value })} /></Field><Field label="موبایل (نام کاربری)"><NumInput value={add.phone} onChange={(v) => setAdd({ ...add, phone: v })} /></Field>
          <Field label="گذرواژه موقت"><input className="input" dir="ltr" value={add.password} onChange={(e) => setAdd({ ...add, password: e.target.value })} /></Field><Field label="شهر"><input className="input" value={add.city} onChange={(e) => setAdd({ ...add, city: e.target.value })} /></Field></div>}
      </Modal>
    </div>
  )
}

export function TechnicianDetail() {
  const { id } = useParams()
  const t = useGet<any>(`/admin/technicians/${id}`)
  const led = useGet<any>(`/admin/technicians/${id}/ledger`)
  const can = useCan()
  const inv = useInvalidate()
  const [rev, setRev] = useState('')
  const [entry, setEntry] = useState<any>(null)
  if (t.isLoading) return <Loading />
  if (t.error) return <ErrorBox error={t.error} />
  const x = t.data
  return (
    <div className="space-y-4">
      <PageHeader title={x.full_name} actions={<><StatusBadge map={techStatus} value={x.status} /><Link to="/admin/technicians" className="btn-ghost btn-sm">بازگشت</Link></>} />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="p-4"><KV k="موبایل" v={<span dir="ltr">{x.phone}</span>} /><KV k="کد ملی" v={x.national_id} /><KV k="شهر" v={`${x.province} ${x.city}`} /><KV k="نشانی" v={x.address} /><KV k="درباره" v={x.about} /><KV k="قطعات نزد تکنسین" v={fa(x.stock_units)} /></Card>
        <Card className="p-4"><div className="text-xs text-soft">مانده حساب (طلب تکنسین)</div><div className="text-3xl font-extrabold num">{toman(x.balance)}</div>
          {can('service.technicians') && <div className="mt-3 flex gap-2"><button className="btn-soft btn-sm" onClick={() => setEntry({ kind: 'payment', amount: Math.max(x.balance, 0), note: '' })}>ثبت پرداخت</button><button className="btn-ghost btn-sm" onClick={() => setEntry({ kind: 'adjustment', amount: 0, note: '' })}>اصلاحیه</button></div>}</Card>
      </div>
      {x.status === 'pending' && can('service.technicians') && (
        <Card className="space-y-3 p-4"><Field label="توضیح بررسی"><input className="input" value={rev} onChange={(e) => setRev(e.target.value)} /></Field>
          <div className="flex gap-2"><ActionButton onClick={async () => { await post(`/admin/technicians/${id}/review`, { decision: 'approve', notes: rev }); inv('/admin/technicians') }}>تایید</ActionButton><ActionButton className="btn-ghost text-bad" onClick={async () => { await post(`/admin/technicians/${id}/review`, { decision: 'reject', notes: rev }); inv('/admin/technicians') }}>رد</ActionButton></div></Card>
      )}
      {can('service.technicians') && x.status !== 'pending' && <ActionButton className="btn-ghost" onClick={async () => { await patch(`/admin/technicians/${id}`, { is_active: !x.is_active }); inv('/admin/technicians') }}>{x.is_active ? 'غیرفعال کردن حساب' : 'فعال کردن حساب'}</ActionButton>}
      <Card><div className="border-b border-line p-4 font-bold">صورت‌حساب</div>
        {!led.data?.entries?.length ? <Empty title="تراکنشی نیست" /> : led.data.entries.map((e: any) => (
          <div key={e.id} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0"><div><b>{ledgerKind[e.kind]}</b><div className="text-xs text-soft">{e.note} · {dateTime(e.created_at)}</div></div><span className={`font-bold num ${e.amount < 0 ? 'text-ok' : 'text-warn'}`} dir="ltr">{e.amount > 0 ? '+' : ''}{fa(e.amount)}</span></div>
        ))}
      </Card>
      <Modal open={!!entry} onClose={() => setEntry(null)} title={entry?.kind === 'payment' ? 'ثبت پرداخت به تکنسین' : 'اصلاحیه حساب'} footer={<ActionButton onClick={async () => { await post(`/admin/technicians/${id}/ledger`, entry); inv('/admin/technicians'); setEntry(null) }}>ثبت</ActionButton>}>
        {entry && <div className="space-y-3"><Field label={entry.kind === 'adjustment' ? 'مبلغ (مثبت = طلب بیشتر، منفی = کسر)' : 'مبلغ پرداختی'}>{entry.kind === 'payment' ? <MoneyInput value={entry.amount} onChange={(v) => setEntry({ ...entry, amount: v })} /> : <input className="input num" type="number" dir="ltr" value={entry.amount} onChange={(e) => setEntry({ ...entry, amount: +e.target.value })} />}</Field>
          <Field label="توضیح (الزامی)"><input className="input" value={entry.note} onChange={(e) => setEntry({ ...entry, note: e.target.value })} /></Field></div>}
      </Modal>
    </div>
  )
}
