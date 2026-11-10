import { useState } from 'react'
import { Fingerprint, Loader2 } from 'lucide-react'
import { ActionButton, Badge, Empty, Field, Modal } from './ui'
import { del, post } from '../lib/api'
import { dateTime } from '../lib/format'
import { useGet, useInvalidate } from '../lib/hooks'

const jobTone: Record<string, [string, 'ok' | 'warn' | 'bad' | 'mute']> = { pending: ['در انتظار انگشت‌گذاری…', 'warn'], done: ['ثبت شد', 'ok'], failed: ['ناموفق', 'bad'], cancelled: ['لغو شد', 'mute'] }

/**
 * Fingerprint registration happens on the reader itself: this dialog sends a job to the chosen reader
 * ("enrol this person") and shows its progress while the employee places a finger on the sensor.
 */
export function FingerprintEnroll({ employee, onClose }: { employee: { id: number; name: string } | null; onClose: () => void }) {
  const inv = useInvalidate()
  const devices = useGet<any[]>(employee ? '/hr/devices' : null)
  const jobs = useGet<any[]>(employee ? `/hr/employees/${employee.id}/fingerprint-jobs` : null, { refetchInterval: 3000 })
  const [dev, setDev] = useState(0)
  if (!employee) return null
  const readers = (devices.data ?? []).filter((d) => d.kind === 'fingerprint' && d.is_active)
  const active = jobs.data?.find((j) => j.status === 'pending')
  return (
    <Modal open onClose={onClose} title={`ثبت اثر انگشت — ${employee.name}`}>
      <div className="space-y-4">
        <p className="rounded-xl bg-brand-tint p-3 text-xs leading-6 text-brand">ثبت اثر انگشت روی خود دستگاه انجام می‌شود. دستگاه را انتخاب کنید و «شروع» را بزنید؛ سپس پرسنل انگشت خود را روی حسگر دستگاه بگذارد (معمولاً دو یا سه بار).</p>
        {!readers.length ? <Empty title="دستگاه اثر انگشتی ثبت نشده" hint="از بخش «دستگاه‌های حضور» یک دستگاه از نوع اثر انگشت اضافه کنید." /> : (
          <Field label="دستگاه"><select className="input" value={dev} onChange={(e) => setDev(+e.target.value)}><option value={0}>انتخاب دستگاه…</option>{readers.map((d) => <option key={d.id} value={d.id}>{d.name}{d.online ? ' (آنلاین)' : ' (آفلاین)'}</option>)}</select></Field>
        )}
        {active ? (
          <div className="flex items-center justify-between rounded-xl border border-warn/40 bg-warn/10 p-3 text-sm"><span className="flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />منتظر انگشت‌گذاری روی «{active.device}»…</span>
            <ActionButton className="btn-ghost btn-sm" onClick={async () => { await del(`/hr/fingerprint-jobs/${active.id}`); inv('/hr/employees') }}>لغو</ActionButton></div>
        ) : <ActionButton disabled={!dev} onClick={async () => { await post(`/hr/employees/${employee.id}/fingerprint-jobs`, { device_id: dev }); inv('/hr/employees') }}><Fingerprint className="h-4 w-4" />شروع ثبت اثر انگشت</ActionButton>}
        {!!jobs.data?.length && <div><h4 className="mb-2 text-sm font-bold">آخرین درخواست‌ها</h4><ul className="divide-y divide-line/60 rounded-xl border border-line text-sm">{jobs.data.map((j) => (
          <li key={j.id} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"><span>{j.device} <span className="text-xs text-soft">· {dateTime(j.created_at)}</span></span><span className="flex items-center gap-2">{j.error && <span className="text-xs text-bad">{j.error}</span>}<Badge tone={jobTone[j.status]?.[1]}>{jobTone[j.status]?.[0] ?? j.status}</Badge></span></li>))}</ul></div>}
      </div>
    </Modal>
  )
}
