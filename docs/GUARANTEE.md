# Guarantees

## How the end date is calculated

Every product has **مدت گارانتی (ماه)**, **گارانتی طلایی** and **شروع محاسبه گارانتی از** (the *basis*):

| Basis | Expiry = |
|---|---|
| **تاریخ تولید** (default) | manufacture date + N months. The manufacture date is read **from the guarantee code**. If the code carries no date, the purchase date is used. |
| تاریخ خرید | purchase date + N months |
| بهترین حالت برای مشتری | the later of the two |

* Months are **Solar Hijri months** (the day of month is kept; `…/06/31 + 6 months` becomes `…/12/29`).
* When a code only tells the month, the manufacture date is the **last day of that month** (best for the customer). Old
  "seasonal" codes (1403/1404, serial → batch) name only a season: the **last day of the season's closing month** is used.
  A year-only formula uses the last day of the year.
* The golden period starts at the purchase date and never outlives the guarantee.
* A device sold after its guarantee already ran out is registered with an expiry equal to the purchase date (shows "expired").
* Registration stays open for 24 months after manufacture (as before).

The customer sees the manufacture date and the resulting end date **while typing the code**, and the guarantee page stores
*how* the end date was found ("محاسبه از تاریخ تولید") and whether an admin changed it by hand.

### Editing the dates

Guarantee page → **ویرایش تاریخ‌ها**: manufacture, purchase and **end date** (with ±3/6/12-month buttons and a reason that goes
to the audit log). A hand-set date is stamped "ویرایش دستی". **محاسبه مجدد طبق قوانین محصول** returns to the automatic value.
When creating a guarantee as an admin you can also type the manufacture date or end date directly.

## Code formulas

*Products → edit → فرمول ساخت کد گارانتی*. Build the code from pieces; the order is the order the code is written
(the builder always reads **left to right**, even though the page is right-to-left, and codes are shown in LTR everywhere):

| Piece | Meaning |
|---|---|
| متن ثابت | prefix / postfix / separator, e.g. `FZ`, `-` |
| سال | 4 or 2 digits, Jalali or Gregorian |
| ماه / روز | 2 digits |
| عدد ثابت | fixed digits, e.g. `24` |
| سریال | running number, 1–12 digits (exactly one per formula) |

Example `[year4][FZ][month][24][serial5]` → `1405FZ082400123`. The preview shows a sample code, the manufacture date read from
it and the validation regex. A formula replaces the format/prefix/regex fields; case does not matter when a customer types it.

**تولید کد** (product row) prints a batch (up to 5000) for a date and a starting serial: copy or download as an Excel file.

The old formats (`evinki_vacuum` with seasonal 1403/1404 codes, `jalali_encoded`, simple, any) keep working.

## Other tools from the old system

* **خروجی اکسل** on the guarantee list: filters by status, product, expiring in 30/60/90 days, code range.
* **سطل بازیافت**: deleted guarantees, customers, products, repairs and parts can be restored.
* **جستجوی سراسری** in the admin header (guarantees, customers, products, repairs, technicians).
* **ورود گروهی تکنسین‌ها** from .xlsx/.csv (with a template).
* **ادغام محصول**: two entries of the same device (the shop's *FZ530* and the guarantee system's *FZ530*) are merged; all guarantees,
  repairs, orders and carts move to the one you keep and blanks (photos, specs, code rules) are filled from the other.
  The guarantee import does this automatically: a legacy product with the same model code (`FZ530`) is merged into the
  shop product and a legacy category with the same name (e.g. جاروبرقی خانگی) into the existing one.
  After an import `evinkictl backfill-manufacture` fills the manufacture date of old guarantees from their codes (done automatically
  by the importer; the old expiry dates are not touched).
