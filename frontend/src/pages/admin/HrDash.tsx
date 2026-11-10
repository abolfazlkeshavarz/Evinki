import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertCircle, CalendarClock, ClipboardList, DoorOpen, Download, MonitorSmartphone, Plus, ScanFace, Timer, Trash2, UserCheck, UserX, Users } from 'lucide-react'
import { ActionButton, Badge, Card, DataTable, Empty, ErrorBox, Field, JalaliInput, Loading, Modal, PageHeader, Pager, Stat, StatusBadge, Tabs } from '../../components/ui'
import { ArrivalsChart, DeptChart, HoursChart, PresenceArea, RankList, StatusDonut, TrendChart } from '../../components/HrCharts'
import { Avatar } from './HrEmployees'
import { del, download, post } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { dateTime, fa, faDigits, jalaliParts, jalaliToISO, minutes, todayISO } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { dayStatus } from '../../lib/labels'

const hhmm = (m: number) => (m ? `${fa(Math.floor(m / 60))}:${String(m % 60).padStart(2, '0').replace(/\d/g, (d) => fa(d))}` : '—')

// ───────────────────────────── dashboard ─────────────────────────────

export function HrDashboard() {
  const d = useGet<any>('/hr/dashboard', { refetchInterval: 60_000 })
  const t = useGet<any[]>('/hr/today', { refetchInterval: 60_000 })
  if (d.isLoading) return <Loading />
  if (d.error) return <ErrorBox error={d.error} retry={() => d.refetch()} />
  const x = d.data, c = x.counters ?? {}
  const rows = t.data ?? []
  const inside = rows.filter((r) => r.inside)
  const late = rows.filter((r) => r.late_minutes > 0)
  return (
    <div className="space-y-5">
      <PageHeader title="داشبورد حضور و غیاب" subtitle={faDigits(x.date)} actions={<Link to="/kiosk" target="_blank" className="btn-soft"><MonitorSmartphone className="h-4 w-4" />باز کردن کیوسک ورود</Link>} />
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <Stat label="کل پرسنل فعال" value={fa(x.employees)} />
        <Stat label="حاضران امروز" value={fa(x.arrived_today)} hint={`${fa(x.inside_now)} نفر هم‌اکنون داخل`} tone="ok" />
        <Stat label="غایبان امروز" value={fa(c.absent ?? 0)} hint={`${fa(c.leave ?? 0)} نفر در مرخصی`} tone="bad" />
        <Stat label="تأخیر امروز" value={fa(x.late_today)} hint="ورود دیرتر از شیفت" tone="warn" />
      </div>
      <div className="grid gap-5 lg:grid-cols-3">
        <Card className="p-4 lg:col-span-2"><h3 className="mb-3 font-bold">روند حضور و غیاب هفت روز گذشته</h3><PresenceArea series={x.trend} /></Card>
        <Card className="p-4"><h3 className="mb-3 font-bold">ترکیب وضعیت امروز</h3><StatusDonut counters={c} /></Card>
      </div>
      <div className="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <Stat label="وظایف باز" value={fa(x.open_tasks)} hint={`${fa(x.overdue_tasks)} مورد از موعد گذشته`} tone={x.overdue_tasks ? 'warn' : undefined} to="/admin/hr/tasks" />
        <Stat label="مرخصی در انتظار تأیید" value={fa(x.pending_leaves)} tone={x.pending_leaves ? 'warn' : undefined} to="/admin/hr/leaves" />
        <Stat label="بدون ثبت چهره" value={fa(x.face_not_enrolled)} hint="روی کیوسک شناسایی نمی‌شوند" tone={x.face_not_enrolled ? 'bad' : 'ok'} to="/admin/hr/employees" />
        <Stat label="دستگاه‌های آفلاین" value={`${fa(x.devices_offline)} / ${fa(x.devices)}`} tone={x.devices_offline ? 'bad' : 'ok'} to="/admin/hr/devices" />
      </div>
      <div className="grid gap-5 lg:grid-cols-2">
        <Card className="p-4"><h3 className="font-bold">حاضران داخل کارخانه</h3><p className="mb-2 text-xs text-soft">ورود زده‌اند و هنوز خروج نزده‌اند</p>
          {!inside.length ? <Empty title="کسی داخل کارخانه نیست" /> : <ul className="divide-y divide-line/60">{inside.slice(0, 8).map((r) => (
            <li key={r.employee_id} className="flex items-center gap-3 py-2.5"><Avatar name={r.name} src={r.photo} /><div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{r.name}</p><p className="text-xs text-soft">{r.department || 'بدون واحد'}</p></div>
              <div className="text-end"><p className="text-sm font-medium text-ok num">{faDigits(r.first_in)}</p><p className="text-xs text-soft">{hhmm(r.worked_minutes)} کارکرد</p></div></li>))}</ul>}
        </Card>
        <Card className="p-4"><h3 className="font-bold">تأخیرهای امروز</h3><p className="mb-2 text-xs text-soft">ورود دیرتر از ساعت شیفت</p>
          {!late.length ? <Empty title="امروز تأخیری ثبت نشده" /> : <ul className="divide-y divide-line/60">{late.slice(0, 8).map((r) => (
            <li key={r.employee_id} className="flex items-center gap-3 py-2.5"><Avatar name={r.name} src={r.photo} /><div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{r.name}</p><p className="text-xs text-soft">ورود {faDigits(r.first_in)}</p></div>
              <Badge tone="warn"><AlertCircle className="me-1 h-3.5 w-3.5" />{fa(r.late_minutes)} دقیقه</Badge></li>))}</ul>}
        </Card>
      </div>
      <div className="grid gap-5 lg:grid-cols-2">
        <Card className="p-4"><h3 className="mb-3 font-bold">ساعت ورود (هفت روز گذشته)</h3><ArrivalsChart arrivals={x.arrivals} /></Card>
        <Card className="p-4"><h3 className="mb-3 font-bold">پرسنل به تفکیک واحد</h3>
          <ul className="space-y-2.5">{(x.departments ?? []).map((dp: any) => (<li key={dp.name}><div className="mb-1 flex justify-between text-sm"><span>{dp.name}</span><b className="num">{fa(dp.count)}</b></div><div className="h-2 overflow-hidden rounded-full bg-muted"><div className="h-full bg-brand" style={{ width: `${(dp.count / Math.max(1, x.employees)) * 100}%` }} /></div></li>))}</ul></Card>
      </div>
      <Card>
        <div className="border-b border-line p-4 font-bold">وضعیت امروز به تفکیک پرسنل</div>
        <DataTable rows={rows} rowKey={(r: any) => r.employee_id} cols={[
          { head: 'پرسنل', cell: (r: any) => <div className="flex items-center gap-2.5"><Avatar name={r.name} src={r.photo} size={30} /><div><b>{r.name}</b><div className="text-xs text-soft num">{r.personnel_code}</div></div></div> },
          { head: 'واحد', cell: (r: any) => r.department || '—' }, { head: 'وضعیت', cell: (r: any) => <StatusBadge map={dayStatus} value={r.status} /> },
          { head: 'ورود', cell: (r: any) => <span className="num" dir="ltr">{faDigits(r.first_in) || '—'}</span> }, { head: 'خروج', cell: (r: any) => <span className="num" dir="ltr">{faDigits(r.last_out) || '—'}</span> },
          { head: 'کارکرد', cell: (r: any) => hhmm(r.worked_minutes) }, { head: 'تأخیر', cell: (r: any) => (r.late_minutes ? <span className="font-medium text-warn">{fa(r.late_minutes)} دقیقه</span> : '—') }]} />
      </Card>
    </div>
  )
}

// ───────────────────────────── attendance log ─────────────────────────────

const methodLabel: Record<string, string> = { face: 'چهره', pin: 'کد پرسنلی', fingerprint: 'اثر انگشت', manual: 'دستی' }

export function AttendancePage() {
  const can = useCan()
  const inv = useInvalidate()
  const [from, setFrom] = useState(todayISO())
  const [to, setTo] = useState(todayISO())
  const [emp, setEmp] = useState(0)
  const [page, setPage] = useState(1)
  const [add, setAdd] = useState<any>(null)
  const emps = useGet<any>('/hr/employees?limit=500&active=true')
  const list = useGet<any>('/hr/attendance' + qs({ from, to, employee_id: emp || undefined, page, limit: 50 }))
  return (
    <div>
      <PageHeader title="ترددها" subtitle="همهٔ ورود و خروج‌های ثبت‌شده با چهره، اثر انگشت، کد پرسنلی یا دستی." actions={can('hr.manage') && <button className="btn-primary" onClick={() => setAdd({ employee_id: 0, kind: 'in', date: todayISO(), time: '08:00', note: '' })}><Plus className="h-4 w-4" />ثبت دستی تردد</button>} />
      <Card>
        <div className="flex flex-wrap items-end gap-3 border-b border-line p-3">
          <Field label="از"><JalaliInput value={from} onChange={(v) => { v && setFrom(v); setPage(1) }} /></Field>
          <Field label="تا"><JalaliInput value={to} onChange={(v) => { v && setTo(v); setPage(1) }} /></Field>
          <Field label="پرسنل"><select className="input" value={emp} onChange={(e) => { setEmp(+e.target.value); setPage(1) }}><option value={0}>همه</option>{emps.data?.items.map((e: any) => <option key={e.id} value={e.id}>{e.first_name} {e.last_name}</option>)}</select></Field>
        </div>
        {list.isLoading ? <Loading /> : list.error ? <ErrorBox error={list.error} /> : !list.data.items.length ? <Empty title="تردادی در این بازه نیست" /> : (
          <>
            <DataTable rows={list.data.items} rowKey={(r: any) => r.id} cols={[
              { head: 'پرسنل', cell: (r: any) => <div><b>{r.employee_name}</b><div className="text-xs text-soft num">{r.personnel_code}</div></div> },
              { head: 'نوع', cell: (r: any) => <Badge tone={r.kind === 'in' ? 'ok' : 'info'}>{r.kind === 'in' ? 'ورود' : 'خروج'}</Badge> },
              { head: 'زمان', cell: (r: any) => <span className="num">{dateTime(r.happened_at)}</span> },
              { head: 'روش', cell: (r: any) => <span>{methodLabel[r.method] ?? r.method}{r.confidence ? <span className="text-xs text-soft num"> · {fa(Math.round(r.confidence * 100))}٪</span> : null}</span> },
              { head: 'دستگاه', cell: (r: any) => <span>{r.device || '—'}{r.created_offline && <span className="ms-1 text-xs text-soft">(آفلاین)</span>}</span> },
              { head: 'یادداشت', cell: (r: any) => r.note || '—' },
              ...(can('hr.delete') ? [{ head: '', cell: (r: any) => <ActionButton className="btn-ghost btn-sm text-bad" confirm="این تردد حذف شود؟" onClick={async () => { await del(`/hr/attendance/${r.id}`); inv('/hr/attendance') }}><Trash2 className="h-4 w-4" /></ActionButton> }] : [])]} />
            <Pager page={page} total={list.data.total} limit={list.data.limit} onPage={setPage} />
          </>)}
      </Card>
      <Modal open={!!add} onClose={() => setAdd(null)} title="ثبت دستی تردد" footer={<ActionButton onClick={async () => {
        if (!add.employee_id) throw new Error('پرسنل را انتخاب کنید')
        await post('/hr/attendance', { employee_id: add.employee_id, kind: add.kind, note: add.note, happened_at: `${add.date}T${add.time}:00+03:30` }); inv('/hr/attendance'); setAdd(null)
      }}>ثبت</ActionButton>}>
        {add && <div className="grid gap-3 sm:grid-cols-2">
          <Field label="پرسنل" className="sm:col-span-2"><select className="input" value={add.employee_id} onChange={(e) => setAdd({ ...add, employee_id: +e.target.value })}><option value={0}>انتخاب…</option>{emps.data?.items.map((e: any) => <option key={e.id} value={e.id}>{e.first_name} {e.last_name}</option>)}</select></Field>
          <Field label="نوع"><select className="input" value={add.kind} onChange={(e) => setAdd({ ...add, kind: e.target.value })}><option value="in">ورود</option><option value="out">خروج</option></select></Field>
          <Field label="ساعت"><input className="input num" dir="ltr" type="time" value={add.time} onChange={(e) => setAdd({ ...add, time: e.target.value })} /></Field>
          <Field label="تاریخ"><JalaliInput value={add.date} onChange={(v) => v && setAdd({ ...add, date: v })} /></Field>
          <Field label="یادداشت (دلیل ثبت دستی)"><input className="input" value={add.note} onChange={(e) => setAdd({ ...add, note: e.target.value })} /></Field>
        </div>}
      </Modal>
    </div>
  )
}

// ───────────────────────────── reports ─────────────────────────────

const addDays = (iso: string, n: number) => { const d = new Date(iso + 'T12:00:00Z'); d.setUTCDate(d.getUTCDate() + n); return d.toISOString().slice(0, 10) }
function presets() {
  const today = todayISO()
  const { y, m } = jalaliParts(new Date())
  const monthStart = (yy: number, mm: number) => jalaliToISO(yy + Math.floor((mm - 1) / 12), ((mm - 1 + 12) % 12) + 1, 1) as string
  const thisStart = monthStart(y, m), nextStart = monthStart(y, m + 1)
  const lastStart = monthStart(y, m - 1)
  const weekOffset = (new Date().getDay() + 1) % 7 // week starts on Saturday
  return {
    today: [today, today], week: [addDays(today, -weekOffset), today], month: [thisStart, addDays(nextStart, -1) > today ? today : addDays(nextStart, -1)],
    last_month: [lastStart, addDays(thisStart, -1)], d30: [addDays(today, -29), today], d90: [addDays(today, -89), today],
  } as Record<string, [string, string]>
}
const presetLabels: [string, string][] = [['today', 'امروز'], ['week', 'این هفته'], ['month', 'این ماه'], ['last_month', 'ماه قبل'], ['d30', '۳۰ روز اخیر'], ['d90', '۹۰ روز اخیر']]

export function Report() {
  const P = useMemo(presets, [])
  const [from, setFrom] = useState(P.month[0])
  const [to, setTo] = useState(P.month[1])
  const [preset, setPreset] = useState('month')
  const [tab, setTab] = useState<'overview' | 'table' | 'rank'>('overview')
  const [emp, setEmp] = useState<any>(null)
  const d = useGet<any>('/hr/reports/range' + qs({ from, to }))
  const one = useGet<any>(emp ? '/hr/reports/range' + qs({ from, to, employee_id: emp.employee_id }) : null)
  const rows: any[] = d.data?.employees ?? []
  const tot = d.data?.totals
  const top = (key: string, n = 6) => [...rows].sort((a, b) => b[key] - a[key]).filter((r) => r[key] > 0).slice(0, n)
  return (
    <div className="space-y-4">
      <PageHeader title="گزارش حضور و غیاب" subtitle="خلاصه، نمودارها و رتبه‌بندی پرسنل در بازهٔ دلخواه"
        actions={<><button className="btn-ghost btn-sm" onClick={() => download(`/hr/reports/range.csv${qs({ from, to })}`, 'attendance-summary.csv')}><Download className="h-4 w-4" />خلاصه (اکسل)</button>
          <button className="btn-ghost btn-sm" onClick={() => download(`/hr/reports/range.csv${qs({ from, to, detail: 1 })}`, 'attendance-daily.csv')}><Download className="h-4 w-4" />ریز روزانه (اکسل)</button></>} />
      <Card className="flex flex-wrap items-end gap-3 p-3">
        <div className="flex flex-wrap gap-1.5">{presetLabels.map(([k, l]) => <button key={k} className={preset === k ? 'btn-primary btn-sm' : 'btn-ghost btn-sm'} onClick={() => { setPreset(k); setFrom(P[k][0]); setTo(P[k][1]) }}>{l}</button>)}</div>
        <Field label="از"><JalaliInput value={from} onChange={(v) => { if (v) { setFrom(v); setPreset('') } }} /></Field>
        <Field label="تا"><JalaliInput value={to} onChange={(v) => { if (v) { setTo(v); setPreset('') } }} /></Field>
      </Card>
      {d.isLoading ? <Loading /> : d.error ? <ErrorBox error={d.error} retry={() => d.refetch()} /> : !rows.length ? <Empty title="پرسنل فعالی نیست" /> : (
        <>
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
            <Stat label="نرخ حضور" value={`${fa(tot.attendance_rate)}٪`} tone={tot.attendance_rate >= 90 ? 'ok' : tot.attendance_rate >= 75 ? 'warn' : 'bad'} />
            <Stat label="روز-نفر حضور" value={fa(tot.present_days)} tone="ok" /><Stat label="روز-نفر غیبت" value={fa(tot.absent_days)} tone="bad" />
            <Stat label="مرخصی / مأموریت" value={fa(tot.leave_days)} tone="info" /><Stat label="مجموع کارکرد" value={fa(Math.round(tot.worked_minutes / 60))} hint="ساعت" />
            <Stat label="مجموع تأخیر" value={fa(Math.round(tot.late_minutes / 60))} hint={`ساعت · ${fa(tot.late_count)} بار`} tone="warn" />
          </div>
          <Tabs value={tab} onChange={setTab} items={[{ key: 'overview', label: 'نمودارها' }, { key: 'table', label: 'جدول پرسنل' }, { key: 'rank', label: 'رتبه‌بندی' }]} />
          {tab === 'overview' && (
            <div className="grid gap-5 lg:grid-cols-2">
              <Card className="p-4 lg:col-span-2"><h3 className="mb-3 font-bold">روند روزانه حضور، مرخصی و غیبت</h3><TrendChart series={d.data.series} h={300} /></Card>
              <Card className="p-4"><h3 className="mb-3 font-bold">ساعات کارکرد و اضافه‌کار روزانه</h3><HoursChart series={d.data.series} /></Card>
              <Card className="p-4"><h3 className="mb-3 font-bold">ساعت ورود پرسنل</h3><ArrivalsChart arrivals={d.data.arrivals} /></Card>
              <Card className="p-4 lg:col-span-2"><h3 className="mb-3 font-bold">نرخ حضور به تفکیک واحد</h3><DeptChart depts={d.data.departments} /></Card>
            </div>)}
          {tab === 'table' && (
            <Card><DataTable rows={rows} rowKey={(r: any) => r.employee_id} onRow={setEmp} cols={[
              { head: 'پرسنل', cell: (r: any) => <div><b>{r.name}</b><div className="text-xs text-soft">{r.department}</div></div> },
              { head: 'حضور', cell: (r: any) => fa(r.present_days) }, { head: 'غیبت', cell: (r: any) => <span className={r.absent_days ? 'num font-bold text-bad' : 'num'}>{fa(r.absent_days)}</span> },
              { head: 'مرخصی', cell: (r: any) => fa(r.leave_days) }, { head: 'ناقص', cell: (r: any) => (r.incomplete_days ? <Badge tone="warn">{fa(r.incomplete_days)}</Badge> : '—') },
              { head: 'کارکرد', cell: (r: any) => minutes(r.worked_minutes) }, { head: 'تأخیر', cell: (r: any) => (r.late_minutes ? `${minutes(r.late_minutes)} (${fa(r.late_count)}×)` : '—') },
              { head: 'اضافه‌کار', cell: (r: any) => minutes(r.overtime_minutes) },
              { head: 'نرخ حضور', cell: (r: any) => <div className="flex items-center gap-2"><div className="h-1.5 w-16 overflow-hidden rounded-full bg-muted"><div className={r.attendance_rate >= 90 ? 'h-full bg-ok' : r.attendance_rate >= 75 ? 'h-full bg-warn' : 'h-full bg-bad'} style={{ width: `${r.attendance_rate}%` }} /></div><span className="num text-xs">{fa(r.attendance_rate)}٪</span></div> }]} /></Card>)}
          {tab === 'rank' && (
            <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
              <Card className="p-4"><h3 className="mb-3 font-bold">بیشترین تأخیر</h3><RankList rows={top('late_minutes')} valueKey="late_minutes" unit="دقیقه" /></Card>
              <Card className="p-4"><h3 className="mb-3 font-bold">بیشترین غیبت</h3><RankList rows={top('absent_days')} valueKey="absent_days" unit="روز" tone="bad" /></Card>
              <Card className="p-4"><h3 className="mb-3 font-bold">بیشترین اضافه‌کار</h3><RankList rows={top('overtime_minutes')} valueKey="overtime_minutes" unit="ساعت" tone="ok" fmt={(n) => fa(Math.round(n / 6) / 10)} /></Card>
              <Card className="p-4"><h3 className="mb-3 font-bold">بیشترین کارکرد</h3><RankList rows={top('worked_minutes')} valueKey="worked_minutes" unit="ساعت" tone="ok" fmt={(n) => fa(Math.round(n / 60))} /></Card>
              <Card className="p-4"><h3 className="mb-3 font-bold">بیشترین تعجیل در خروج</h3><RankList rows={top('early_leave_minutes')} valueKey="early_leave_minutes" unit="دقیقه" /></Card>
              <Card className="p-4"><h3 className="mb-3 font-bold">تردد ناقص (بدون خروج)</h3><RankList rows={top('incomplete_days')} valueKey="incomplete_days" unit="روز" /></Card>
            </div>)}
        </>)}
      <Modal open={!!emp} onClose={() => setEmp(null)} wide title={emp?.name ?? ''}>
        {one.isLoading || !one.data ? <Loading /> : (
          <div className="space-y-4">
            <div className="grid grid-cols-3 gap-2 text-center text-sm">{[['حضور', one.data.employees[0]?.present_days], ['غیبت', one.data.employees[0]?.absent_days], ['نرخ حضور', fa(one.data.employees[0]?.attendance_rate) + '٪']].map(([k, v]) => <div key={k as string} className="rounded-xl bg-muted p-2"><div className="text-xs text-soft">{k}</div><b className="num">{typeof v === 'number' ? fa(v) : v}</b></div>)}</div>
            <table className="w-full text-sm"><thead><tr className="text-soft"><th className="th">تاریخ</th><th className="th">وضعیت</th><th className="th">ورود</th><th className="th">خروج</th><th className="th">کارکرد</th><th className="th">تأخیر</th></tr></thead><tbody>
              {one.data.employees[0]?.days?.map((x: any) => <tr key={x.date} className="border-t border-line/60"><td className="td num">{faDigits(x.jalali)}</td><td className="td"><StatusBadge map={dayStatus} value={x.status} /></td><td className="td num" dir="ltr">{faDigits(x.first_in) || '—'}</td><td className="td num" dir="ltr">{faDigits(x.last_out) || '—'}</td><td className="td">{minutes(x.worked_minutes)}</td><td className="td">{x.late_minutes ? minutes(x.late_minutes) : '—'}</td></tr>)}</tbody></table>
          </div>)}
      </Modal>
    </div>
  )
}

void [CalendarClock, ClipboardList, DoorOpen, ScanFace, Timer, UserCheck, UserX, Users, TrendChart]
