import { useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router-dom'
import { CheckCircle2, Plus, Wrench } from 'lucide-react'
import { ActionButton, Card, Empty, ErrorBox, Field, KV, Loading, Modal, NumInput, PageHeader, Stat, StatusBadge, useToast } from '../../components/ui'
import { del, post, put } from '../../lib/api'
import { useUser } from '../../lib/auth'
import { date, dateTime, fa, toman } from '../../lib/format'
import { useGet, useInvalidate } from '../../lib/hooks'
import { ProductCard } from '../shop/Shop'
import { guaranteeStatus, orderStatus, repairStatus, tierLabel } from '../../lib/labels'

export function AccountHome() {
  const orders = useGet<any[]>('/shop/orders')
  const gs = useGet<any[]>('/my/guarantees')
  const repairs = useGet<any[]>('/my/repairs')
  return (
    <div className="space-y-6">
      <PageHeader title="پیشخوان" />
      <div className="grid grid-cols-3 gap-3">
        <Stat label="سفارش‌ها" value={fa(orders.data?.length ?? 0)} to="/account/orders" />
        <Stat label="گارانتی‌ها" value={fa(gs.data?.length ?? 0)} to="/account/guarantees" />
        <Stat label="سرویس‌ها" value={fa(repairs.data?.length ?? 0)} />
      </div>
      {!!repairs.data?.length && (
        <Card><div className="border-b border-line p-4 font-bold">درخواست‌های سرویس</div>
          {repairs.data.map((r) => (
            <div key={r.id} className="flex items-center justify-between gap-3 border-b border-line/60 p-4 last:border-0 text-sm">
              <div><div className="font-semibold">{r.product_name} <span className="font-normal text-soft num">{r.number}</span></div><div className="text-xs text-soft">{r.technician_name ? 'تکنسین: ' + r.technician_name : 'در انتظار تخصیص تکنسین'} · {dateTime(r.created_at)}</div></div>
              <StatusBadge map={repairStatus} value={r.status} />
            </div>
          ))}
        </Card>
      )}
      <Card className="flex flex-wrap items-center justify-between gap-3 p-5"><div><div className="font-bold">دستگاه جدید دارید؟</div><div className="text-sm text-soft">گارانتی آن را ثبت کنید.</div></div><Link to="/guarantee" className="btn-primary">ثبت گارانتی</Link></Card>
    </div>
  )
}

export function MyOrders() {
  const q = useGet<any[]>('/shop/orders')
  return (
    <div><PageHeader title="سفارش‌های من" />
      {q.isLoading ? <Loading /> : !q.data?.length ? <Empty title="هنوز سفارشی ثبت نکرده‌اید" action={<Link to="/products" className="btn-primary">مشاهده محصولات</Link>} /> : (
        <div className="space-y-3">{q.data.map((o) => (
          <Link key={o.id} to={`/account/orders/${o.id}`} className="card flex items-center justify-between gap-3 p-4 transition hover:border-brand">
            <div><div className="font-bold num">{o.number}</div><div className="text-xs text-soft">{dateTime(o.created_at)} · {fa(o.item_count)} کالا</div></div>
            <div className="text-end"><StatusBadge map={orderStatus} value={o.status} /><div className="mt-1 text-sm font-bold num">{toman(o.total)}</div></div>
          </Link>))}</div>
      )}
    </div>
  )
}

export function MyOrder() {
  const { id } = useParams()
  const [sp] = useSearchParams()
  const q = useGet<any>(`/shop/orders/${id}`)
  const inv = useInvalidate()
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} />
  const { order: o, items } = q.data
  return (
    <div className="space-y-4">
      {sp.get('paid') === 'true' && <div className="flex items-center gap-2 rounded-xl bg-ok/10 p-4 text-ok"><CheckCircle2 className="h-5 w-5" />پرداخت با موفقیت انجام شد. سفارش شما ثبت شد.</div>}
      {sp.get('paid') === 'false' && o.status === 'pending_payment' && <div className="rounded-xl bg-bad/10 p-4 text-bad">پرداخت انجام نشد. می‌توانید دوباره تلاش کنید.</div>}
      <PageHeader title={`سفارش ${o.number}`} actions={<StatusBadge map={orderStatus} value={o.status} />} />
      <Card className="divide-y divide-line">
        {items.map((i: any) => <div key={i.id} className="flex justify-between gap-3 p-4 text-sm"><span>{i.name} × {fa(i.qty)}{i.serial_code && <span className="ms-2 text-xs text-soft" dir="ltr">({i.serial_code})</span>}</span><span className="num">{toman(i.total)}</span></div>)}
      </Card>
      <Card className="p-4">
        <KV k="جمع کالاها" v={toman(o.subtotal)} />{o.discount > 0 && <KV k="تخفیف" v={toman(o.discount)} />}<KV k="ارسال" v={toman(o.shipping_fee)} /><KV k="مبلغ نهایی" v={<b>{toman(o.total)}</b>} />
        {o.tracking_code && <KV k="کد رهگیری" v={<span dir="ltr" className="num">{o.tracking_code}</span>} />}
        <KV k="نشانی" v={`${o.address?.city ?? ''}، ${o.address?.address ?? ''}`} />
      </Card>
      {o.status === 'pending_payment' && (
        <div className="flex gap-2">
          <ActionButton className="btn-primary" onClick={async () => { const r = await post(`/shop/orders/${o.id}/pay`); window.location.href = r.payment_url }}>پرداخت</ActionButton>
          <ActionButton className="btn-ghost" confirm="سفارش لغو شود؟" onClick={async () => { await post(`/shop/orders/${o.id}/cancel`); inv('/shop/orders') }}>لغو سفارش</ActionButton>
        </div>
      )}
    </div>
  )
}

export function MyGuarantees() {
  const q = useGet<any[]>('/my/guarantees')
  const [svc, setSvc] = useState<any>(null)
  const [text, setText] = useState('')
  const toast = useToast()
  return (
    <div><PageHeader title="گارانتی‌های من" actions={<Link to="/guarantee" className="btn-soft btn-sm"><Plus className="h-4 w-4" />ثبت گارانتی</Link>} />
      {q.isLoading ? <Loading /> : !q.data?.length ? <Empty title="گارانتی ثبت‌شده‌ای ندارید" hint="گارانتی خریدهای آنلاین به‌صورت خودکار اضافه می‌شود." /> : (
        <div className="space-y-3">{q.data.map((g) => (
          <Card key={g.id} className="p-4">
            <div className="flex items-start justify-between gap-3"><div><div className="font-bold">{g.product_name}</div><div className="text-xs text-soft num" dir="ltr">{g.code}</div></div><div className="flex gap-1"><StatusBadge map={guaranteeStatus} value={g.status} />{g.tier === 'golden' && <StatusBadge map={tierLabel} value="golden" />}</div></div>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-sm"><span className="text-soft">پایان گارانتی: {date(g.expiry_date)} {g.days_remaining >= 0 ? `(${fa(g.days_remaining)} روز مانده)` : '(منقضی)'}</span>
              {['approved', 'renewed', 'expired'].includes(g.status) && <button className="btn-soft btn-sm" onClick={() => setSvc(g)}><Wrench className="h-4 w-4" />درخواست سرویس</button>}</div>
          </Card>))}</div>
      )}
      <Modal open={!!svc} onClose={() => setSvc(null)} title="درخواست سرویس" footer={<ActionButton onClick={async () => { await post('/my/service-requests', { guarantee_id: svc.id, complaint: text }); toast.ok('درخواست ثبت شد؛ همکاران ما تماس می‌گیرند'); setSvc(null); setText('') }}>ثبت درخواست</ActionButton>}>
        <p className="mb-3 text-sm text-soft">{svc?.product_name}</p>
        <Field label="مشکل دستگاه را توضیح دهید"><textarea className="input" rows={4} value={text} onChange={(e) => setText(e.target.value)} /></Field>
      </Modal>
    </div>
  )
}

export function Addresses() {
  const q = useGet<any[]>('/shop/addresses')
  const inv = useInvalidate()
  const [edit, setEdit] = useState<any>(null)
  return (
    <div><PageHeader title="نشانی‌های من" actions={<button className="btn-primary btn-sm" onClick={() => setEdit({ recipient: '', phone: '', province: '', city: '', address: '', postal_code: '', title: '', is_default: false })}><Plus className="h-4 w-4" />نشانی جدید</button>} />
      {q.isLoading ? <Loading /> : !q.data?.length ? <Empty title="نشانی ثبت نشده است" /> : (
        <div className="grid gap-3 sm:grid-cols-2">{q.data.map((a) => (
          <Card key={a.id} className="p-4 text-sm"><div className="font-bold">{a.recipient}</div><div className="num text-soft" dir="ltr">{a.phone}</div><div className="mt-1">{a.province} {a.city}، {a.address}</div>
            <div className="mt-3 flex gap-2"><button className="btn-ghost btn-sm" onClick={() => setEdit(a)}>ویرایش</button><ActionButton className="btn-ghost btn-sm text-bad" confirm="حذف شود؟" onClick={async () => { await del(`/shop/addresses/${a.id}`); inv('/shop/addresses') }}>حذف</ActionButton></div></Card>))}</div>
      )}
      <Modal open={!!edit} onClose={() => setEdit(null)} title="نشانی" footer={<ActionButton onClick={async () => { edit.id ? await put(`/shop/addresses/${edit.id}`, edit) : await post('/shop/addresses', edit); inv('/shop/addresses'); setEdit(null) }}>ذخیره</ActionButton>}>
        {edit && <div className="grid gap-3 sm:grid-cols-2">
          <Field label="نام گیرنده"><input className="input" value={edit.recipient} onChange={(e) => setEdit({ ...edit, recipient: e.target.value })} /></Field>
          <Field label="موبایل"><NumInput value={edit.phone} onChange={(v) => setEdit({ ...edit, phone: v })} /></Field>
          <Field label="استان"><input className="input" value={edit.province} onChange={(e) => setEdit({ ...edit, province: e.target.value })} /></Field>
          <Field label="شهر"><input className="input" value={edit.city} onChange={(e) => setEdit({ ...edit, city: e.target.value })} /></Field>
          <Field label="نشانی" className="sm:col-span-2"><textarea className="input" rows={2} value={edit.address} onChange={(e) => setEdit({ ...edit, address: e.target.value })} /></Field>
          <Field label="کد پستی"><NumInput value={edit.postal_code} onChange={(v) => setEdit({ ...edit, postal_code: v })} /></Field>
          <label className="flex items-center gap-2 pt-6 text-sm"><input type="checkbox" checked={edit.is_default} onChange={(e) => setEdit({ ...edit, is_default: e.target.checked })} />نشانی پیش‌فرض</label>
        </div>}
      </Modal>
    </div>
  )
}

export function Profile() {
  const u = useUser()
  const [cur, setCur] = useState('')
  const [np, setNp] = useState('')
  return (
    <div className="max-w-md space-y-4"><PageHeader title="پروفایل" />
      <Card className="p-4"><KV k="نام" v={u?.full_name || '—'} /><KV k="موبایل" v={<span dir="ltr" className="num">{u?.phone}</span>} /></Card>
      <Card className="space-y-3 p-4"><div className="font-bold">تعیین / تغییر گذرواژه</div>
        <p className="text-xs text-soft">ورود با کد پیامکی همیشه ممکن است؛ گذرواژه اختیاری است.</p>
        <Field label="گذرواژه فعلی (اگر دارید)"><input className="input" type="password" value={cur} onChange={(e) => setCur(e.target.value)} /></Field>
        <Field label="گذرواژه جدید"><input className="input" type="password" value={np} onChange={(e) => setNp(e.target.value)} /></Field>
        <ActionButton success="گذرواژه ذخیره شد؛ دوباره وارد شوید" onClick={async () => { await post('/auth/change-password', { current_password: cur, new_password: np }); setTimeout(() => (location.href = '/login'), 1200) }}>ذخیره</ActionButton>
      </Card>
    </div>
  )
}

export function Wishlist() {
  const q = useGet<any[]>('/shop/wishlist')
  return (
    <div><PageHeader title="علاقه‌مندی‌ها" />
      {q.isLoading ? <Loading /> : !q.data?.length ? <Empty title="فهرست علاقه‌مندی‌ها خالی است" hint="روی قلب کنار هر محصول بزنید تا اینجا ذخیره شود." /> : (
        <div className="grid grid-cols-2 gap-3 md:grid-cols-3">{q.data.map((p) => <ProductCard key={p.id} p={{ ...p, in_stock: true }} />)}</div>
      )}
    </div>
  )
}
