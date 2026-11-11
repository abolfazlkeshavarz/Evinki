import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Download, Pencil, Plus, RotateCcw, Trash2 } from 'lucide-react'
import { ActionButton, Badge, Card, DataTable, Empty, ErrorBox, Field, JalaliInput, KV, Loading, Modal, NumInput, PageHeader, Pager, SearchBox, StatusBadge, Tabs } from '../../components/ui'
import { del, download, patch, post } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { addJalaliMonths, date, fa, todayISO } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { guaranteeStatus, repairStatus, tierLabel } from '../../lib/labels'

export function Guarantees() {
  const [status, setStatus] = useState('')
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [expDays, setExpDays] = useState('')
  const [add, setAdd] = useState<any>(null)
  const [chk, setChk] = useState<any>(null)
  const [exportOpen, setExportOpen] = useState<any>(null)
  const can = useCan()
  const nav = useNavigate()
  const products = useGet<any[]>('/lookup/products')
  const inv = useInvalidate()
  const list = useGet<any>('/admin/guarantees' + qs({ status, q, page, limit: 20, expiring_in_days: expDays || undefined }))
  // live check of the typed code: shows the manufacture date it carries and the expiry that will result
  useEffect(() => {
    setChk(null)
    if (!add || !add.product_id || (add.code ?? '').length < 4) return
    const t = setTimeout(async () => { try { setChk(await post('/public/guarantees/validate', { product_id: add.product_id, code: add.code, purchase_date: add.purchase_date })) } catch { /* ignore */ } }, 350)
    return () => clearTimeout(t)
  }, [add?.product_id, add?.code, add?.purchase_date])
  return (
    <div>
      <PageHeader title="گارانتی‌ها" actions={<><button className="btn-ghost" onClick={() => setExportOpen({ status, product_id: 0, expiring_in_days: expDays, code_from: '', code_to: '' })}><Download className="h-4 w-4" />خروجی اکسل</button>{can('warranty.write') && <button className="btn-primary" onClick={() => setAdd({ code: '', product_id: 0, purchase_date: todayISO(), manufacture_date: '', expiry_date: '', full_name: '', phone: '', city: '' })}><Plus className="h-4 w-4" />ثبت گارانتی</button>}</>} />
      <Tabs value={status} onChange={(s) => { setStatus(s); setPage(1) }} items={[{ key: '', label: 'همه' }, { key: 'pending', label: 'در انتظار تایید' }, { key: 'approved', label: 'فعال' }, { key: 'renewed', label: 'تمدید شده' }, { key: 'expired', label: 'منقضی' }, { key: 'rejected', label: 'رد شده' }, { key: 'cancelled', label: 'لغو شده' }]} />
      <Card>
        <div className="flex flex-wrap items-center gap-3 border-b border-line p-3">
          <SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} placeholder="کد، نام، موبایل یا کد ملی" />
          <select className="input w-auto" value={expDays} onChange={(e) => { setExpDays(e.target.value); setPage(1) }}><option value="">همهٔ تاریخ‌های انقضا</option><option value="30">انقضا تا ۳۰ روز آینده</option><option value="60">انقضا تا ۶۰ روز آینده</option><option value="90">انقضا تا ۹۰ روز آینده</option></select>
        </div>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data.items.length ? <Empty title="گارانتی پیدا نشد" /> : (
          <>
            <DataTable rows={list.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => nav(`/admin/guarantees/${r.id}`)} cols={[
              { head: 'کد', cell: (r: any) => <b dir="ltr" className="num">{r.code}</b> }, { head: 'مشتری', cell: (r: any) => <div>{r.customer_name}<div className="text-xs text-soft num" dir="ltr">{r.customer_phone}</div></div> },
              { head: 'محصول', cell: (r: any) => r.product_name }, { head: 'تولید', cell: (r: any) => (r.manufacture_date ? <span className="num">{date(r.manufacture_date)}{r.manufacture_approx ? <span className="text-xs text-soft"> ~</span> : null}</span> : '—') }, { head: 'پایان', cell: (r: any) => <div className="num">{date(r.expiry_date)}<div className="text-xs text-soft">{r.days_remaining >= 0 ? fa(r.days_remaining) + ' روز مانده' : 'منقضی'}</div></div> },
              { head: 'وضعیت', cell: (r: any) => <div className="flex gap-1"><StatusBadge map={guaranteeStatus} value={r.status} />{r.tier === 'golden' && <StatusBadge map={tierLabel} value="golden" />}</div> }]} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} />
          </>
        )}
      </Card>
      <Modal open={!!add} onClose={() => setAdd(null)} title="ثبت گارانتی (تایید‌شده)" footer={<ActionButton onClick={async () => { await post('/admin/guarantees', { ...add, manufacture_date: add.manufacture_date || undefined, expiry_date: add.expiry_date || undefined }); inv('/admin/guarantees'); setAdd(null) }}>ثبت</ActionButton>}>
        {add && <div className="grid gap-3 sm:grid-cols-2">
          <Field label="محصول" className="sm:col-span-2"><select className="input" value={add.product_id} onChange={(e) => setAdd({ ...add, product_id: +e.target.value })}><option value={0}>انتخاب…</option>{products.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></Field>
          <Field label="کد گارانتی"><input className="input" dir="ltr" value={add.code} onChange={(e) => setAdd({ ...add, code: e.target.value })} /></Field>
          {chk && <p className={`rounded-lg p-2.5 text-sm sm:col-span-2 ${chk.message_level === 'success' ? 'bg-ok/10 text-ok' : chk.message_level === 'warning' ? 'bg-warn/15 text-warn' : 'bg-bad/10 text-bad'}`}>{chk.message}{chk.expiry_date && <span className="mt-1 block text-xs text-ink">پایان گارانتی بر اساس قوانین محصول: <b>{date(chk.expiry_date)}</b>{chk.manufacture_date && <> · تولید: {date(chk.manufacture_date)}</>}</span>}</p>}
          <Field label="تاریخ خرید"><JalaliInput value={add.purchase_date} onChange={(v) => setAdd({ ...add, purchase_date: v })} /></Field>
          <Field label="تاریخ تولید (اختیاری)" hint="اگر خالی باشد از روی کد خوانده می‌شود"><JalaliInput value={add.manufacture_date} onChange={(v) => setAdd({ ...add, manufacture_date: v })} /></Field>
          <Field label="تاریخ پایان گارانتی (اختیاری)" hint="اگر خالی باشد خودکار حساب می‌شود"><JalaliInput value={add.expiry_date} onChange={(v) => setAdd({ ...add, expiry_date: v })} /></Field>
          <Field label="نام مشتری"><input className="input" value={add.full_name} onChange={(e) => setAdd({ ...add, full_name: e.target.value })} /></Field>
          <Field label="موبایل"><NumInput value={add.phone} onChange={(v) => setAdd({ ...add, phone: v })} /></Field>
          <Field label="شهر"><input className="input" value={add.city} onChange={(e) => setAdd({ ...add, city: e.target.value })} /></Field>
        </div>}
      </Modal>
      <Modal open={!!exportOpen} onClose={() => setExportOpen(null)} title="خروجی اکسل گارانتی‌ها" footer={<ActionButton onClick={async () => { await download('/admin/guarantees-export' + qs({ ...exportOpen, product_id: exportOpen.product_id || undefined }), 'guarantees.csv'); setExportOpen(null) }}><Download className="h-4 w-4" />دریافت فایل</ActionButton>}>
        {exportOpen && <div className="grid gap-3 sm:grid-cols-2">
          <Field label="وضعیت"><select className="input" value={exportOpen.status} onChange={(e) => setExportOpen({ ...exportOpen, status: e.target.value })}><option value="">همه</option><option value="pending">در انتظار تایید</option><option value="approved">فعال</option><option value="renewed">تمدید شده</option><option value="expired">منقضی</option><option value="rejected">رد شده</option><option value="cancelled">لغو شده</option></select></Field>
          <Field label="محصول"><select className="input" value={exportOpen.product_id} onChange={(e) => setExportOpen({ ...exportOpen, product_id: +e.target.value })}><option value={0}>همه</option>{products.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}</select></Field>
          <Field label="انقضا تا (روز آینده)"><select className="input" value={exportOpen.expiring_in_days} onChange={(e) => setExportOpen({ ...exportOpen, expiring_in_days: e.target.value })}><option value="">همه</option><option value="30">۳۰ روز</option><option value="60">۶۰ روز</option><option value="90">۹۰ روز</option></select></Field><span />
          <Field label="از کد"><input className="input" dir="ltr" value={exportOpen.code_from} onChange={(e) => setExportOpen({ ...exportOpen, code_from: e.target.value })} /></Field>
          <Field label="تا کد"><input className="input" dir="ltr" value={exportOpen.code_to} onChange={(e) => setExportOpen({ ...exportOpen, code_to: e.target.value })} /></Field>
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
  const [ed, setEd] = useState<any>(null)
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} />
  const { guarantee: g, repairs } = q.data
  const act = (a: string, body: any = {}) => async () => { await post(`/admin/guarantees/${id}/${a}`, body); inv('/admin/guarantees') }
  const approver = can('warranty.approve')
  return (
    <div className="space-y-4">
      <PageHeader title={`گارانتی ${g.code}`} actions={<><StatusBadge map={guaranteeStatus} value={g.status} />{g.tier === 'golden' && <StatusBadge map={tierLabel} value="golden" />}<Link to="/admin/guarantees" className="btn-ghost btn-sm">بازگشت</Link></>} />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="p-4"><KV k="محصول" v={g.product_name} /><KV k="تاریخ تولید" v={g.manufacture_date ? <>{date(g.manufacture_date)}{g.manufacture_approx && <span className="text-xs text-soft"> (پایان ماه/فصل کد؛ بهترین حالت برای مشتری)</span>}</> : <span className="text-soft">در کد مشخص نیست</span>} /><KV k="تاریخ خرید" v={date(g.purchase_date)} /><KV k="پایان گارانتی" v={<>{date(g.expiry_date)}<span className="ms-2 text-xs text-soft">{g.expiry_edited_at ? `ویرایش دستی (${date(g.expiry_edited_at)})${g.expiry_edit_note ? ' — ' + g.expiry_edit_note : ''}` : g.expiry_basis === 'manufacture' ? 'محاسبه از تاریخ تولید' : 'محاسبه از تاریخ خرید'}</span></>} /><KV k="روز باقی‌مانده" v={g.days_remaining >= 0 ? fa(g.days_remaining) : 'منقضی'} />
          {g.golden_expiry_date && <KV k="گارانتی طلایی تا" v={date(g.golden_expiry_date)} />}<KV k="منبع ثبت" v={{ web: 'ثبت آنلاین مشتری', admin: 'ثبت توسط کارمند', order: 'خرید از فروشگاه', import: 'انتقال از سامانه قدیم', app: 'اپلیکیشن' }[g.source as string] ?? g.source} />
          <KV k="عکس فاکتور" v={g.invoice_file ? <a href={g.invoice_file} target="_blank" rel="noopener"><img src={g.invoice_file} alt="فاکتور" className="h-20 rounded-lg border border-line object-cover" onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} /><span className="text-brand underline">مشاهده</span></a> : <span className="text-soft">ندارد</span>} />
          <KV k="عکس کارت گارانتی" v={g.card_file ? <a href={g.card_file} target="_blank" rel="noopener"><img src={g.card_file} alt="کارت گارانتی" className="h-20 rounded-lg border border-line object-cover" onError={(e) => ((e.target as HTMLImageElement).style.display = 'none')} /><span className="text-brand underline">مشاهده</span></a> : <span className="text-soft">ندارد</span>} />{g.notes && <KV k="یادداشت" v={g.notes} />}</Card>
        <Card className="p-4"><KV k="مشتری" v={g.customer_name} /><KV k="موبایل" v={<span dir="ltr" className="num">{g.customer_phone}</span>} /><KV k="شهر" v={g.customer_city} /></Card>
      </div>
      {can('warranty.write') && <Card className="flex flex-wrap items-center gap-2 p-4"><button className="btn-ghost" onClick={() => setEd({ purchase_date: g.purchase_date?.slice(0, 10), manufacture_date: g.manufacture_date?.slice(0, 10) ?? '', expiry_date: g.expiry_date?.slice(0, 10), expiry_note: '' })}><Pencil className="h-4 w-4" />ویرایش تاریخ‌ها (تولید، خرید، پایان گارانتی)</button>
        <ActionButton className="btn-ghost" confirm="تاریخ پایان و گارانتی طلایی از نو بر اساس قوانین محصول و تاریخ‌های ثبت‌شده محاسبه می‌شود. ادامه می‌دهید؟" success="دوباره محاسبه شد" onClick={async () => { await patch(`/admin/guarantees/${id}`, { recalculate: true }); inv('/admin/guarantees') }}><RotateCcw className="h-4 w-4" />محاسبه مجدد طبق قوانین محصول</ActionButton></Card>}
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
        <div className="mt-2 flex gap-2">{[3, 6, 12].map((m) => <button key={m} className="btn-ghost btn-sm" onClick={() => setD({ ...d, date: addJalaliMonths(g.expiry_date.slice(0, 10), m) })}>+{fa(m)} ماه</button>)}</div>
        <Field label="یادداشت" className="mt-3"><input className="input" value={d.notes} onChange={(e) => setD({ ...d, notes: e.target.value })} /></Field>
      </Modal>
      <Modal open={!!ed} onClose={() => setEd(null)} title="ویرایش تاریخ‌های گارانتی" footer={<ActionButton success="ذخیره شد" onClick={async () => { await patch(`/admin/guarantees/${id}`, { purchase_date: ed.purchase_date, manufacture_date: ed.manufacture_date, expiry_date: ed.expiry_date, expiry_note: ed.expiry_note }); inv('/admin/guarantees'); setEd(null) }}>ذخیره</ActionButton>}>
        {ed && <div className="grid gap-3 sm:grid-cols-2">
          <Field label="تاریخ تولید"><JalaliInput value={ed.manufacture_date} onChange={(v) => setEd({ ...ed, manufacture_date: v })} /></Field>
          <Field label="تاریخ خرید"><JalaliInput value={ed.purchase_date} onChange={(v) => v && setEd({ ...ed, purchase_date: v })} clearable={false} /></Field>
          <Field label="تاریخ پایان گارانتی" className="sm:col-span-2"><JalaliInput value={ed.expiry_date} onChange={(v) => v && setEd({ ...ed, expiry_date: v })} clearable={false} /></Field>
          <div className="flex flex-wrap gap-2 sm:col-span-2">{[-3, 3, 6, 12].map((m) => <button key={m} type="button" className="btn-ghost btn-sm" onClick={() => setEd({ ...ed, expiry_date: addJalaliMonths(ed.expiry_date, m) })}>{m > 0 ? '+' : ''}{fa(m)} ماه</button>)}</div>
          <Field label="دلیل ویرایش (در گزارش تغییرات ثبت می‌شود)" className="sm:col-span-2"><input className="input" value={ed.expiry_note} onChange={(e) => setEd({ ...ed, expiry_note: e.target.value })} /></Field>
          <p className="text-xs leading-6 text-soft sm:col-span-2">تغییر دستی تاریخ پایان، جایگزین محاسبهٔ خودکار می‌شود و با مهر «ویرایش دستی» نمایش داده می‌شود. برای برگشت به محاسبهٔ خودکار از «محاسبه مجدد» استفاده کنید.</p>
        </div>}
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
  const [sp] = useSearchParams()
  const [q, setQ] = useState(sp.get('q') ?? '')
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

const trashKinds: [string, string][] = [['guarantees', 'گارانتی‌ها'], ['customers', 'مشتریان'], ['products', 'محصولات'], ['repairs', 'تعمیرها'], ['parts', 'قطعات']]

/** Deleted records are only hidden: restore them from here (the old system's "سطل زباله"). */
export function TrashPage() {
  const [kind, setKind] = useState('guarantees')
  const sum = useGet<Record<string, number>>('/admin/trash')
  const list = useGet<any[]>(`/admin/trash/${kind}`)
  const inv = useInvalidate()
  return (
    <div>
      <PageHeader title="سطل بازیافت" subtitle="موارد حذف‌شده اینجا می‌مانند و هر زمان قابل بازیابی‌اند." />
      <Tabs value={kind} onChange={setKind} items={trashKinds.map(([k, l]) => ({ key: k, label: l, count: sum.data?.[k] }))} />
      <Card>{list.isLoading ? <Loading /> : !list.data?.length ? <Empty title="چیزی در سطل نیست" /> : list.data.map((r) => (
        <div key={r.id} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0">
          <div><b dir="auto">{r.label}</b><div className="text-xs text-soft">حذف‌شده: {date(r.deleted_at)}</div></div>
          <ActionButton className="btn-soft btn-sm" success="بازیابی شد" onClick={async () => { await post(`/admin/trash/${kind}/${r.id}/restore`); inv('/admin') }}><RotateCcw className="h-4 w-4" />بازیابی</ActionButton>
        </div>))}</Card>
    </div>
  )
}
void Trash2
