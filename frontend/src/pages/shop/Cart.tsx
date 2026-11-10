import { useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { Minus, Plus, Trash2, Package } from 'lucide-react'
import { ActionButton, Card, Empty, ErrorBox, Field, Loading, NumInput, useToast } from '../../components/ui'
import { del, post, put } from '../../lib/api'
import { toman, fa } from '../../lib/format'
import { useGet, useInvalidate } from '../../lib/hooks'
import { useUser } from '../../lib/auth'

function Summary({ c }: { c: any }) {
  return (
    <div className="space-y-2 text-sm">
      <div className="flex justify-between"><span className="text-soft">جمع کالاها</span><span className="num">{toman(c.subtotal)}</span></div>
      {c.discount > 0 && <div className="flex justify-between text-ok"><span>تخفیف ({c.coupon})</span><span className="num">- {toman(c.discount)}</span></div>}
      <div className="flex justify-between"><span className="text-soft">هزینه ارسال</span><span className="num">{c.shipping_fee ? toman(c.shipping_fee) : 'رایگان'}</span></div>
      <div className="flex justify-between border-t border-line pt-3 text-base font-bold"><span>مبلغ قابل پرداخت</span><span className="num">{toman(c.total)}</span></div>
    </div>
  )
}

export function Cart() {
  const cart = useGet<any>('/shop/cart')
  const inv = useInvalidate()
  const toast = useToast()
  const user = useUser()
  const [code, setCode] = useState('')
  if (cart.isLoading) return <Loading />
  if (cart.error) return <ErrorBox error={cart.error} />
  const c = cart.data
  if (!c.lines.length) return <Empty title="سبد خرید شما خالی است" action={<Link className="btn-primary" to="/products">مشاهده محصولات</Link>} />
  const setQty = async (l: any, qty: number) => {
    try { await put('/shop/cart/items', { product_id: l.product_id ?? undefined, part_id: l.part_id ?? undefined, qty }); inv('/shop/cart') } catch (e) { toast.error(e) }
  }
  return (
    <div className="grid gap-6 md:grid-cols-[1fr_340px]">
      <Card className="divide-y divide-line">
        {c.lines.map((l: any) => (
          <div key={l.id} className="flex items-center gap-3 p-4">
            <div className="grid h-16 w-16 shrink-0 place-items-center overflow-hidden rounded-xl bg-muted">{l.image ? <img src={l.image} alt="" className="h-full w-full object-cover" /> : <Package className="h-6 w-6 text-soft" />}</div>
            <div className="min-w-0 flex-1">
              <div className="truncate font-semibold">{l.name}</div>
              <div className="text-sm text-soft num">{toman(l.unit_price)}</div>
              {(!l.orderable || l.qty > l.available) && <div className="text-xs text-bad">{!l.orderable ? 'در حال حاضر قابل سفارش نیست' : `فقط ${fa(l.available)} عدد موجود است`}</div>}
            </div>
            <div className="flex items-center gap-1 rounded-xl border border-line">
              <button className="p-2.5" aria-label="کم کردن" onClick={() => setQty(l, l.qty - 1)}><Minus className="h-4 w-4" /></button>
              <span className="w-6 text-center text-sm font-semibold num">{fa(l.qty)}</span>
              <button className="p-2.5" aria-label="زیاد کردن" onClick={() => setQty(l, l.qty + 1)}><Plus className="h-4 w-4" /></button>
            </div>
            <button className="p-2 text-bad" aria-label="حذف" onClick={() => setQty(l, 0)}><Trash2 className="h-5 w-5" /></button>
          </div>
        ))}
      </Card>
      <Card className="h-fit space-y-4 p-5">
        <div className="flex gap-2">
          <input className="input" placeholder="کد تخفیف" dir="ltr" value={code} onChange={(e) => setCode(e.target.value)} />
          <ActionButton className="btn-ghost" onClick={async () => { await post('/shop/cart/coupon', { code }); inv('/shop/cart'); setCode('') }}>اعمال</ActionButton>
        </div>
        <Summary c={c} />
        <Link className="btn-primary w-full" to={user ? '/checkout' : '/login'} state={{ from: '/checkout' }}>{user ? 'ادامه و پرداخت' : 'ورود و ادامه خرید'}</Link>
      </Card>
    </div>
  )
}

export function Checkout() {
  const cart = useGet<any>('/shop/cart')
  const addrs = useGet<any[]>('/shop/addresses')
  const nav = useNavigate()
  const [sel, setSel] = useState<number | 'new' | null>(null)
  const [note, setNote] = useState('')
  const [f, setF] = useState({ recipient: '', phone: '', province: '', city: '', address: '', postal_code: '' })
  if (cart.isLoading || addrs.isLoading) return <Loading />
  const c = cart.data
  if (!c?.lines?.length) return <Empty title="سبد خرید خالی است" action={<Link className="btn-primary" to="/products">محصولات</Link>} />
  const list = addrs.data ?? []
  const chosen = sel ?? (list.length ? (list.find((a) => a.is_default) ?? list[0]).id : 'new')
  return (
    <div className="grid gap-6 md:grid-cols-[1fr_340px]">
      <div className="space-y-4">
        <h1 className="text-xl font-bold">نشانی تحویل</h1>
        {list.map((a) => (
          <label key={a.id} className={`card flex cursor-pointer items-start gap-3 p-4 ${chosen === a.id ? 'border-brand ring-1 ring-brand' : ''}`}>
            <input type="radio" name="addr" checked={chosen === a.id} onChange={() => setSel(a.id)} className="mt-1.5" />
            <div className="text-sm"><div className="font-semibold">{a.recipient} <span className="font-normal text-soft num" dir="ltr">{a.phone}</span></div><div className="text-soft">{a.province} {a.city}، {a.address}</div></div>
          </label>
        ))}
        <label className={`card flex cursor-pointer items-center gap-3 p-4 ${chosen === 'new' ? 'border-brand ring-1 ring-brand' : ''}`}>
          <input type="radio" name="addr" checked={chosen === 'new'} onChange={() => setSel('new')} /><span className="text-sm font-semibold">نشانی جدید</span>
        </label>
        {chosen === 'new' && (
          <Card className="grid gap-3 p-4 sm:grid-cols-2">
            <Field label="نام گیرنده"><input className="input" value={f.recipient} onChange={(e) => setF({ ...f, recipient: e.target.value })} /></Field>
            <Field label="موبایل گیرنده"><NumInput value={f.phone} onChange={(v) => setF({ ...f, phone: v })} /></Field>
            <Field label="استان"><input className="input" value={f.province} onChange={(e) => setF({ ...f, province: e.target.value })} /></Field>
            <Field label="شهر"><input className="input" value={f.city} onChange={(e) => setF({ ...f, city: e.target.value })} /></Field>
            <Field label="نشانی کامل" className="sm:col-span-2"><textarea className="input" rows={2} value={f.address} onChange={(e) => setF({ ...f, address: e.target.value })} /></Field>
            <Field label="کد پستی"><NumInput value={f.postal_code} onChange={(v) => setF({ ...f, postal_code: v })} /></Field>
          </Card>
        )}
        <Field label="توضیحات سفارش (اختیاری)"><textarea className="input" rows={2} value={note} onChange={(e) => setNote(e.target.value)} /></Field>
      </div>
      <Card className="h-fit space-y-4 p-5">
        <h2 className="font-bold">خلاصه سفارش</h2>
        <Summary c={c} />
        <ActionButton className="btn-primary w-full" onClick={async () => {
          const body = chosen === 'new' ? { address: f, note } : { address_id: chosen, note }
          const r = await post('/shop/checkout', body)
          window.location.href = r.payment_url
        }}>پرداخت آنلاین</ActionButton>
        <p className="text-xs text-soft">با پرداخت، به درگاه امن بانکی منتقل می‌شوید. <Link to="/cart" className="text-brand">بازگشت به سبد</Link></p>
      </Card>
    </div>
  )
}

void del; void nav_unused
function nav_unused() {}
