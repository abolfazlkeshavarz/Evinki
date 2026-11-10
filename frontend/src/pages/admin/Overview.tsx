import { Link } from 'react-router-dom'
import { AlertCircle } from 'lucide-react'
import { Card, ErrorBox, Loading, PageHeader, Stat } from '../../components/ui'
import { fa, toman } from '../../lib/format'
import { useGet } from '../../lib/hooks'
import { useUser } from '../../lib/auth'

function Todo({ n, label, to }: { n: number; label: string; to: string }) {
  if (!n) return null
  return (
    <Link to={to} className="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface px-4 py-3 transition hover:border-brand">
      <span className="flex items-center gap-2 text-sm font-medium"><AlertCircle className="h-4 w-4 text-warn" />{label}</span>
      <span className="rounded-full bg-warn/15 px-2.5 py-0.5 text-sm font-bold text-warn num">{fa(n)}</span>
    </Link>
  )
}

export function Overview() {
  const q = useGet<any>('/manager/overview', { refetchInterval: 60_000 })
  const u = useUser()
  if (q.isLoading) return <Loading />
  if (q.error) return <ErrorBox error={q.error} retry={() => q.refetch()} />
  const d = q.data
  const todos = d.service.awaiting_review + d.service.requests_pending + d.service.shipments_to_receive + d.warranty.pending_approval + d.warehouse.pending_documents + d.hr.pending_leaves + d.service.technicians_pending + d.shop.to_ship + d.service.unassigned
  return (
    <div className="space-y-6">
      <PageHeader title={`سلام ${u?.full_name?.split(' ')[0] ?? ''} 👋`} subtitle="خلاصه وضعیت امروز" />
      <section>
        <h2 className="mb-2 text-sm font-bold text-soft">نیاز به اقدام شما {todos === 0 && '— همه چیز مرتب است ✓'}</h2>
        <div className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
          <Todo n={d.service.awaiting_review} label="گزارش تعمیر منتظر بررسی" to="/admin/service" />
          <Todo n={d.service.unassigned} label="درخواست سرویس بدون تکنسین" to="/admin/service" />
          <Todo n={d.service.requests_pending} label="درخواست قطعه منتظر تایید" to="/admin/part-requests" />
          <Todo n={d.service.shipments_to_receive} label="مرسوله قطعه در انتظار دریافت" to="/admin/shipments" />
          <Todo n={d.service.technicians_pending} label="تکنسین منتظر تایید" to="/admin/technicians" />
          <Todo n={d.warranty.pending_approval} label="گارانتی منتظر تایید" to="/admin/guarantees" />
          <Todo n={d.shop.to_ship} label="سفارش آماده ارسال" to="/admin/orders" />
          <Todo n={d.warehouse.pending_documents} label="سند انبار منتظر تایید" to="/admin/warehouse/documents" />
          <Todo n={d.hr.pending_leaves} label="درخواست مرخصی" to="/admin/hr/leaves" />
        </div>
      </section>
      <section>
        <h2 className="mb-2 text-sm font-bold text-soft">فروشگاه</h2>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Stat label="سفارش امروز" value={fa(d.shop.orders_today)} /><Stat label="فروش امروز" value={toman(d.shop.revenue_today)} />
          <Stat label="فروش این ماه" value={toman(d.shop.revenue_month)} /><Stat label="در انتظار ارسال" value={fa(d.shop.to_ship)} tone={d.shop.to_ship ? 'warn' : undefined} to="/admin/orders" />
        </div>
      </section>
      <section>
        <h2 className="mb-2 text-sm font-bold text-soft">گارانتی و خدمات</h2>
        <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
          <Stat label="گارانتی فعال" value={fa(d.warranty.active)} /><Stat label="تا ۳۰ روز آینده منقضی" value={fa(d.warranty.expiring_30d)} />
          <Stat label="تعمیر در جریان" value={fa(d.service.open)} to="/admin/service" /><Stat label="بدهی به تکنسین‌ها" value={toman(d.service.payable)} tone={d.service.payable ? 'warn' : undefined} to="/admin/shipments" />
        </div>
      </section>
      <section className="grid gap-3 lg:grid-cols-3">
        <Card className="p-4"><h3 className="mb-2 text-sm font-bold text-soft">انبار</h3><div className="grid grid-cols-2 gap-3"><Stat label="کمبود موجودی" value={fa(d.warehouse.low_stock)} tone={d.warehouse.low_stock ? 'bad' : undefined} to="/admin/warehouse?low=1" /><Stat label="سند منتظر تایید" value={fa(d.warehouse.pending_documents)} to="/admin/warehouse/documents" /></div></Card>
        <Card className="p-4"><h3 className="mb-2 text-sm font-bold text-soft">حضور امروز</h3><div className="grid grid-cols-2 gap-3"><Stat label="حاضر" value={`${fa(d.hr.arrived_today)} از ${fa(d.hr.employees)}`} to="/admin/hr/report" /><Stat label="مرخصی در انتظار" value={fa(d.hr.pending_leaves)} to="/admin/hr/leaves" /></div></Card>
        <Card className="p-4"><h3 className="mb-2 text-sm font-bold text-soft">رضایت مشتریان (۳۰ روز)</h3><div className="grid grid-cols-2 gap-3"><Stat label="میانگین امتیاز" value={d.surveys.avg_rating_30d ? fa(d.surveys.avg_rating_30d) + ' از ۵' : '—'} to="/admin/surveys" /><Stat label="پاسخ دریافتی" value={fa(d.surveys.responses_30d)} /></div></Card>
      </section>
    </div>
  )
}
