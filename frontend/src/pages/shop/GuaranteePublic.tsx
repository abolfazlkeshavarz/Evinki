import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { CheckCircle2, ShieldCheck, XCircle } from 'lucide-react'
import { ActionButton, Card, Field, ImageUpload, JalaliInput, KV, NumInput, StatusBadge, Tabs, useToast } from '../../components/ui'
import { get, post } from '../../lib/api'
import { date, fa, todayISO } from '../../lib/format'
import { guaranteeStatus, tierLabel } from '../../lib/labels'
import { useGet } from '../../lib/hooks'

export function GuaranteePublic() {
  const [sp] = useSearchParams()
  const [tab, setTab] = useState<'check' | 'register'>('check')
  const [code, setCode] = useState(sp.get('code') ?? '')
  const [res, setRes] = useState<any>(null)
  const [busy, setBusy] = useState(false)
  const check = async (c = code) => {
    if (!c.trim()) return
    setBusy(true)
    try { setRes(await get('/public/guarantees/check?code=' + encodeURIComponent(c))) } finally { setBusy(false) }
  }
  useEffect(() => { if (sp.get('code')) check(sp.get('code')!) }, [])
  return (
    <div className="mx-auto max-w-xl">
      <div className="mb-5 text-center"><div className="mx-auto mb-3 grid h-14 w-14 place-items-center rounded-2xl bg-brand-tint text-brand"><ShieldCheck className="h-8 w-8" /></div><h1 className="text-2xl font-extrabold">گارانتی اوینکی</h1><p className="mt-1 text-sm text-soft">وضعیت گارانتی را بررسی کنید یا دستگاه تازه‌خریده را ثبت کنید.</p></div>
      <Tabs value={tab} onChange={setTab} items={[{ key: 'check', label: 'استعلام' }, { key: 'register', label: 'ثبت گارانتی' }]} />
      {tab === 'check' ? (
        <Card className="space-y-4 p-5">
          <form className="flex gap-2" onSubmit={(e) => { e.preventDefault(); check() }}>
            <input className="input" dir="ltr" placeholder="کد گارانتی" value={code} onChange={(e) => setCode(e.target.value)} aria-label="کد گارانتی" />
            <button className="btn-primary" disabled={busy}>استعلام</button>
          </form>
          {res && !res.found && <div className="flex items-start gap-2 rounded-xl bg-bad/10 p-4 text-sm text-bad"><XCircle className="mt-0.5 h-5 w-5 shrink-0" />کدی با این مشخصات ثبت نشده است. اگر دستگاه را تازه خریده‌اید، از تب «ثبت گارانتی» آن را ثبت کنید.</div>}
          {res?.found && (
            <div>
              <div className="mb-3 flex items-center gap-2"><CheckCircle2 className="h-6 w-6 text-ok" /><span className="font-bold">گارانتی ثبت شده است</span><StatusBadge map={guaranteeStatus} value={res.status} />{res.tier === 'golden' && <StatusBadge map={tierLabel} value="golden" />}</div>
              <KV k="محصول" v={res.product_name} /><KV k="دارنده" v={res.owner} /><KV k="تاریخ خرید" v={date(res.purchase_date)} /><KV k="پایان گارانتی" v={date(res.expiry_date)} />
              <KV k="روز باقی‌مانده" v={res.days_remaining >= 0 ? fa(res.days_remaining) + ' روز' : 'منقضی شده'} />
            </div>
          )}
        </Card>
      ) : <Register initialCode={code} />}
    </div>
  )
}

function Register({ initialCode }: { initialCode: string }) {
  const products = useGet<any[]>('/lookup/products')
  const toast = useToast()
  const [done, setDone] = useState(false)
  const [f, setF] = useState({ code: initialCode, product_id: 0, purchase_date: todayISO(), full_name: '', phone: '', national_id: '', city: '', invoice_file: '', card_file: '' })
  const [chk, setChk] = useState<any>(null)
  useEffect(() => {
    setChk(null)
    if (!f.product_id || f.code.length < 4) return
    const t = setTimeout(async () => { try { setChk(await post('/public/guarantees/validate', { product_id: f.product_id, code: f.code })) } catch { /* ignore */ } }, 400)
    return () => clearTimeout(t)
  }, [f.code, f.product_id])
  if (done) return <Card className="p-8 text-center"><CheckCircle2 className="mx-auto mb-3 h-12 w-12 text-ok" /><h2 className="text-lg font-bold">گارانتی شما ثبت شد</h2><p className="mt-2 text-sm text-soft">پس از بررسی، با پیامک به شما اطلاع می‌دهیم.</p></Card>
  return (
    <Card className="space-y-4 p-5">
      <Field label="محصول">
        <select className="input" value={f.product_id} onChange={(e) => setF({ ...f, product_id: +e.target.value })}>
          <option value={0}>انتخاب کنید…</option>{products.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </select>
      </Field>
      <Field label="کد گارانتی"><input className="input" dir="ltr" value={f.code} onChange={(e) => setF({ ...f, code: e.target.value })} /></Field>
      {chk && <p className={`rounded-lg p-2.5 text-sm ${chk.message_level === 'success' ? 'bg-ok/10 text-ok' : chk.message_level === 'warning' ? 'bg-warn/15 text-warn' : 'bg-bad/10 text-bad'}`}>{chk.message}</p>}
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="تاریخ خرید (شمسی)"><JalaliInput value={f.purchase_date} onChange={(v) => setF({ ...f, purchase_date: v })} /></Field>
        <Field label="نام و نام خانوادگی"><input className="input" value={f.full_name} onChange={(e) => setF({ ...f, full_name: e.target.value })} /></Field>
        <Field label="موبایل"><NumInput value={f.phone} onChange={(v) => setF({ ...f, phone: v })} placeholder="09123456789" /></Field>
        <Field label="کد ملی"><NumInput value={f.national_id} onChange={(v) => setF({ ...f, national_id: v })} /></Field>
        <Field label="شهر"><input className="input" value={f.city} onChange={(e) => setF({ ...f, city: e.target.value })} /></Field>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="عکس فاکتور خرید *" hint="الزامی؛ عکس واضح از فاکتور یا رسید خرید"><ImageUpload value={f.invoice_file} onChange={(u) => setF({ ...f, invoice_file: u })} purpose="invoice" /></Field>
        <Field label="عکس کارت گارانتی *" hint="الزامی؛ عکس کارت گارانتی که همراه دستگاه است"><ImageUpload value={f.card_file} onChange={(u) => setF({ ...f, card_file: u })} purpose="guarantee_card" /></Field>
      </div>
      <ActionButton className="btn-primary w-full" disabled={chk ? !chk.can_register : false} onClick={async () => {
        if (!f.product_id) { toast.info('محصول را انتخاب کنید'); return }
        if (!f.invoice_file || !f.card_file) { toast.info('عکس فاکتور و عکس کارت گارانتی را بارگذاری کنید'); return }
        await post('/public/guarantees/register', f); setDone(true)
      }}>ثبت گارانتی</ActionButton>
    </Card>
  )
}
