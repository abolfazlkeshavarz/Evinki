/**
 * استخراجِ بردارِ ویژگیِ چهره با MobileFaceNet (آموزش‌دیده با ArcFace)، اجرا
 * با ONNX Runtime Web — یک‌راست در مرورگر، بدون سرور.
 *
 * چرا این مدل: نسخهٔ قبلی (`faceRecognitionNet` خودِ face-api.js، معادلِ
 * ResNet ساده‌شدهٔ dlib) گاهی افرادِ ثبت‌نام‌نشده را هم تأیید می‌کرد.
 * MobileFaceNet+ArcFace استانداردِ صنعتیِ تشخیصِ چهرهٔ سبک است — همان
 * معماری‌ای که تقریباً همهٔ سامانه‌های تشخیصِ چهرهٔ موبایل/edge از آن استفاده
 * می‌کنند — با دقتِ به‌مراتب بالاتر و حجمِ مدلِ کوچک‌تر، مناسبِ گوشیِ
 * کم‌توانِ کیوسک. جزئیاتِ کامل مدل، منبع و مجوز: public/models/mobilefacenet/MODEL_INFO.md
 *
 * توجه به مجوز: وزن‌های این مدل (نه کدِ InsightFace) فقط برای مصارفِ
 * غیرتجاری مجازند — این پروژه یک سامانهٔ داخلیِ غیرتجاریِ کارخانه است.
 */
import * as ort from 'onnxruntime-web/wasm'
import { ALIGNED_SIZE } from './faceAlign'

const MODEL_URL = '/models/mobilefacenet/w600k_mbf.onnx'

// نامِ دقیقِ گره‌های ورودی/خروجی همان چیزی است که خودِ فایلِ ONNX دارد
// (با ابزارِ رسمیِ onnx بازرسی شده، نه حدسی) — اگر مدل عوض شد، این دو را با
// بازرسیِ فایلِ جدید به‌روز کنید.
const INPUT_NAME = 'input.1'
const OUTPUT_NAME = '516'
export const EMBEDDING_DIM = 512

/**
 * شناسهٔ مدل که همراهِ هر نمونه ذخیره می‌شود — سرور و همین برنامه با آن
 * می‌فهمند یک بردار با کدام مدل ساخته شده. بردارهای مدلِ قبلی (face-api-128)
 * هرگز نباید با این‌ها مقایسه شوند؛ فضای بردارِ دو مدل کاملاً متفاوت است.
 */
export const FACE_MODEL_NAME = 'mobilefacenet-arcface-512'

// یک‌رشته‌ای عمداً: روی گوشیِ ۲۰۱۸ نیازی به چندرشته‌ای (که به
// SharedArrayBuffer/هدرهای COOP-COEP نیاز دارد و بدونِ آن‌ها بی‌سروصدا کند یا
// ناپایدار می‌شود) نیست — این مدل آن‌قدر کوچک است که تک‌رشته‌ای هم برای
// «چند فریم موقعِ ثبت‌نام/تشخیص» به‌اندازهٔ کافی سریع است.
ort.env.wasm.numThreads = 1
// wasmPaths عمداً تنظیم نمی‌شود: با ایمپورتِ 'onnxruntime-web/wasm'، Vite خودش
// فایلِ .wasm را از طریق new URL(..., import.meta.url) پیدا و در dist/assets
// با نامِ هش‌دار کپی می‌کند — همان الگوی استانداردِ باندلرها. تنظیمِ دستیِ
// wasmPaths باعث می‌شد یک نسخهٔ ۱۴ مگابایتیِ اضافه در dist بماند.

let sessionPromise: Promise<ort.InferenceSession> | null = null

function loadSession(): Promise<ort.InferenceSession> {
  sessionPromise ??= ort.InferenceSession.create(MODEL_URL, {
    executionProviders: ['wasm'],
    graphOptimizationLevel: 'all',
  })
  return sessionPromise
}

/** بارگذاریِ زودهنگامِ مدل — تا اولین تشخیص/ثبت‌نامِ واقعی منتظرِ دانلود نماند. */
export function preloadMobileFaceNet(): Promise<void> {
  return loadSession().then(() => undefined)
}

/** بومِ ۱۱۲×۱۱۲ هم‌ترازشده را به تنسورِ CHW موردِ نیازِ مدل تبدیل می‌کند. */
function toInputTensor(canvas: HTMLCanvasElement): ort.Tensor {
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas 2d context در دسترس نیست')
  const { data } = ctx.getImageData(0, 0, ALIGNED_SIZE, ALIGNED_SIZE)

  const plane = ALIGNED_SIZE * ALIGNED_SIZE
  const chw = new Float32Array(3 * plane)
  // نرمال‌سازیِ دقیقاً مطابقِ آموزشِ مدل: (pixel − 127.5) / 128 برای هر کانال.
  for (let i = 0; i < plane; i++) {
    const o = i * 4
    chw[i] = (data[o] - 127.5) / 128
    chw[plane + i] = (data[o + 1] - 127.5) / 128
    chw[2 * plane + i] = (data[o + 2] - 127.5) / 128
  }
  return new ort.Tensor('float32', chw, [1, 3, ALIGNED_SIZE, ALIGNED_SIZE])
}

function l2Normalize(v: Float32Array): Float32Array {
  let sum = 0
  for (let i = 0; i < v.length; i++) sum += v[i] * v[i]
  const norm = Math.sqrt(sum) || 1e-8
  const out = new Float32Array(v.length)
  for (let i = 0; i < v.length; i++) out[i] = v[i] / norm
  return out
}

/**
 * بردارِ ۵۱۲بُعدیِ L2-نرمال‌شدهٔ یک چهرهٔ از قبل هم‌ترازشده (۱۱۲×۱۱۲) را برمی‌گرداند.
 * تنسورهای میانی بعد از inference آزاد می‌شوند تا روی گوشیِ کم‌رم نشتِ حافظه پیش نیاید.
 */
export async function embedAlignedFace(aligned: HTMLCanvasElement): Promise<Float32Array> {
  const session = await loadSession()
  const input = toInputTensor(aligned)
  let results: ort.InferenceSession.OnnxValueMapType | undefined
  try {
    results = await session.run({ [INPUT_NAME]: input })
    const output = results[OUTPUT_NAME]
    if (!output) {
      throw new Error(`خروجیِ مدل یافت نشد (انتظار گرهٔ "${OUTPUT_NAME}")`)
    }
    return l2Normalize(Float32Array.from(output.data as Float32Array))
  } finally {
    input.dispose()
    if (results) {
      for (const key of Object.keys(results)) results[key].dispose()
    }
  }
}

export { l2Normalize }
