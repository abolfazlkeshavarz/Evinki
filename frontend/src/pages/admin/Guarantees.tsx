import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { ActionButton, Badge, Card, DataTable, Empty, ErrorBox, Field, JalaliInput, KV, Loading, Modal, NumInput, PageHeader, Pager, SearchBox, StatusBadge, Tabs } from '../../components/ui'
import { del, patch, post } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { addMonthsISO, date, fa, todayISO } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { guaranteeStatus, repairStatus, tierLabel } from '../../lib/labels'

export function Guarantees() {
  const [status, setStatus] = useState('')
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [exp, setExp] = useState(false)
  const [add, setAdd] = useState<any>(null)
  const can = useCan()
  const nav = useNavigate()
  const products = useGet<any[]>('/lookup/products')
  const inv = useInvalidate()
  const list = useGet<any>('/admin/guarantees' + qs({ status, q, page, limit: 20, expiring_in_days: exp ? 30 : undefined }))
  return (
    <div>
      <PageHeader title="گارانتی‌ها" actions={can('warranty.write') && <button className="btn-primary" onClick={() => setAdd({ code: '', product_id: 0, purchase_date: todayISO(), full_name: '', phone: '', city: '' })}><Plus className="h-4 w-4" />ثبت گارانتی</button>} />
      <Tabs value={status} onChange={(s) => { setStatus(s); setPage(1) }} items={[{ key: '', label: 'همه' }, { key: 'pending', label: 'در انتظار تایید' }, { key: 'approved', label: 'فعال' }, { key: 'renewed', label: 'تمدید شده' }, { key: 'expired', label: 'منقضی' }, { key: 'rejected', label: 'رد شده' }, { key: 'cancelled', label: 'لغو شده' }]} />
      <Card>
        <div className="flex flex-wrap items-center gap-3 border-b border-line p-3">
          <SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} placeholder="کد، نام، موبایل یا کد ملی" />
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={exp} onChange={(e) => { setExp(e.target.checked); setPage(1) }} />فقط تا ۳۰ روز آینده منقضی می‌شوند</label>
        </div>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data.items.length ? <Empty title="گارانتی پیدا نشد" /> : (
          <>
            <DataTable rows={list.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => nav(`/admin/guarantees/${r.id}`)} cols={[
              { head: 'کد', cell: (r: any) => <b dir="ltr" className="num">{r.code}</b> }, { head: 'مشتری', cell: (r: any) => <div>{r.customer_name}<div className="text-xs text-soft num" dir="ltr">{r.customer_phone}</div></div> },
              { head: 'محصول', cell: (r: any) => r.product_name }, { head: 'پایان', cell: (r: any) => <div className="num">{date(r.expiry_date)}<div className="text-xs text-soft">{r.days_remaining >= 0 ? fa(r.days_remaining) + ' روز مانده' : 'منقضی'}</div></div> },
              { head: 'وضعیت', cell: (r: any) => <div className="flex gap-1"><StatusBadge map={guaranteeStatus} value={r.status} />{r.tier === 'golden' && <StatusBadge map={tierLabel} value="golden" />}</div> }]} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} />
          </>
        )}
      </Card>
      <Modal open={!!add} onClose={() => setAdd(null)} title="ثبت گارانتی (تایید‌شده)" footer={<ActionButton onClick={async () => { await post('/admin/guarantees', add); inv('/admin/guarantees'); setAdd(null) }}>ثبت</ActionButton>}>
        {add && <div className="grid gap-3 sm:grid-cols-2">
          <Field label="محصول" className="sm:col-span-2"><select className="input" value={add.product_id} onChange={(e) => setAdd({ ...add, product_id: +e.target.value })}><option value={0}>انتخاب…</option>{products.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></Field>
          <Field label="کد گارانتی"><input className="input" dir="ltr" value={add.code} onChange={(e) => setAdd({ ...add, code: e.target.value })} /></Field>
          <Field label="تاریخ خرید"><JalaliInput value={add.purchase_date} onChange={(v) => setAdd({ ...add, purchase_date: v })} /></Field>
          <Field label="نام مشتری"><input className="input" value={add.full_name} onChange={(e) => setAdd({ ...add, full_name: e.target.value })} /></Field>
          <Field label="موبایل"><NumInput value={add.phone} onChange={(v) => setAdd({ ...add, phone: v })} /></Field>
          <Field label="شهر"><input className="input" value={add.city} onChange={(e) => setAdd({ ...add, city: e.target.value })} /></Field>
        </div>}
      </Modal>
    </div>
  )
}

export function GuaranteeDetail() {
  const { id } = useParams()
  const q = useGet<any>(`/admin/guarantees/${id}`)
  const can = useCan()
  const inv = useInvalidate()
  const nav = useNavigate()
  const [dlg, setDlg] = useState<'renew' | 'golden' | null>(null)
  const [d, setD] = useState({ date: '', months: 3, notes: '' })
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} />
  const { guarantee: g, repairs } = q.data
  const act = (a: string, body: any = {}) => async () => { await post(`/admin/guarantees/${id}/${a}`, body); inv('/admin/guarantees') }
  const approver = can('warranty.approve')
  return (
    <div className="space-y-4">
      <PageHeader title={`گارانتی ${g.code}`} actions={<><StatusBadge map={guaranteeStatus} value={g.status} />{g.tier === 'golden' && <StatusBadge map={tierLabel} value="golden" />}<Link to="/admin/guarantees" className="btn-ghost btn-sm">بازگشت</Link></>} />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="p-4"><KV k="محصول" v={g.product_name} /><KV k="تاریخ خرید" v={date(g.purchase_date)} /><KV k="پایان گارانتی" v={date(g.expiry_date)} /><KV k="روز باقی‌مانده" v={g.days_remaining >= 0 ? fa(g.days_remaining) : 'منقضی'} />
          {g.golden_expiry_date && <KV k="گارانتی طلایی تا" v={date(g.golden_expiry_date)} />}<KV k="منبع ثبت" v={{ web: 'ثبت آنلاین مشتری', admin: 'ثبت توسط کارمند', order: 'خرید از فروشگاه', import: 'انتقال از سامانه قدیم', app: 'اپلیکیشن' }[g.source as string] ?? g.source} />
          {g.invoice_file && <KV k="فاکتور" v={<a className="text-brand underline" href={g.invoice_file} target="_blank">مشاهده</a>} />}{g.notes && <KV k="یادداشت" v={g.notes} />}</Card>
        <Card className="p-4"><KV k="مشتری" v={g.customer_name} /><KV k="موبایل" v={<span dir="ltr" className="num">{g.customer_phone}</span>} /><KV k="شهر" v={g.customer_city} /></Card>
      </div>
      {approver && (
        <Card className="flex flex-wrap gap-2 p-4">
          {g.status === 'pending' && <><ActionButton onClick={act('approve')} success="تایید شد و پیامک ارسال می‌شود">تایید</ActionButton><ActionButton className="btn-ghost text-bad" confirm="رد شود؟" onClick={act('reject')}>رد</ActionButton></>}
          {['approved', 'renewed', 'expired'].includes(g.status) && <button className="btn-soft" onClick={() => { setD({ date: g.expiry_date?.slice(0, 10), months: 3, notes: '' }); setDlg('renew') }}>تمدید</button>}
          {['approved', 'renewed'].includes(g.status) && <button className="btn-soft" onClick={() => { setD({ date: todayISO(), months: 3, notes: '' }); setDlg('golden') }}>تعیین گارانتی طلایی</button>}
          {g.golden_expiry_date && <ActionButton className="btn-ghost" onClick={async () => { await del(`/admin/guarantees/${id}/golden`); inv('/admin/guarantees') }}>حذف طلایی</ActionButton>}
          {['pending', 'approved', 'renewed'].includes(g.status) && <ActionButton className="btn-ghost text-bad" confirm="گارانتی لغو شود؟" onClick={act('cancel')}>لغو</ActionButton>}
          {can('warranty.delete') && <ActionButton className="btn-ghost text-bad" confirm="گارانتی حذف شود؟" onClick={async () => { await del(`/admin/guarantees/${id}`); nav('/admin/guarantees') }}>حذف</ActionButton>}
        </Card>
      )}
      {can('service.manage') && ['approved', 'renewed', 'expired'].includes(g.status) && <OpenRepair guaranteeId={g.id} onDone={(rid) => nav(`/admin/repairs/${rid}`)} />}
      <Card><div className="border-b border-line p-4 font-bold">سابقه تعمیرات</div>
        {!repairs.length ? <Empty title="تعمیری ثبت نشده" /> : repairs.map((r: any) => <Link key={r.id} to={`/admin/repairs/${r.id}`} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0 hover:bg-muted/40"><div><b className="num">{r.number}</b> <span className="text-soft">{r.complaint}</span></div><div className="flex gap-2">{r.guarantee_was_expired && <Badge tone="warn">خارج از گارانتی</Badge>}<StatusBadge map={repairStatus} value={r.status} /></div></Link>)}</Card>
      <Modal open={dlg === 'renew'} onClose={() => setDlg(null)} title="تمدید گارانتی" footer={<ActionButton onClick={async () => { await post(`/admin/guarantees/${id}/renew`, { new_expiry_date: d.date, notes: d.notes }); inv('/admin/guarantees'); setDlg(null) }}>تمدید</ActionButton>}>
        <Field label="تاریخ جدید پایان گارانتی"><JalaliInput value={d.date} onChange={(v) => setD({ ...d, date: v })} /></Field>
        <div className="mt-2 flex gap-2">{[3, 6, 12].map((m) => <button key={m} className="btn-ghost btn-sm" onClick={() => setD({ ...d, date: addMonthsISO(g.expiry_date.slice(0, 10), m) })}>+{fa(m)} ماه</button>)}</div>
        <Field label="یادداشت" className="mt-3"><input className="input" value={d.notes} onChange={(e) => setD({ ...d, notes: e.target.value })} /></Field>
      </Modal>
      <Modal open={dlg === 'golden'} onClose={() => setDlg(null)} title="گارانتی طلایی" footer={<ActionButton onClick={async () => { await post(`/admin/guarantees/${id}/golden`, { start_date: d.date, golden_months: d.months }); inv('/admin/guarantees'); setDlg(null) }}>ثبت</ActionButton>}>
        <Field label="شروع"><JalaliInput value={d.date} onChange={(v) => setD({ ...d, date: v })} /></Field>
        <Field label="مدت (ماه)" className="mt-3"><NumInput value={d.months} onChange={(v) => setD({ ...d, months: +v })} /></Field>
      </Modal>
    </div>
  )
}

function OpenRepair({ guaranteeId, onDone }: { guaranteeId: number; onDone: (id: number) => void }) {
  const [open, setOpen] = useState(false)
  const [tech, setTech] = useState(0)
  const [text, setText] = useState('')
  const techs = useGet<any[]>(open ? '/admin/technicians?status=approved' : null)
  return (
    <>
      <button className="btn-soft" onClick={() => setOpen(true)}>ثبت درخواست سرویس برای این دستگاه</button>
      <Modal open={open} onClose={() => setOpen(false)} title="درخواست سرویس" footer={<ActionButton onClick={async () => { const r = await post('/admin/repairs', { guarantee_id: guaranteeId, technician_id: tech || null, complaint: text }); onDone(r.id) }}>ثبت</ActionButton>}>
        <Field label="مشکل"><textarea className="input" rows={3} value={text} onChange={(e) => setText(e.target.value)} /></Field>
        <Field label="تکنسین (می‌توانید بعداً تعیین کنید)" className="mt-3"><select className="input" value={tech} onChange={(e) => setTech(+e.target.value)}><option value={0}>بدون تکنسین</option>{techs.data?.map((t) => <option key={t.id} value={t.id}>{t.full_name} — {t.city}</option>)}</select></Field>
      </Modal>
    </>
  )
}

export function Customers() {
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const list = useGet<any>('/admin/customers' + qs({ q, page, limit: 25 }))
  const [view, setView] = useState<number | null>(null)
  const one = useGet<any>(view ? `/admin/customers/${view}` : null)
  return (
    <div>
      <PageHeader title="مشتریان" />
      <Card>
        <div className="border-b border-line p-3"><SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} placeholder="نام، موبایل یا کد ملی" /></div>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data.items.length ? <Empty /> : (
          <>
            <DataTable rows={list.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => setView(r.id)} cols={[{ head: 'نام', cell: (r: any) => <b>{r.full_name}</b> }, { head: 'موبایل', cell: (r: any) => <span dir="ltr" className="num">{r.phone}</span> }, { head: 'شهر', cell: (r: any) => r.city || '—' }, { head: 'گارانتی', cell: (r: any) => fa(r.guarantee_count) }]} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} />
          </>
        )}
      </Card>
      <Modal open={!!view} onClose={() => setView(null)} title="مشتری">
        {one.isLoading ? <Loading /> : one.data && <><KV k="نام" v={one.data.full_name} /><KV k="موبایل" v={<span dir="ltr">{one.data.phone}</span>} /><KV k="کد ملی" v={one.data.national_id} /><KV k="نشانی" v={`${one.data.province} ${one.data.city} ${one.data.address}`} />
          <div className="mt-4 font-bold">گارانتی‌ها</div>{one.data.guarantees.map((g: any) => <Link key={g.id} to={`/admin/guarantees/${g.id}`} className="flex justify-between border-b border-line/60 py-2 text-sm"><span>{g.product_name} <span dir="ltr" className="text-soft">{g.code}</span></span><StatusBadge map={guaranteeStatus} value={g.status} /></Link>)}</>}
      </Modal>
    </div>
  )
}

void patch
