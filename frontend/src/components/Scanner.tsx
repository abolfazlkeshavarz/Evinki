import { useEffect, useRef, useState } from 'react'
import jsQR from 'jsqr'
import { Camera, CameraOff } from 'lucide-react'

/**
 * Camera QR/barcode reader. Uses the browser's BarcodeDetector when present and
 * falls back to jsQR (iOS Safari). Browsers only allow the camera on HTTPS or
 * localhost, so a clear message is shown otherwise and manual entry still works.
 */
export function Scanner({ onCode, paused }: { onCode: (code: string) => void; paused?: boolean }) {
  const video = useRef<HTMLVideoElement>(null)
  const [state, setState] = useState<'idle' | 'on' | 'error'>('idle')
  const [err, setErr] = useState('')
  const last = useRef({ code: '', at: 0 })
  const stream = useRef<MediaStream | null>(null)

  const stop = () => { stream.current?.getTracks().forEach((t) => t.stop()); stream.current = null; setState('idle') }
  useEffect(() => stop, [])

  const start = async () => {
    if (!window.isSecureContext) { setErr('دوربین فقط روی آدرس امن (https) کار می‌کند. می‌توانید کد را دستی وارد کنید.'); setState('error'); return }
    try {
      stream.current = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' }, audio: false })
      const v = video.current!
      v.srcObject = stream.current
      await v.play()
      setState('on')
      const Detector = (window as any).BarcodeDetector
      const det = Detector ? new Detector({ formats: ['qr_code', 'code_128', 'ean_13', 'code_39'] }) : null
      const canvas = document.createElement('canvas')
      const ctx = canvas.getContext('2d', { willReadFrequently: true })!
      const tick = async () => {
        if (!stream.current) return
        if (!paused && v.readyState >= 2) {
          let code = ''
          try {
            if (det) code = (await det.detect(v))[0]?.rawValue ?? ''
            else {
              canvas.width = v.videoWidth; canvas.height = v.videoHeight
              ctx.drawImage(v, 0, 0)
              code = jsQR(ctx.getImageData(0, 0, canvas.width, canvas.height).data, canvas.width, canvas.height)?.data ?? ''
            }
          } catch { /* frame not ready */ }
          const now = Date.now()
          // the same label stays in view for a while; only report it again after 2.5 s
          if (code && (code !== last.current.code || now - last.current.at > 2500)) {
            last.current = { code, at: now }
            navigator.vibrate?.(60)
            onCode(code)
          }
        }
        setTimeout(tick, 150)
      }
      tick()
    } catch (e: any) {
      setErr(e?.name === 'NotAllowedError' ? 'اجازه دسترسی به دوربین داده نشد.' : 'دوربین در دسترس نیست.')
      setState('error')
    }
  }

  return (
    <div className="overflow-hidden rounded-2xl border border-line bg-black/90">
      <div className={state === 'on' ? 'relative aspect-[4/3]' : 'hidden'}>
        <video ref={video} className="h-full w-full object-cover" playsInline muted />
        <div className="pointer-events-none absolute inset-8 rounded-2xl border-2 border-white/70" />
      </div>
      {state !== 'on' && (
        <div className="flex flex-col items-center gap-3 p-8 text-center text-white">
          {state === 'error' ? <CameraOff className="h-8 w-8 opacity-70" /> : <Camera className="h-8 w-8 opacity-70" />}
          {err && <p className="text-sm opacity-80">{err}</p>}
          <button className="btn bg-white text-black" onClick={start}>{state === 'error' ? 'تلاش دوباره' : 'روشن کردن دوربین'}</button>
        </div>
      )}
      {state === 'on' && <button className="w-full bg-black/60 py-2 text-xs text-white" onClick={stop}>خاموش کردن دوربین</button>}
    </div>
  )
}
