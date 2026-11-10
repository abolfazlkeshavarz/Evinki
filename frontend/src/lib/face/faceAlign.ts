/**
 * هم‌ترازیِ چهره برای MobileFaceNet — تبدیل چهرهٔ خام به قابِ ۱۱۲×۱۱۲ استاندارد.
 *
 * مدل‌های خانوادهٔ ArcFace/MobileFaceNet روی چهرهٔ خام کار نمی‌کنند؛ باید طبق
 * قراردادِ خودِ InsightFace با ۵ نقطهٔ کلیدی (دو چشم، نوک بینی، دو گوشهٔ لب)
 * به یک قالبِ ثابتِ ۱۱۲×۱۱۲ «چرخانده و مقیاس داده» شوند — یعنی یک تبدیلِ
 * تشابهیِ دوبعدی (چرخش + مقیاسِ یکنواخت + انتقال، بدون برش/کجی). این ۵ نقطه
 * از همان ۶۸ نقطهٔ face-api (که قبلاً هم برای liveness استفاده می‌شد) گرفته
 * می‌شود؛ نیازی به مدلِ تشخیصِ نقاطِ جداگانه نیست.
 */
import type * as faceapi from '@vladmandic/face-api'

export const ALIGNED_SIZE = 112

type Pt = { x: number; y: number }

/** قالبِ استانداردِ ۵نقطه‌ایِ ArcFace روی بومِ ۱۱۲×۱۱۲ (ثابتِ عمومی، نه محتوای دارای مالکیت). */
const ARCFACE_TEMPLATE: Pt[] = [
  { x: 38.2946, y: 51.6963 }, // چشمِ سمتِ چپِ تصویر
  { x: 73.5318, y: 51.5014 }, // چشمِ سمتِ راستِ تصویر
  { x: 56.0252, y: 71.7366 }, // نوکِ بینی
  { x: 41.5493, y: 92.3655 }, // گوشهٔ چپِ لب
  { x: 70.7299, y: 92.2041 }, // گوشهٔ راستِ لب
]

function avgPoint(points: Pt[]): Pt {
  let x = 0
  let y = 0
  for (const p of points) {
    x += p.x
    y += p.y
  }
  return { x: x / points.length, y: y / points.length }
}

/** ۵ نقطهٔ کلیدیِ لازم برای هم‌ترازی را از ۶۸ نقطهٔ face-api استخراج می‌کند. */
export function fivePoints(landmarks: faceapi.FaceLandmarks68): Pt[] {
  const leftEye = avgPoint(landmarks.getLeftEye())
  const rightEye = avgPoint(landmarks.getRightEye())
  const nose = landmarks.getNose()
  const noseTip = nose[3] ?? nose[0]
  const mouth = landmarks.getMouth()
  const leftMouth = mouth[0]
  const rightMouth = mouth[6] ?? mouth[mouth.length - 1]
  return [leftEye, rightEye, noseTip, leftMouth, rightMouth]
}

/**
 * تبدیلِ تشابهیِ ۲بعدی (چرخش + مقیاسِ یکنواخت + انتقال، بدونِ آینه) که با کمترین
 * مجموعِ مربعاتِ خطا نقاطِ src را به dst می‌برد — با روش کمترین مربعاتِ خطیِ
 * مستقیم (معادلِ Umeyama برای حالتِ بدونِ آینه، بدونِ نیاز به SVD عمومی):
 *
 *   x' = a·x − b·y + tx
 *   y' = b·x + a·y + ty
 *
 * چون این معادله در a,b,tx,ty خطی است، حلِ کمترین‌مربعات با حلِ یک دستگاهِ
 * ۴×۴ به‌دست می‌آید — بدونِ کتابخانهٔ جبرِ خطی.
 */
export function estimateSimilarity(
  src: Pt[],
  dst: Pt[],
): { a: number; b: number; tx: number; ty: number } {
  const n = src.length
  let Sx = 0, Sy = 0, Sxx_yy = 0, Spx = 0, Spy = 0, C1 = 0, C2 = 0
  for (let i = 0; i < n; i++) {
    const { x, y } = src[i]
    const { x: xp, y: yp } = dst[i]
    Sx += x
    Sy += y
    Sxx_yy += x * x + y * y
    Spx += xp
    Spy += yp
    C1 += x * xp + y * yp
    C2 += y * xp - x * yp
  }
  const D = Sxx_yy - (Sx * Sx + Sy * Sy) / n
  const a = (C1 - (Sx * Spx + Sy * Spy) / n) / D
  const b = (Sy * Spx - Sx * Spy) / n / D - C2 / D
  const tx = (Spx - a * Sx + b * Sy) / n
  const ty = (Spy - b * Sx - a * Sy) / n
  return { a, b, tx, ty }
}

/**
 * چهرهٔ روبه‌روی دوربین را با تبدیلِ تشابهیِ برآوردشده به یک بومِ ۱۱۲×۱۱۲
 * هم‌ترازشده می‌برد — دقیقاً ورودیِ موردِ انتظارِ MobileFaceNet.
 */
export function alignFace(
  source: HTMLVideoElement | HTMLCanvasElement,
  landmarks: faceapi.FaceLandmarks68,
): HTMLCanvasElement {
  const src = fivePoints(landmarks)
  const { a, b, tx, ty } = estimateSimilarity(src, ARCFACE_TEMPLATE)

  const canvas = document.createElement('canvas')
  canvas.width = ALIGNED_SIZE
  canvas.height = ALIGNED_SIZE
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas 2d context در دسترس نیست')

  const width = 'videoWidth' in source ? source.videoWidth : source.width
  const height = 'videoHeight' in source ? source.videoHeight : source.height

  // ctx.setTransform(a, b, c, d, e, f) یعنی: x'=a·x+c·y+e ، y'=b·x+d·y+f —
  // دقیقاً همان تبدیلِ بالا با c=−b ، d=a.
  ctx.setTransform(a, b, -b, a, tx, ty)
  ctx.drawImage(source, 0, 0, width, height)
  ctx.setTransform(1, 0, 0, 1, 0, 0)
  return canvas
}
