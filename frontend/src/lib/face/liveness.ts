/**
 * هندسهٔ کمکیِ چهره — نسبتِ چرخشِ افقیِ سر و بازبودنِ چشم‌ها.
 *
 * قبلاً پایهٔ یک چالشِ «سر را بچرخانید» بود که برای تأییدِ تردد استفاده
 * می‌شد؛ آن روش کند و برای پرسنل گیج‌کننده بود، پس در کیوسک با یک تأییدِ
 * ساده‌تر (چند ثانیه نگاه‌داشتنِ چهره جلوی دوربین، در Kiosk.tsx) جایگزین شد.
 * این دو تابع هنوز در faceEngine.ts برای محاسبهٔ face.yaw/face.ear استفاده
 * می‌شوند (برای مصارف احتمالیِ بعدی)، هرچند کیوسک دیگر خودش آن‌ها را نمی‌خواند.
 */

/** میانگین نسبت باز بودن دو چشم از نقاط ۶تایی هر چشم. */
export function eyeAspectRatio(eye: { x: number; y: number }[]): number {
  if (eye.length < 6) return 0
  const d = (a: { x: number; y: number }, b: { x: number; y: number }) =>
    Math.hypot(a.x - b.x, a.y - b.y)
  const horizontal = d(eye[0], eye[3])
  if (horizontal < 1e-6) return 0
  return (d(eye[1], eye[5]) + d(eye[2], eye[4])) / (2 * horizontal)
}

/**
 * معیار چرخش افقی سر.
 *
 * نسبت فاصله نوک بینی تا مرکز هر چشم را می‌سنجد. چون خارج‌قسمت است، به اندازه
 * صورت و فاصله از دوربین وابسته نیست.
 */
export function horizontalYaw(
  noseTip: { x: number; y: number },
  leftEye: { x: number; y: number }[],
  rightEye: { x: number; y: number }[],
): number {
  const centroid = (points: { x: number; y: number }[]) => ({
    x: points.reduce((sum, p) => sum + p.x, 0) / points.length,
    y: points.reduce((sum, p) => sum + p.y, 0) / points.length,
  })
  const l = centroid(leftEye)
  const r = centroid(rightEye)
  const dl = Math.hypot(noseTip.x - l.x, noseTip.y - l.y)
  const dr = Math.hypot(noseTip.x - r.x, noseTip.y - r.y)
  const total = dl + dr
  if (total < 1e-6) return 0
  return (dl - dr) / total
}
