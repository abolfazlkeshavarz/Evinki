import { useEffect, useRef, useState } from 'react'
import { Camera, CheckCircle2, Loader2, ScanFace, Trash2, TriangleAlert } from 'lucide-react'
import clsx from 'clsx'
import { Badge, Modal, useToast } from './ui'
import { del, post } from '../lib/api'
import { fa } from '../lib/format'
import { useGet, useInvalidate } from '../lib/hooks'
import { cropFace, FACE_MODEL_NAME, faceEngine } from '../lib/face/faceEngine'

// More samples, slightly different frame to frame, make matching more reliable in both directions
// (fewer rejections of the right person, fewer wrong accepts).
const TARGET_SAMPLES = 5
const MIN_CAPTURE_SCORE = 0.75      // a sample the detector is unsure about (poor light/angle) is never saved
const STABLE_TICKS_BEFORE_CAPTURE = 2
const CHECK_INTERVAL_MS = 350
const CAPTURE_COOLDOWN_MS = 900

interface Sample { id: number; model_name: string; quality: number | null; image: string }

/**
 * Face registration: the employee just looks at the camera; 5 samples are taken automatically.
 * The recognition vector is computed in the browser (the photo is only kept as a small thumbnail).
 */
export function FaceEnroll({ employee, onClose }: { employee: { id: number; name: string } | null; onClose: () => void }) {
  const toast = useToast()
  const inv = useInvalidate()
  const open = !!employee
  const videoRef = useRef<HTMLVideoElement | null>(null)
  const [ready, setReady] = useState(faceEngine.ready)
  const [cameraError, setCameraError] = useState('')
  const [live, setLive] = useState<'searching' | 'ok' | 'far'>('searching')
  const [busy, setBusy] = useState(false)
  const [flash, setFlash] = useState(false)
  const samples = useGet<Sample[]>(employee ? `/hr/employees/${employee.id}/faces` : null)

  const current = (samples.data ?? []).filter((s) => s.model_name === FACE_MODEL_NAME)
  const stale = (samples.data ?? []).filter((s) => s.model_name !== FACE_MODEL_NAME)
  const count = current.length
  const enough = count >= TARGET_SAMPLES
  const countRef = useRef(count)
  countRef.current = count

  const [loadError, setLoadError] = useState('')
  useEffect(() => {
    if (!open || ready) return
    setLoadError('')
    faceEngine.load().then(() => setReady(true)).catch((e) => setLoadError('بارگذاری مدل تشخیص چهره ناموفق بود: ' + (e?.message ?? '')))
  }, [open, ready])

  useEffect(() => {
    if (!open) return
    setCameraError('')
    let stream: MediaStream | null = null
    let cancelled = false
    if (!navigator.mediaDevices?.getUserMedia) { setCameraError('دوربین فقط روی آدرس امن (https) در دسترس است.'); return }
    navigator.mediaDevices.getUserMedia({ video: { facingMode: 'user', width: { ideal: 1280 } }, audio: false })
      .then(async (s) => {
        stream = s
        if (cancelled) { s.getTracks().forEach((t) => t.stop()); return }
        if (videoRef.current) { videoRef.current.srcObject = s; await videoRef.current.play() }
      })
      .catch(() => setCameraError('دسترسی به دوربین ممکن نشد. مجوز دوربین را در مرورگر فعال کنید.'))
    return () => { cancelled = true; stream?.getTracks().forEach((t) => t.stop()) }
  }, [open])

  const announced = useRef(false)
  useEffect(() => {
    if (enough && !announced.current) { announced.current = true; toast.ok(`چهرهٔ ${employee?.name ?? ''} با موفقیت ثبت شد`) }
    if (!enough) announced.current = false
  }, [enough, employee, toast])

  // automatic capture loop
  const capturing = useRef(false)
  const cooldown = useRef(false)
  const stable = useRef(0)
  useEffect(() => {
    if (!open || !ready || !employee) return
    let cancelled = false
    const id = employee.id
    const timer = setInterval(async () => {
      if (cancelled || capturing.current || cooldown.current || countRef.current >= TARGET_SAMPLES) return
      const video = videoRef.current
      if (!video || video.readyState < 2) return
      const face = await faceEngine.detect(video)
      if (!face) { setLive('searching'); stable.current = 0; return }
      if (face.box.width < video.videoWidth * 0.16 || face.score < MIN_CAPTURE_SCORE) { setLive('far'); stable.current = 0; return }
      setLive('ok')
      if (++stable.current < STABLE_TICKS_BEFORE_CAPTURE) return
      stable.current = 0
      capturing.current = true
      setBusy(true)
      try {
        const emb = await faceEngine.getEmbedding(video, face)
        await post(`/hr/employees/${id}/faces`, { vector: Array.from(emb), model_name: FACE_MODEL_NAME, quality: Math.round(face.score * 100) / 100, image: cropFace(video, face.box) ?? '' })
        inv('/hr/employees')
        setFlash(true); setTimeout(() => setFlash(false), 700)
      } catch (e) { toast.error(e) } finally {
        capturing.current = false; setBusy(false); cooldown.current = true
        setTimeout(() => { cooldown.current = false }, CAPTURE_COOLDOWN_MS)
      }
    }, CHECK_INTERVAL_MS)
    return () => { cancelled = true; clearInterval(timer) }
  }, [open, ready, employee, inv, toast])

  if (!employee) return null
  return (
    <Modal open wide onClose={onClose} title={`ثبت چهره — ${employee.name}`}
      footer={<><button className="btn-ghost" onClick={onClose}>بستن</button>
        {enough ? <span className="flex items-center gap-2 rounded-xl bg-ok/12 px-4 py-2 text-sm font-medium text-ok"><CheckCircle2 className="h-4 w-4" />ثبت کامل شد</span>
          : <span className="flex items-center gap-2 rounded-xl bg-brand-tint px-4 py-2 text-sm font-medium text-brand">{busy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Camera className="h-4 w-4" />}{busy ? 'در حال ثبت نمونه…' : 'به‌محض دیدن چهره در کادر، خودکار ثبت می‌شود'}</span>}</>}>
      <div className="grid gap-5 md:grid-cols-[1.3fr_1fr]">
        <div>
          <div className="relative aspect-[4/3] overflow-hidden rounded-2xl bg-black">
            <video ref={videoRef} muted playsInline className="h-full w-full -scale-x-100 object-cover" />
            <div className={clsx('pointer-events-none absolute left-1/2 top-1/2 h-52 w-52 -translate-x-1/2 -translate-y-1/2 rounded-full border-4 transition', live === 'ok' ? 'border-emerald-400' : live === 'far' ? 'border-amber-400' : 'border-white/40')} />
            {(cameraError || loadError) && <div className="absolute inset-0 grid place-items-center bg-black/85 px-6 text-center text-sm leading-7 text-red-200">{cameraError || loadError}</div>}
            {!ready && !cameraError && !loadError && <div className="absolute inset-0 grid place-items-center bg-black/70 text-sm text-white"><span className="flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" />آماده‌سازی موتور تشخیص چهره…</span></div>}
            <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black to-transparent px-4 py-3 text-center text-sm text-white">
              {live === 'ok' ? (flash ? '✓ ثبت شد' : busy ? 'در حال ثبت…' : 'مستقیم به دوربین نگاه کنید') : live === 'far' ? 'کمی نزدیک‌تر شوید' : 'چهره‌ای در کادر دیده نمی‌شود'}
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 rounded-xl bg-brand-tint px-3.5 py-2.5 text-xs leading-6 text-brand"><ScanFace className="h-4 w-4 shrink-0" />تصویر کامل ذخیره نمی‌شود؛ فقط بردار عددی چهره و یک تصویر بندانگشتی کوچک.</div>
        </div>
        <div>
          <div className="mb-3 flex items-center justify-between gap-2">
            <h4 className="font-bold">نمونه‌های ثبت‌شده</h4>
            <div className="flex items-center gap-2">
              <Badge tone={enough ? 'ok' : 'warn'}>{enough ? <CheckCircle2 className="h-3.5 w-3.5" /> : <TriangleAlert className="h-3.5 w-3.5" />}{fa(count)} از {fa(TARGET_SAMPLES)}</Badge>
              {(samples.data?.length ?? 0) > 0 && <button className="rounded-lg p-1.5 text-soft hover:bg-bad/10 hover:text-bad" title="حذف همه و شروع دوباره"
                onClick={async () => { if (!window.confirm('همهٔ نمونه‌های چهره حذف شود؟ بعد می‌توانید از نو ثبت کنید.')) return; await del(`/hr/employees/${employee.id}/faces`); inv('/hr/employees') }}><Trash2 className="h-4 w-4" /></button>}
            </div>
          </div>
          {stale.length > 0 && <p className="mb-3 rounded-xl bg-bad/10 px-3.5 py-2.5 text-xs leading-6 text-bad">{fa(stale.length)} نمونه با مدل قدیمی ثبت شده و برای تشخیص استفاده نمی‌شود. «حذف همه» را بزنید و دوباره جلوی دوربین بایستید.</p>}
          {!enough && <p className="mb-3 rounded-xl bg-warn/15 px-3.5 py-2.5 text-xs leading-6 text-warn">فقط جلوی دوربین بایستید — {fa(TARGET_SAMPLES)} نمونه پشت‌سر هم خودکار ثبت می‌شود.</p>}
          {samples.isLoading ? <div className="py-8 text-center text-sm text-soft">در حال بارگذاری…</div>
            : !samples.data?.length ? <div className="rounded-2xl border border-dashed border-line py-10 text-center text-sm text-soft">هنوز نمونه‌ای ثبت نشده است</div>
              : <ul className="space-y-2">{samples.data.map((s, i) => (
                <li key={s.id} className="flex items-center gap-3 rounded-xl border border-line p-2.5">
                  {s.image ? <img src={s.image} alt="" className="h-12 w-12 rounded-lg object-cover" /> : <div className="grid h-12 w-12 place-items-center rounded-lg bg-muted text-soft"><ScanFace className="h-5 w-5" /></div>}
                  <div className="flex-1 text-sm"><p className="flex items-center gap-1.5 font-medium">نمونه {fa(i + 1)}{s.model_name !== FACE_MODEL_NAME && <Badge tone="bad">قدیمی</Badge>}</p>
                    <p className="text-xs text-soft">کیفیت: {s.quality ? fa(Math.round(s.quality * 100)) + '٪' : '—'}</p></div>
                  <button className="rounded-lg p-2 text-soft hover:bg-bad/10 hover:text-bad" title="حذف نمونه" onClick={async () => { await del(`/hr/employees/${employee.id}/faces/${s.id}`); inv('/hr/employees') }}><Trash2 className="h-4 w-4" /></button>
                </li>))}</ul>}
        </div>
      </div>
    </Modal>
  )
}
