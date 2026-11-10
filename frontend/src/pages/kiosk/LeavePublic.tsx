import { useState } from 'react'
import { CalendarClock, CheckCircle2 } from 'lucide-react'
import { ActionButton, Card, Field, JalaliInput, NumInput } from '../../components/ui'
import { post } from '../../lib/api'
import { todayISO } from '../../lib/format'
import { leaveType } from '../../lib/labels'

/** Employees ask for leave with their personnel code + PIN - no account needed. */
export default function LeavePublic() {
  const [f, setF] = useState({ personnel_code: '', pin: '', leave_type: 'daily', from: todayISO(), to: todayISO(), from_time: '08:00', to_time: '16:30', reason: '' })
  const [done, setDone] = useState(false)
  const hourly = f.leave_type === 'hourly'
  const set = (p: Partial<typeof f>) => setF({ ...f, ...p })
  return (
    <div className="grid min-h-screen place-items-center bg-bg p-4">
      <div className="w-full max-w-md">
        <div className="mb-5 text-center"><img src="/logo-evinki.png" alt="اوینکی" className="mx-auto mb-3 h-10" /><h1 className="flex items-center justify-center gap-2 text-xl font-extrabold"><CalendarClock className="h-6 w-6 text-brand" />درخواست مرخصی</h1><p className="mt-1 text-sm text-soft">با کد پرسنلی و رمز خود درخواست ثبت کنید؛ نتیجه را مدیر اعلام می‌کند.</p></div>
        {done ? <Card className="p-8 text-center"><CheckCircle2 className="mx-auto mb-3 h-12 w-12 text-ok" /><h2 className="font-bold">درخواست شما ثبت شد</h2><button className="btn-ghost mt-4" onClick={() => { setDone(false); set({ reason: '', pin: '' }) }}>ثبت درخواست دیگر</button></Card> : (
          <Card className="space-y-4 p-5">
            <div className="grid gap-3 sm:grid-cols-2"><Field label="کد پرسنلی"><NumInput value={f.personnel_code} onChange={(v) => set({ personnel_code: v })} /></Field><Field label="رمز"><NumInput type="password" maxLength={8} value={f.pin} onChange={(v) => set({ pin: v })} /></Field></div>
            <Field label="نوع"><select className="input" value={f.leave_type} onChange={(e) => set({ leave_type: e.target.value })}>{Object.entries(leaveType).map(([k, v]) => <option key={k} value={k}>{v}</option>)}</select></Field>
            <div className="grid gap-3 sm:grid-cols-2"><Field label={hourly ? 'تاریخ' : 'از تاریخ'}><JalaliInput value={f.from} onChange={(v) => v && set({ from: v, to: hourly ? v : f.to })} /></Field>
              {!hourly && <Field label="تا تاریخ"><JalaliInput value={f.to} onChange={(v) => v && set({ to: v })} /></Field>}
              {hourly && <><Field label="از ساعت"><input className="input num" dir="ltr" type="time" value={f.from_time} onChange={(e) => set({ from_time: e.target.value })} /></Field><Field label="تا ساعت"><input className="input num" dir="ltr" type="time" value={f.to_time} onChange={(e) => set({ to_time: e.target.value })} /></Field></>}</div>
            <Field label="دلیل"><textarea className="input" rows={2} value={f.reason} onChange={(e) => set({ reason: e.target.value })} /></Field>
            <ActionButton className="btn-primary w-full" onClick={async () => {
              const s = hourly ? `${f.from}T${f.from_time}:00+03:30` : `${f.from}T00:00:00+03:30`
              const e = hourly ? `${f.from}T${f.to_time}:00+03:30` : `${f.to}T23:59:00+03:30`
              await post('/public/leaves', { personnel_code: f.personnel_code, pin: f.pin, leave_type: f.leave_type, start_at: s, end_at: e, reason: f.reason }); setDone(true)
            }}>ثبت درخواست</ActionButton>
          </Card>)}
      </div>
    </div>
  )
}
