/**
 * موتور تشخیص چهره — کاملاً داخل مرورگر.
 *
 * چرا سمت مرورگر؟ چون تبلت ورودی کارخانه باید در قطعی اینترنت هم کار کند.
 * مدل‌ها یک‌بار دانلود و توسط Service Worker کش می‌شوند؛ پس از آن تشخیص بدون
 * هیچ ارتباطی با سرور انجام می‌شود و فقط نتیجه (تردد) در صف ارسال قرار می‌گیرد.
 *
 * دو مرحلهٔ جدا:
 *   ۱. یافتنِ چهره در فریم (SSD MobileNet v1) + ۶۸ نقطهٔ کلیدی — سبک، روی
 *      هر فریمِ بررسی‌شده اجرا می‌شود.
 *   ۲. استخراجِ بردارِ ویژگی با MobileFaceNet/ArcFace (ONNX Runtime Web) —
 *      فقط وقتی صدا زده می‌شود که مرحلهٔ ۱ یک چهرهٔ تکی و مناسب پیدا کرده
 *      باشد، نه روی هر فریمِ خام.
 *
 * بردارِ خروجیِ ArcFace همیشه L2-نرمال می‌شود؛ تطبیق با شباهتِ کسینوسی انجام
 * می‌شود (هرچه بیشتر، شبیه‌تر) نه فاصلهٔ اقلیدسی.
 */
import * as faceapi from '@vladmandic/face-api'
import { eyeAspectRatio, horizontalYaw } from './liveness'
import { alignFace } from './faceAlign'
import { embedAlignedFace, EMBEDDING_DIM, FACE_MODEL_NAME, preloadMobileFaceNet } from './mobileFaceNet'

const MODEL_URL = '/models'

/** شباهتِ کسینوسیِ کمینه برای «همان فرد» — قابلِ تنظیم از تنظیماتِ سرور. */
export const DEFAULT_THRESHOLD = 0.42

/** هم‌قدم با FACE_AMBIGUITY_MARGIN در تنظیمات سرور — پیش‌فرض تا وقتی گالری بارگذاری شود. */
export const DEFAULT_AMBIGUITY_MARGIN = 0.05

export { EMBEDDING_DIM, FACE_MODEL_NAME }

export interface DetectedFace {
  box: { x: number; y: number; width: number; height: number }
  score: number
  /** معیار چرخش افقی سر — ورودی تشخیص زنده بودن */
  yaw: number
  /** میانگین باز بودن چشم‌ها — برای تشخیص پلک */
  ear: number
  /** ۶۸ نقطهٔ کلیدی — لازم برای هم‌ترازیِ چهره قبل از MobileFaceNet */
  landmarks: faceapi.FaceLandmarks68
}

export interface MatchCandidate {
  employeeId: number
  fullName: string
  personnelCode: string
  photoPath?: string | null
  departmentName?: string | null
  vectors: Float32Array[]
}

export interface MatchResult {
  candidate: MatchCandidate
  similarity: number
}

/** نقاط کلیدی چهره را به معیارهای «زنده بودن» تبدیل می‌کند. */
function livenessSignals(landmarks: faceapi.FaceLandmarks68): { yaw: number; ear: number } {
  const leftEye = landmarks.getLeftEye()
  const rightEye = landmarks.getRightEye()
  const nose = landmarks.getNose()
  // در مدل ۶۸ نقطه‌ای، نقطه ۳۰ نوک بینی است و getNose از نقطه ۲۷ شروع می‌شود
  const noseTip = nose[3] ?? nose[0]
  return {
    yaw: horizontalYaw(noseTip, leftEye, rightEye),
    ear: (eyeAspectRatio(leftEye) + eyeAspectRatio(rightEye)) / 2,
  }
}

type Status = 'idle' | 'loading' | 'ready' | 'error'

class FaceEngine {
  status: Status = 'idle'
  error = ''
  /** موتور فعال TensorFlow (برای تشخیص/نقاط): webgl (سریع) یا cpu (کند ولی همه‌جا کار می‌کند) */
  backend = ''
  private loadPromise: Promise<void> | null = null
  /**
   * تشخیصِ چهره — نه تطبیق — با SSD MobileNet v1 به‌جای TinyFaceDetector.
   *
   * TinyFaceDetector سریع‌تر است ولی کادرِ چهره را کم‌دقیق‌تر پیدا می‌کند؛
   * یک کادرِ کج/نادقیق یعنی هم‌ترازیِ بعدی هم کمی نادرست می‌شود و بردارِ
   * MobileFaceNet را کمی به‌هم می‌ریزد. SSD MobileNet v1 کندتر است (چند ده
   * میلی‌ثانیهٔ بیشتر روی WebGL) ولی برای تبلتِ ثابتِ کنار درب، دقت مهم‌تر از
   * چند فریم در ثانیه است.
   */
  private options = new faceapi.SsdMobilenetv1Options({ minConfidence: 0.6, maxResults: 5 })

  get ready() {
    return this.status === 'ready'
  }

  /** بارگذاری مدل‌ها (یک‌بار). فراخوانی مکرر بی‌خطر است. */
  load(): Promise<void> {
    this.loadPromise ??= this.doLoad()
    return this.loadPromise
  }

  private async doLoad(): Promise<void> {
    this.status = 'loading'
    try {
      await this.selectBackend()
      await Promise.all([
        faceapi.nets.ssdMobilenetv1.loadFromUri(MODEL_URL),
        faceapi.nets.faceLandmark68Net.loadFromUri(MODEL_URL),
        preloadMobileFaceNet(),
      ])
      this.status = 'ready'
    } catch (err) {
      this.status = 'error'
      this.error =
        err instanceof Error ? err.message : 'بارگذاری مدل‌های تشخیص چهره ناموفق بود'
      this.loadPromise = null
      throw err
    }
  }

  /**
   * انتخاب موتور محاسباتی TensorFlow (برای تشخیص/نقاط — MobileFaceNet جداگانه
   * و همیشه با ONNX Runtime Web/WASM اجرا می‌شود، به این انتخاب کاری ندارد).
   *
   * توجه: `setBackend` در صورت ناموفق بودن، خطا پرتاب نمی‌کند بلکه `false`
   * برمی‌گرداند. پس حتماً باید مقدار بازگشتی را بررسی کرد، وگرنه روی دستگاهی که
   * WebGL ندارد هیچ موتوری فعال نمی‌شود و تشخیص چهره بی‌صدا از کار می‌افتد.
   */
  private async selectBackend(): Promise<void> {
    // تایپ‌های بسته، سطح tf را کامل صادر نمی‌کنند
    const tf = faceapi.tf as unknown as {
      setBackend: (name: string) => Promise<boolean>
      ready: () => Promise<void>
      getBackend: () => string | null
    }

    for (const name of ['webgl', 'cpu']) {
      try {
        if (await tf.setBackend(name)) {
          await tf.ready()
          this.backend = name
          if (name === 'cpu') {
            console.warn(
              'WebGL در دسترس نیست؛ تشخیص چهره روی CPU اجرا می‌شود و کندتر خواهد بود.',
            )
          }
          return
        }
      } catch {
        // موتور بعدی را امتحان کن
      }
    }
    throw new Error('هیچ موتور محاسباتی در دسترس نیست (نه WebGL و نه CPU)')
  }

  /** یک چهره را در تصویر پیدا می‌کند (بدون بردارِ ویژگی — سبک، برای هر فریمِ بررسی‌شده). */
  async detect(
    input: HTMLVideoElement | HTMLCanvasElement | HTMLImageElement,
  ): Promise<DetectedFace | null> {
    if (!this.ready) return null
    const result = await faceapi.detectSingleFace(input, this.options).withFaceLandmarks()

    if (!result) return null
    const { x, y, width, height } = result.detection.box
    return {
      box: { x, y, width, height },
      score: result.detection.score,
      landmarks: result.landmarks,
      ...livenessSignals(result.landmarks),
    }
  }

  /**
   * تشخیص با گزارش تعداد نفرات داخل کادر — در یک بار پردازش (بدون بردارِ ویژگی).
   *
   * اگر دو نفر جلوی دوربین باشند، `detectSingleFace` بی‌سروصدا یکی را انتخاب
   * می‌کند و ممکن است تردد به نام نفر اشتباه ثبت شود. اینجا تعداد را هم
   * برمی‌گردانیم تا تبلت در این حالت ثبت نکند و از کاربر بخواهد تنها بایستد.
   *
   * «نفر اصلی» بزرگ‌ترین چهره است، یعنی نزدیک‌ترین فرد به دوربین.
   */
  async detectPrimary(
    input: HTMLVideoElement | HTMLCanvasElement | HTMLImageElement,
  ): Promise<{ face: DetectedFace | null; count: number }> {
    if (!this.ready) return { face: null, count: 0 }

    const results = await faceapi.detectAllFaces(input, this.options).withFaceLandmarks()

    if (results.length === 0) return { face: null, count: 0 }

    const biggest = results.reduce((best, current) =>
      current.detection.box.area > best.detection.box.area ? current : best,
    )
    const { x, y, width, height } = biggest.detection.box
    return {
      face: {
        box: { x, y, width, height },
        score: biggest.detection.score,
        landmarks: biggest.landmarks,
        ...livenessSignals(biggest.landmarks),
      },
      count: results.length,
    }
  }

  /**
   * بردارِ ویژگیِ ۵۱۲بُعدیِ MobileFaceNet/ArcFace را برای یک چهرهٔ ازقبل‌یافته
   * حساب می‌کند — تنها نقطه‌ای که مدلِ سنگین‌تر اجرا می‌شود، و عمداً جدا از
   * `detect`/`detectPrimary` نگه داشته شده: فقط وقتی صدا بزنید که چهره پایدار
   * و تکی است، نه روی هر فریمِ خام.
   */
  async getEmbedding(
    source: HTMLVideoElement | HTMLCanvasElement,
    face: DetectedFace,
  ): Promise<Float32Array> {
    const aligned = alignFace(source, face.landmarks)
    return embedAlignedFace(aligned)
  }
}

export const faceEngine = new FaceEngine()

// ------------------------------------------------------------------- تطبیق

/** شباهتِ کسینوسی بینِ دو بردارِ L2-نرمال‌شده — برابر با حاصل‌ضربِ داخلی. */
export function cosineSimilarity(a: Float32Array | number[], b: Float32Array | number[]): number {
  let dot = 0, na = 0, nb = 0
  for (let i = 0; i < a.length; i++) {
    dot += a[i] * b[i]
    na += a[i] * a[i]
    nb += b[i] * b[i]
  }
  const denom = Math.sqrt(na) * Math.sqrt(nb)
  return denom < 1e-8 ? 0 : dot / denom
}

/**
 * شباهتِ یک نامزد به چهرهٔ روبه‌روی دوربین.
 *
 * اگر فقط «نزدیک‌ترینِ یکیِ» نمونه‌های ثبت‌شده ملاک باشد، کافی است چهرهٔ
 * جلوی دوربین فقط به یکی از چند نمونهٔ یک نفر (حتی یک نمونهٔ کم‌کیفیت) شبیه
 * باشد تا تطبیق بخورد — همین باعث تأیید نادرستِ افرادِ ثبت‌نام‌نشده می‌شود.
 * به‌جایش میانگینِ بیشترین شباهت به چند نمونه (حداکثر ۳) ملاک است: یک فردِ
 * واقعی باید هم‌زمان به چند نمونهٔ همان شخص شبیه باشد، نه فقط یکی.
 */
function candidateSimilarity(embedding: Float32Array, candidate: MatchCandidate): number {
  const sims: number[] = []
  for (const vector of candidate.vectors) {
    if (vector.length !== embedding.length) continue
    sims.push(cosineSimilarity(embedding, vector))
  }
  if (sims.length === 0) return -Infinity
  sims.sort((a, b) => b - a) // نزولی: بیشترین شباهت اول
  const k = Math.min(3, sims.length)
  let sum = 0
  for (let i = 0; i < k; i++) sum += sims[i]
  return sum / k
}

/**
 * نزدیک‌ترین پرسنل به بردار داده‌شده را پیدا می‌کند.
 *
 * دو دلیل برای برگرداندنِ `null` (یعنی «شناسایی نشد»، نه یک تطبیقِ نادرست):
 *   ۱. بیشترین شباهت از آستانه کمتر است — کسی که اصلاً ثبت‌نام نشده.
 *   ۲. دو پرسنلِ متفاوت به‌اندازهٔ کافی به هم شبیه‌اند (اختلافِ شباهتشان کمتر
 *      از ambiguityMargin است) که نتوان مطمئن بود کدام است — مثلاً
 *      خواهر/برادرِ شبیه به هم که فقط یکی‌شان ثبت‌نام کرده. حدس زدن اینجا از
 *      رد کردن و هدایت به کد پرسنلی/PIN بدتر است.
 */
export function findBestMatch(
  embedding: Float32Array,
  candidates: MatchCandidate[],
  threshold = DEFAULT_THRESHOLD,
  ambiguityMargin = DEFAULT_AMBIGUITY_MARGIN,
): MatchResult | null {
  let best: MatchResult | null = null
  // بیشترین شباهتِ یک نامزدِ «دیگر» (غیر از فردِ فعلاً برنده) — برای تشخیصِ ابهام
  let runnerUpSimilarity = -Infinity

  for (const candidate of candidates) {
    const candidateBest = candidateSimilarity(embedding, candidate)
    if (candidateBest === -Infinity) continue

    if (!best || candidateBest > best.similarity) {
      if (best) runnerUpSimilarity = Math.max(runnerUpSimilarity, best.similarity)
      best = { candidate, similarity: candidateBest }
    } else {
      runnerUpSimilarity = Math.max(runnerUpSimilarity, candidateBest)
    }
  }

  if (!best || best.similarity < threshold) return null
  if (best.similarity - runnerUpSimilarity < ambiguityMargin) return null
  return best
}

/** شباهتِ کسینوسی را به درصد اطمینان قابل‌فهم برای کاربر تبدیل می‌کند. */
export function similarityToConfidence(similarity: number, threshold = DEFAULT_THRESHOLD): number {
  // شباهتِ ۱ (تطبیقِ کامل) → ۱۰۰٪؛ دقیقاً روی آستانه → حدودِ ۶۰٪.
  const raw = 0.6 + 0.4 * ((similarity - threshold) / (1 - threshold))
  return Math.round(Math.min(1, Math.max(0, raw)) * 100) / 100
}

/** فریم فعلی ویدیو را به JPEG کوچک تبدیل می‌کند (برای بایگانی تردد). */
export function captureSnapshot(video: HTMLVideoElement, maxWidth = 320): string | null {
  if (!video.videoWidth) return null
  const scale = Math.min(1, maxWidth / video.videoWidth)
  const canvas = document.createElement('canvas')
  canvas.width = Math.round(video.videoWidth * scale)
  canvas.height = Math.round(video.videoHeight * scale)
  const ctx = canvas.getContext('2d')
  if (!ctx) return null
  ctx.drawImage(video, 0, 0, canvas.width, canvas.height)
  return canvas.toDataURL('image/jpeg', 0.72)
}

/** بریدن ناحیه چهره از فریم — برای تصویر پروفایل هنگام ثبت‌نام. */
export function cropFace(
  video: HTMLVideoElement,
  box: DetectedFace['box'],
  size = 224,
): string | null {
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size
  const ctx = canvas.getContext('2d')
  if (!ctx) return null

  const pad = Math.max(box.width, box.height) * 0.28
  const sx = Math.max(0, box.x - pad)
  const sy = Math.max(0, box.y - pad)
  const side = Math.min(
    Math.max(box.width, box.height) + pad * 2,
    video.videoWidth - sx,
    video.videoHeight - sy,
  )
  ctx.drawImage(video, sx, sy, side, side, 0, 0, size, size)
  return canvas.toDataURL('image/jpeg', 0.85)
}
