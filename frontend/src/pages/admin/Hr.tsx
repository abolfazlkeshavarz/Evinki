import { useState } from 'react'
import { Download, Plus } from 'lucide-react'
import { ActionButton, Badge, Card, DataTable, Empty, ErrorBox, Field, JalaliInput, KV, Loading, Modal, NumInput, PageHeader, StatusBadge, Tabs } from '../../components/ui'
import { FieldDef, ResourcePage } from '../../components/Resource'
import { del, download, patch, post } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { addMonthsISO, date, dateTime, fa, minutes, todayISO } from '../../lib/format'
import { qs, useGet, useInvalidate } from '../../lib/hooks'
import { dayStatus, leaveStatus, leaveType, priority, taskStatus } from '../../lib/labels'

export function Shifts() {
  const days = ['شنبه', 'یکشنبه', 'دوشنبه', 'سه‌شنبه', 'چهارشنبه', 'پنجشنبه', 'جمعه']
  return (
    <div className="space-y-10">
      <ResourcePage title="شیفت‌های کاری" path="/hr/shifts" canWrite={['hr.manage']} canDelete={['hr.delete']} searchable={false} newRow={{ is_active: true, grace_in_minutes: 10, grace_out_minutes: 10, work_days: '0,1,2,3,4,5', break_minutes: 0, start_time: '08:00', end_time: '16:30' }}
        fields={[{ key: 'name', label: 'نام شیفت', required: true }, { key: 'start_time', label: 'شروع', type: 'time', half: true }, { key: 'end_time', label: 'پایان', type: 'time', half: true }, { key: 'crosses_midnight', label: 'از نیمه‌شب می‌گذرد', type: 'bool', half: true }, { key: 'break_minutes', label: 'استراحت (دقیقه)', type: 'int', half: true },
          { key: 'grace_in_minutes', label: 'مهلت تأخیر ورود (دقیقه)', type: 'int', half: true }, { key: 'grace_out_minutes', label: 'مهلت تعجیل خروج (دقیقه)', type: 'int', half: true }, { key: 'work_days', label: 'روزهای کاری', hint: 'شماره روزها با کاما؛ شنبه=۰ … جمعه=۶ (مثال: 0,1,2,3,4,5)' }]}
        columns={[{ head: 'شیفت', cell: (r) => <b>{r.name}</b> }, { head: 'ساعت', cell: (r) => <span className="num" dir="ltr">{r.start_time} – {r.end_time}</span> }, { head: 'روزهای کاری', cell: (r) => r.work_days.split(',').map((d: string) => days[+d]).join('، ') }, { head: 'مهلت', cell: (r) => `${fa(r.grace_in_minutes)} / ${fa(r.grace_out_minutes)} دقیقه` }]} />
      <ResourcePage title="واحدها" path="/hr/departments" canWrite={['hr.manage']} canDelete={['hr.delete']} searchable={false} newRow={{ is_active: true }}
        fields={[{ key: 'name', label: 'نام واحد', required: true }, { key: 'description', label: 'توضیح' }, { key: 'is_active', label: 'فعال', type: 'bool' }]} columns={[{ head: 'واحد', cell: (r) => <b>{r.name}</b> }, { head: 'توضیح', cell: (r) => r.description || '—' }]} />
      <ResourcePage title="تعطیلات رسمی" path="/hr/holidays" canWrite={['hr.manage']} canDelete={['hr.delete']} searchable={false} pageSize={60}
        fields={[{ key: 'day', label: 'تاریخ', type: 'jdate', required: true, half: true }, { key: 'title', label: 'مناسبت', required: true, half: true }]} columns={[{ head: 'تاریخ', cell: (r) => date(r.day) }, { head: 'مناسبت', cell: (r) => r.title }]} />
    </div>
  )
}

export function Leaves() {
  const [status, setStatus] = useState('pending')
  const d = useGet<any[]>('/hr/leaves' + qs({ status }))
  const emps = useGet<any>('/hr/employees?limit=500')
  const can = useCan()
  const inv = useInvalidate()
  const [add, setAdd] = useState<any>(null)
  return (
    <div>
      <PageHeader title="مرخصی و مأموریت" actions={can('hr.manage') && <button className="btn-primary" onClick={() => setAdd({ employee_id: 0, leave_type: 'daily', start_at: '', end_at: '', reason: '' })}><Plus className="h-4 w-4" />ثبت مرخصی</button>} />
      <Tabs value={status} onChange={setStatus} items={[{ key: 'pending', label: 'در انتظار' }, { key: 'approved', label: 'تایید شده' }, { key: 'rejected', label: 'رد شده' }, { key: '', label: 'همه' }]} />
      <Card>{d.isLoading ? <Loading /> : !d.data?.length ? <Empty title="درخواستی نیست" /> : d.data.map((l) => (
        <div key={l.id} className="flex flex-wrap items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0">
          <div><b>{l.employee_name}</b> <Badge>{leaveType[l.leave_type]}</Badge><div className="text-xs text-soft">{dateTime(l.start_at)} تا {dateTime(l.end_at)}{l.reason && ` · ${l.reason}`}</div></div>
          <div className="flex items-center gap-2"><StatusBadge map={leaveStatus} value={l.status} />{l.status === 'pending' && can('hr.manage') && <>
            <ActionButton className="btn-soft btn-sm" onClick={async () => { await post(`/hr/leaves/${l.id}/review`, { decision: 'approve' }); inv('/hr') }}>تایید</ActionButton>
            <ActionButton className="btn-ghost btn-sm text-bad" onClick={async () => { await post(`/hr/leaves/${l.id}/review`, { decision: 'reject' }); inv('/hr') }}>رد</ActionButton></>}</div>
        </div>))}</Card>
      <Modal open={!!add} onClose={() => setAdd(null)} title="ثبت مرخصی" footer={<ActionButton onClick={async () => { await post('/hr/leaves', { ...add, start_at: add.start_at + 'T00:00:00+03:30', end_at: add.end_at + 'T23:59:00+03:30' }); inv('/hr/leaves'); setAdd(null) }}>ثبت (تایید شده)</ActionButton>}>
        {add && <div className="grid gap-3 sm:grid-cols-2"><Field label="پرسنل" className="sm:col-span-2"><select className="input" value={add.employee_id} onChange={(e) => setAdd({ ...add, employee_id: +e.target.value })}><option value={0}>انتخاب…</option>{emps.data?.items.map((e: any) => <option key={e.id} value={e.id}>{e.first_name} {e.last_name}</option>)}</select></Field>
          <Field label="نوع"><select className="input" value={add.leave_type} onChange={(e) => setAdd({ ...add, leave_type: e.target.value })}>{Object.entries(leaveType).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</select></Field><span />
          <Field label="از تاریخ"><JalaliInput value={add.start_at} onChange={(v) => setAdd({ ...add, start_at: v })} /></Field><Field label="تا تاریخ"><JalaliInput value={add.end_at} onChange={(v) => setAdd({ ...add, end_at: v })} /></Field>
          <Field label="دلیل" className="sm:col-span-2"><input className="input" value={add.reason} onChange={(e) => setAdd({ ...add, reason: e.target.value })} /></Field></div>}
      </Modal>
    </div>
  )
}

export function Tasks() {
  const emps = useGet<any>('/hr/employees?limit=500')
  const [logTask, setLogTask] = useState<any>(null)
  return (
    <>
    <ResourcePage rowActions={(r) => <button className="btn-soft btn-sm" onClick={() => setLogTask(r)}>گزارش روزانه</button>} title="وظایف و شرح وظایف" path="/hr/tasks" canWrite={['hr.manage']} canDelete={['hr.delete']} newRow={{ status: 'todo', priority: 'normal', recurrence: 'none', progress: 0, is_active: true }}
      fields={[{ key: 'title', label: 'عنوان', required: true }, { key: 'description', label: 'شرح', type: 'textarea' },
        { key: 'employee_id', label: 'مسئول', type: 'select', half: true, options: emps.data?.items.map((e: any) => ({ value: e.id, label: `${e.first_name} ${e.last_name}` })) },
        { key: 'status', label: 'وضعیت', type: 'select', half: true, options: Object.entries(taskStatus).map(([k, v]) => ({ value: k, label: v[0] })) },
        { key: 'priority', label: 'اولویت', type: 'select', half: true, options: Object.entries(priority).map(([k, v]) => ({ value: k, label: v[0] })) },
        { key: 'recurrence', label: 'تکرار', type: 'select', half: true, options: [{ value: 'none', label: 'یک‌بار' }, { value: 'daily', label: 'روزانه' }, { value: 'weekly', label: 'هفتگی' }, { value: 'monthly', label: 'ماهانه' }] },
        { key: 'due_date', label: 'سررسید', type: 'jdate', half: true }, { key: 'progress', label: 'پیشرفت (٪)', type: 'int', half: true }]}
      columns={[{ head: 'وظیفه', cell: (r) => <b>{r.title}</b> }, { head: 'مسئول', cell: (r) => r.employee_name || '—' }, { head: 'سررسید', cell: (r) => date(r.due_date) }, { head: 'اولویت', cell: (r) => <StatusBadge map={priority} value={r.priority} /> }, { head: 'پیشرفت', cell: (r) => `${fa(r.progress)}٪` }, { head: 'وضعیت', cell: (r) => <StatusBadge map={taskStatus} value={r.status} /> }]} />
    <TaskLogs task={logTask} onClose={() => setLogTask(null)} />
    </>
  )
}

/** Day-by-day log of a task: what was done, how long it took. */
function TaskLogs({ task, onClose }: { task: any; onClose: () => void }) {
  const logs = useGet<any[]>(task ? `/hr/tasks/${task.id}/logs` : null)
  const inv = useInvalidate()
  const [f, setF] = useState({ log_date: todayISO(), status: 'done', spent_minutes: '', note: '' })
  return (
    <Modal open={!!task} onClose={onClose} wide title={`گزارش روزانه — ${task?.title ?? ''}`}
      footer={<ActionButton onClick={async () => { await post(`/hr/tasks/${task.id}/logs`, { log_date: f.log_date, status: f.status, spent_minutes: f.spent_minutes ? +f.spent_minutes : null, note: f.note }); inv(`/hr/tasks/${task.id}`); setF({ ...f, note: '', spent_minutes: '' }) }}>ثبت گزارش</ActionButton>}>
      <div className="mb-4 grid gap-3 sm:grid-cols-4">
        <Field label="تاریخ"><JalaliInput value={f.log_date} onChange={(v) => v && setF({ ...f, log_date: v })} /></Field>
        <Field label="وضعیت"><select className="input" value={f.status} onChange={(e) => setF({ ...f, status: e.target.value })}><option value="done">انجام شد</option><option value="partial">نیمه‌کاره</option><option value="skipped">انجام نشد</option></select></Field>
        <Field label="مدت (دقیقه)"><NumInput value={f.spent_minutes} onChange={(v) => setF({ ...f, spent_minutes: v })} /></Field>
        <Field label="توضیح"><input className="input" value={f.note} onChange={(e) => setF({ ...f, note: e.target.value })} /></Field>
      </div>
      {logs.isLoading ? <Loading /> : !logs.data?.length ? <Empty title="هنوز گزارشی ثبت نشده" /> : <ul className="divide-y divide-line/60 text-sm">{logs.data.map((l) => <li key={l.id} className="flex flex-wrap items-center justify-between gap-2 py-2"><span className="num">{date(l.log_date)}</span><span>{l.note || '—'}</span><span className="text-soft">{l.spent_minutes ? minutes(l.spent_minutes) : ''}</span><Badge tone={l.status === 'done' ? 'ok' : l.status === 'partial' ? 'warn' : 'bad'}>{{ done: 'انجام شد', partial: 'نیمه‌کاره', skipped: 'انجام نشد' }[l.status as string] ?? l.status}</Badge></li>)}</ul>}
    </Modal>
  )
}

export function Devices() {
  const d = useGet<any[]>('/hr/devices', { refetchInterval: 30_000 })
  const can = useCan()
  const inv = useInvalidate()
  const [add, setAdd] = useState<any>(null)
  const [key, setKey] = useState('')
  return (
    <div>
      <PageHeader title="دستگاه‌های حضور و غیاب" subtitle="تبلت‌ها و ماژول‌های اثر انگشت با کلید دستگاه به سرور وصل می‌شوند." actions={can('hr.devices') && <button className="btn-primary" onClick={() => setAdd({ name: '', kind: 'tablet', location: '' })}><Plus className="h-4 w-4" />دستگاه جدید</button>} />
      <Card>{d.isLoading ? <Loading /> : !d.data?.length ? <Empty title="دستگاهی ثبت نشده" /> : d.data.map((x) => (
        <div key={x.id} className="flex flex-wrap items-center justify-between gap-3 border-b border-line/60 p-4 text-sm last:border-0">
          <div><b>{x.name}</b> <Badge>{x.kind === 'tablet' ? 'تبلت' : 'اثر انگشت'}</Badge> {x.online ? <Badge tone="ok">آنلاین</Badge> : <Badge tone="bad">آفلاین</Badge>}
            <div className="text-xs text-soft">{x.location} · آخرین ارتباط: {x.last_seen_at ? dateTime(x.last_seen_at) : 'هرگز'} · ترددهای ارسال‌نشده: {fa(x.pending_count)}</div></div>
          {can('hr.devices') && <div className="flex gap-2"><ActionButton className="btn-ghost btn-sm" confirm="کلید قبلی از کار می‌افتد. ادامه می‌دهید؟" onClick={async () => { setKey((await post(`/hr/devices/${x.id}/rotate-key`)).api_key) }}>کلید جدید</ActionButton>
            <ActionButton className="btn-ghost btn-sm" onClick={async () => { await patch(`/hr/devices/${x.id}`, { is_active: !x.is_active }); inv('/hr/devices') }}>{x.is_active ? 'غیرفعال' : 'فعال'}</ActionButton></div>}
        </div>))}</Card>
      <Modal open={!!add} onClose={() => setAdd(null)} title="دستگاه جدید" footer={<ActionButton onClick={async () => { const r = await post('/hr/devices', add); setKey(r.api_key); inv('/hr/devices'); setAdd(null) }}>ثبت</ActionButton>}>
        {add && <div className="space-y-3"><Field label="نام"><input className="input" placeholder="درب اصلی" value={add.name} onChange={(e) => setAdd({ ...add, name: e.target.value })} /></Field><Field label="نوع"><select className="input" value={add.kind} onChange={(e) => setAdd({ ...add, kind: e.target.value })}><option value="tablet">تبلت ورودی (چهره/کد)</option><option value="fingerprint">ماژول اثر انگشت</option></select></Field><Field label="محل"><input className="input" value={add.location} onChange={(e) => setAdd({ ...add, location: e.target.value })} /></Field></div>}
      </Modal>
      <Modal open={!!key} onClose={() => setKey('')} title="کلید دستگاه">
        <p className="mb-3 text-sm text-bad">این کلید فقط همین یک بار نمایش داده می‌شود. آن را روی دستگاه وارد کنید.</p>
        <div className="select-all break-all rounded-xl bg-muted p-3 font-mono text-sm" dir="ltr">{key}</div>
        <button className="btn-soft mt-3 w-full" onClick={() => navigator.clipboard?.writeText(key)}>کپی</button>
      </Modal>
    </div>
  )
}

void del; void KV
