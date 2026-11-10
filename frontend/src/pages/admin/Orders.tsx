import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ActionButton, Card, DataTable, Empty, ErrorBox, KV, Loading, Modal, PageHeader, Pager, SearchBox, StatusBadge, Tabs } from '../../components/ui'
import { post, put } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { dateTime, fa, toman } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { orderStatus } from '../../lib/labels'

export function Orders() {
  const [status, setStatus] = useState('')
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const nav = useNavigate()
  const list = useGet<any>('/admin/orders' + qs({ status, q, page, limit: 20 }))
  return (
    <div>
      <PageHeader title="سفارش‌ها" />
      <Tabs value={status} onChange={(s) => { setStatus(s); setPage(1) }} items={[{ key: '', label: 'همه' }, ...['paid', 'processing', 'shipped', 'delivered', 'pending_payment', 'cancelled'].map((k) => ({ key: k, label: orderStatus[k][0] }))]} />
      <Card>
        <div className="border-b border-line p-3"><SearchBox value={q} onChange={(v) => { setQ(v); setPage(1) }} placeholder="شماره سفارش، نام یا موبایل" /></div>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data.items.length ? <Empty title="سفارشی پیدا نشد" /> : (
          <>
            <DataTable rows={list.data.items} rowKey={(r: any) => r.id} onRow={(r: any) => nav(`/admin/orders/${r.id}`)} cols={[
              { head: 'شماره', cell: (r: any) => <b className="num">{r.number}</b> }, { head: 'مشتری', cell: (r: any) => r.customer_name }, { head: 'تاریخ', cell: (r: any) => dateTime(r.created_at) },
              { head: 'اقلام', cell: (r: any) => fa(r.item_count) }, { head: 'مبلغ', cell: (r: any) => <span className="num">{toman(r.total)}</span> }, { head: 'وضعیت', cell: (r: any) => <StatusBadge map={orderStatus} value={r.status} /> }]} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} />
          </>
        )}
      </Card>
    </div>
  )
}

export function OrderDetail() {
  const { id } = useParams()
  const q = useGet<any>(`/admin/orders/${id}`)
  const can = useCan()
  const inv = useInvalidate()
  const [ship, setShip] = useState<string | null>(null)
  const [track, setTrack] = useState('')
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} />
  const { order: o, items, payments } = q.data
  const act = async (status: string, tracking_code = '') => { await post(`/admin/orders/${id}/status`, { status, tracking_code }); inv('/admin/orders') }
  const manage = can('shop.manage')
  const next = { paid: ['processing', 'shipped'], processing: ['shipped'], shipped: ['delivered'] }[o.status as string] as string[] | undefined
  return (
    <div className="space-y-4">
      <PageHeader title={`سفارش ${o.number}`} subtitle={dateTime(o.created_at)} actions={<><StatusBadge map={orderStatus} value={o.status} /><Link to="/admin/orders" className="btn-ghost btn-sm">بازگشت</Link></>} />
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <div className="border-b border-line p-4 font-bold">اقلام</div>
          {items.map((i: any) => (
            <div key={i.id} className="flex flex-wrap items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0">
              <div><b>{i.name}</b> × {fa(i.qty)}<div className="text-xs text-soft num">{toman(i.unit_price)}</div></div>
              <div className="flex items-center gap-2">
                {i.product_id && manage && ['paid', 'processing', 'shipped'].includes(o.status) && (
                  <input className="input w-44 text-xs" dir="ltr" placeholder="سریال / کد گارانتی دستگاه" defaultValue={i.serial_code} onBlur={async (e) => { if (e.target.value !== i.serial_code) { await put(`/admin/orders/${id}/items/${i.id}/serial`, { serial_code: e.target.value }); inv('/admin/orders') } }} />
                )}
                {i.product_id && !manage && i.serial_code && <span dir="ltr" className="text-xs">{i.serial_code}</span>}
                <b className="num">{toman(i.total)}</b>
              </div>
            </div>
          ))}
          <div className="p-4 text-xs text-soft">با ثبت سریال دستگاه، پس از «تحویل شد» گارانتی مشتری خودکار فعال می‌شود.</div>
        </Card>
        <div className="space-y-4">
          <Card className="p-4"><KV k="مشتری" v={o.customer_name} /><KV k="گیرنده" v={o.address?.recipient} /><KV k="موبایل" v={<span dir="ltr" className="num">{o.address?.phone}</span>} /><KV k="نشانی" v={`${o.address?.province ?? ''} ${o.address?.city ?? ''}، ${o.address?.address ?? ''}`} />{o.note && <KV k="توضیح مشتری" v={o.note} />}</Card>
          <Card className="p-4"><KV k="جمع کالاها" v={toman(o.subtotal)} /><KV k="تخفیف" v={toman(o.discount)} /><KV k="ارسال" v={toman(o.shipping_fee)} /><KV k="مبلغ نهایی" v={<b>{toman(o.total)}</b>} />{o.tracking_code && <KV k="کد رهگیری" v={<span dir="ltr">{o.tracking_code}</span>} />}</Card>
          {payments.map((p: any, i: number) => <Card key={i} className="p-4 text-sm"><KV k="پرداخت" v={p.status === 'paid' ? 'موفق' : p.status === 'failed' ? 'ناموفق' : 'آغاز شده'} /><KV k="کد پیگیری بانک" v={<span dir="ltr">{p.ref_id || '—'}</span>} /></Card>)}
          {manage && next && (
            <Card className="space-y-2 p-4">
              {next.includes('processing') && <ActionButton className="btn-soft w-full" onClick={() => act('processing')}>شروع آماده‌سازی</ActionButton>}
              {next.includes('shipped') && <button className="btn-primary w-full" onClick={() => setShip('')}>ثبت ارسال</button>}
              {next.includes('delivered') && <ActionButton className="btn-primary w-full" confirm="تحویل مشتری ثبت شود؟ گارانتی دستگاه‌ها فعال می‌شود." onClick={() => act('delivered')}>ثبت تحویل</ActionButton>}
              {['paid', 'processing'].includes(o.status) && <ActionButton className="btn-ghost w-full text-bad" confirm="سفارش لغو و موجودی بازگردانده شود؟" onClick={() => act('cancelled')}>لغو سفارش</ActionButton>}
            </Card>
          )}
        </div>
      </div>
      <Modal open={ship !== null} onClose={() => setShip(null)} title="ثبت ارسال" footer={<ActionButton onClick={async () => { await act('shipped', track); setShip(null) }}>ثبت و اطلاع به مشتری</ActionButton>}>
        <label className="label">کد رهگیری پستی</label><input className="input" dir="ltr" value={track} onChange={(e) => setTrack(e.target.value)} />
      </Modal>
    </div>
  )
}
