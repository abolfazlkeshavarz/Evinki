// Persian labels and colours for every status in the system, in one place.
export type Tone = 'ok' | 'warn' | 'bad' | 'info' | 'mute'

type Map = Record<string, [string, Tone]>

export const guaranteeStatus: Map = {
  pending: ['در انتظار تایید', 'warn'], approved: ['فعال', 'ok'], rejected: ['رد شده', 'bad'],
  renewed: ['تمدید شده', 'ok'], cancelled: ['لغو شده', 'mute'], expired: ['منقضی', 'bad'],
}
export const tierLabel: Record<string, [string, Tone]> = { golden: ['طلایی', 'warn'], normal: ['عادی', 'ok'], expired: ['منقضی', 'bad'], '': ['—', 'mute'] }

export const repairStatus: Map = {
  requested: ['درخواست شده', 'warn'], assigned: ['محول شده', 'info'], in_progress: ['در حال انجام', 'info'],
  waiting_parts: ['منتظر قطعه', 'warn'], submitted: ['در انتظار بررسی', 'warn'], approved: ['تایید شده', 'ok'],
  rejected: ['رد شده', 'bad'], cancelled: ['لغو شده', 'mute'],
}
export const requestStatus: Map = {
  pending: ['در انتظار', 'warn'], approved: ['تایید شده', 'info'], delivered: ['تحویل شد', 'ok'],
  not_delivered: ['تحویل نشد', 'bad'], cancelled: ['لغو شده', 'mute'],
}
export const shipmentStatus: Map = {
  sent: ['ارسال شده', 'info'], received: ['دریافت شده', 'info'], invoiced: ['صورتحساب شده', 'warn'],
  paid: ['پرداخت شده', 'ok'], rejected: ['رد شده', 'bad'], cancelled: ['لغو شده', 'mute'],
}
export const returnState: Map = {
  none: ['بدون برگشت', 'mute'], pending: ['باید ارسال شود', 'warn'], in_shipment: ['در مرسوله', 'info'],
  received: ['رسیده', 'ok'], not_received: ['نرسیده', 'bad'],
}
export const orderStatus: Map = {
  pending_payment: ['در انتظار پرداخت', 'warn'], paid: ['پرداخت شده', 'info'], processing: ['در حال آماده‌سازی', 'info'],
  shipped: ['ارسال شده', 'info'], delivered: ['تحویل شده', 'ok'], cancelled: ['لغو شده', 'mute'], refunded: ['بازگشت وجه', 'bad'],
}
export const docStatus: Map = { pending: ['در انتظار تایید', 'warn'], approved: ['تایید شده', 'ok'], rejected: ['رد شده', 'bad'], cancelled: ['لغو شده', 'mute'] }
export const leaveStatus: Map = { pending: ['در انتظار', 'warn'], approved: ['تایید شده', 'ok'], rejected: ['رد شده', 'bad'] }
export const techStatus: Map = { pending: ['در انتظار تایید', 'warn'], approved: ['فعال', 'ok'], rejected: ['رد شده', 'bad'] }
export const surveyStatus: Map = { draft: ['پیش‌نویس', 'mute'], active: ['فعال', 'ok'], closed: ['بسته', 'mute'] }
export const dayStatus: Map = {
  present: ['حاضر', 'ok'], absent: ['غایب', 'bad'], leave: ['مرخصی', 'info'], mission: ['مأموریت', 'info'],
  holiday: ['تعطیل', 'mute'], weekend: ['تعطیل هفتگی', 'mute'], incomplete: ['ناقص', 'warn'], pending: ['در جریان', 'mute'],
}
export const taskStatus: Map = { todo: ['انجام نشده', 'warn'], in_progress: ['در حال انجام', 'info'], done: ['انجام شد', 'ok'], cancelled: ['لغو شده', 'mute'] }
export const priority: Map = { low: ['کم', 'mute'], normal: ['عادی', 'info'], high: ['زیاد', 'warn'], urgent: ['فوری', 'bad'] }
export const method: Record<string, string> = { post: 'پست', courier: 'پیک', in_person: 'حضوری', other: 'سایر' }
export const leaveType: Record<string, string> = { daily: 'روزانه', hourly: 'ساعتی', sick: 'استعلاجی', emergency: 'اضطراری', mission: 'مأموریت', unpaid: 'بدون حقوق' }
export const ledgerKind: Record<string, string> = { part_buyback: 'خرید قطعه قدیمی', labor: 'اجرت تعمیر', adjustment: 'اصلاحیه', payment: 'پرداخت' }
export const roleTitle: Record<string, string> = {
  super_admin: 'مدیر ارشد', admin: 'مدیر سیستم', technical: 'کاربر فنی', shop_manager: 'مدیر فروشگاه', warehouse_manager: 'مدیر انبار',
  storekeeper: 'انباردار', hr_manager: 'سرپرست منابع انسانی', viewer: 'ناظر', technician: 'تکنسین', customer: 'مشتری',
}
export const stageLabel: Record<string, string> = {
  unassigned: 'بدون تکنسین', in_progress: 'در حال انجام', awaiting_review: 'منتظر بررسی', parts_to_approve: 'درخواست قطعه',
  parts_in_transit: 'قطعه در راه', parts_to_return: 'قطعه قدیمی نزد تکنسین', parcel_in_transit: 'مرسوله در راه',
  awaiting_invoice: 'منتظر صورتحساب', awaiting_payment: 'منتظر پرداخت', complete: 'تکمیل', closed: 'بسته',
}
export const stageOrder = Object.keys(stageLabel)
