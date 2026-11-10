import { Suspense, lazy, useState } from 'react'
import { Fingerprint, KeyRound, ScanFace } from 'lucide-react'
import { ActionButton, Badge, Modal, NumInput } from '../../components/ui'
import { FieldDef, ResourcePage } from '../../components/Resource'
import { FingerprintEnroll } from '../../components/FingerprintEnroll'
import { post } from '../../lib/api'
import { useCan } from '../../lib/auth'
import { useGet, useInvalidate } from '../../lib/hooks'

// the face engine (TensorFlow + ONNX, ~1.4 MB) is only downloaded when someone opens the camera dialog
const FaceEnroll = lazy(() => import('../../components/FaceEnroll').then((m) => ({ default: m.FaceEnroll })))

export function Avatar({ name, src, size = 36 }: { name: string; src?: string | null; size?: number }) {
  const initials = (name || '?').trim().split(/\s+/).map((p) => p[0]).slice(0, 2).join('')
  return src
    ? <img src={src} alt="" className="shrink-0 rounded-full object-cover" style={{ width: size, height: size }} />
    : <span className="grid shrink-0 place-items-center rounded-full bg-brand-tint font-bold text-brand" style={{ width: size, height: size, fontSize: size / 2.6 }}>{initials}</span>
}

export function Employees() {
  const deps = useGet<any>('/hr/departments?limit=200')
  const shifts = useGet<any>('/hr/shifts?limit=200')
  const [pin, setPin] = useState<any>(null)
  const [face, setFace] = useState<any>(null)
  const [fp, setFp] = useState<any>(null)
  const [dep, setDep] = useState('')
  const inv = useInvalidate()
  const can = useCan()
  const fields: FieldDef[] = [
    { key: 'photo', label: 'عکس پرسنلی', type: 'image', hint: 'برای نمایش در فهرست‌ها (برای تشخیص چهره از دکمهٔ «چهره» استفاده کنید)' },
    { key: 'personnel_code', label: 'کد پرسنلی', required: true, half: true }, { key: 'national_code', label: 'کد ملی', half: true },
    { key: 'first_name', label: 'نام', required: true, half: true }, { key: 'last_name', label: 'نام خانوادگی', required: true, half: true },
    { key: 'mobile', label: 'موبایل', half: true }, { key: 'position', label: 'سمت', half: true },
    { key: 'department_id', label: 'واحد', type: 'select', half: true, options: deps.data?.items.map((d: any) => ({ value: d.id, label: d.name })) },
    { key: 'shift_id', label: 'شیفت', type: 'select', half: true, options: shifts.data?.items.map((s: any) => ({ value: s.id, label: `${s.name} (${s.start_time}–${s.end_time})` })) },
    { key: 'hire_date', label: 'تاریخ استخدام', type: 'jdate', half: true }, { key: 'is_active', label: 'فعال', type: 'bool', half: true }, { key: 'notes', label: 'یادداشت', type: 'textarea' },
  ]
  const nm = (r: any) => `${r.first_name} ${r.last_name}`
  return (
    <>
      <ResourcePage title="پرسنل" subtitle="ثبت چهره، اثر انگشت و رمز ورود هر نفر از همین‌جا انجام می‌شود." path="/hr/employees" query={{ department_id: dep }} fields={fields} canWrite={['hr.manage']} canDelete={['hr.delete']} newRow={{ is_active: true }} pageSize={25}
        filters={<select className="input w-auto" value={dep} onChange={(e) => setDep(e.target.value)}><option value="">همهٔ واحدها</option>{deps.data?.items.map((d: any) => <option key={d.id} value={d.id}>{d.name}</option>)}</select>}
        rowActions={(r) => can('hr.manage') && (
          <>
            <button className="btn-soft btn-sm" onClick={() => setFace({ id: r.id, name: nm(r) })}><ScanFace className="h-4 w-4" />چهره</button>
            {can('hr.devices') && <button className="btn-ghost btn-sm" onClick={() => setFp({ id: r.id, name: nm(r) })}><Fingerprint className="h-4 w-4" />انگشت</button>}
            <button className="btn-ghost btn-sm" onClick={() => setPin({ id: r.id, name: nm(r), pin: '' })}><KeyRound className="h-4 w-4" />رمز</button>
          </>)}
        columns={[
          { head: 'پرسنل', cell: (r) => <div className="flex items-center gap-2.5"><Avatar name={nm(r)} src={r.photo} /><div><b>{nm(r)}</b><div className="text-xs text-soft num">{r.personnel_code}{r.position ? ` · ${r.position}` : ''}</div></div></div> },
          { head: 'واحد', cell: (r) => r.department || '—' }, { head: 'شیفت', cell: (r) => r.shift || '—' },
          { head: 'چهره', cell: (r) => (r.face_enrolled ? <Badge tone="ok">ثبت شده</Badge> : <Badge tone="warn">ثبت نشده</Badge>) },
          { head: 'اثر انگشت', cell: (r) => (r.fingerprint_enrolled ? <Badge tone="ok">ثبت شده</Badge> : <Badge>ندارد</Badge>) },
          { head: 'رمز', cell: (r) => (r.has_pin ? <Badge tone="ok">دارد</Badge> : <Badge tone="warn">ندارد</Badge>) },
          { head: 'وضعیت', cell: (r) => <Badge tone={r.is_active ? 'ok' : 'mute'}>{r.is_active ? 'فعال' : 'غیرفعال'}</Badge> }]} />
      <Suspense fallback={null}>{face && <FaceEnroll employee={face} onClose={() => { setFace(null); inv('/hr/employees') }} />}</Suspense>
      <FingerprintEnroll employee={fp} onClose={() => { setFp(null); inv('/hr/employees') }} />
      <Modal open={!!pin} onClose={() => setPin(null)} title={`رمز ورود — ${pin?.name ?? ''}`} footer={<ActionButton onClick={async () => { await post(`/hr/employees/${pin.id}/pin`, { pin: pin.pin }); inv('/hr/employees'); setPin(null) }}>ذخیره</ActionButton>}>
        <p className="mb-3 text-sm text-soft">رمز ۴ تا ۸ رقمی برای ورود/خروج با کد پرسنلی (وقتی تشخیص چهره ممکن نیست) و برای ثبت درخواست مرخصی.</p>
        {pin && <NumInput maxLength={8} value={pin.pin} onChange={(v) => setPin({ ...pin, pin: v })} placeholder="مثلاً ۴۳۲۱" />}
      </Modal>
    </>
  )
}
