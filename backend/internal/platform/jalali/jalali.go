// Package jalali: Tehran time and Solar Hijri helpers on top of go-persian-calendar.
package jalali

import (
	"fmt"
	"time"

	ptime "github.com/yaa110/go-persian-calendar"
)

var Tehran *time.Location

func init() {
	l, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		// Iran has had no DST since 2022, so a fixed offset is a safe fallback
		// for minimal containers that ship without tzdata.
		l = time.FixedZone("Asia/Tehran", 3*3600+1800)
	}
	Tehran = l
}

func Now() time.Time { return time.Now().In(Tehran) }

// Date converts a Gregorian instant to (year, month, day) in the Solar Hijri calendar, Tehran time.
func Date(t time.Time) (y, m, d int) {
	p := ptime.New(t.In(Tehran))
	return p.Year(), int(p.Month()), p.Day()
}

// Format renders 1405/03/17.
func Format(t time.Time) string {
	y, m, d := Date(t)
	return fmt.Sprintf("%04d/%02d/%02d", y, m, d)
}

func Today() (int, int, int) { return Date(time.Now()) }

// FromJalali builds a Gregorian date (midnight, Tehran) from a Solar Hijri date.
func FromJalali(y, m, d int) time.Time {
	return ptime.Date(y, ptime.Month(m), d, 0, 0, 0, 0, Tehran).Time()
}

// IranWeekday numbers days the Iranian way: Saturday=0 ... Friday=6.
func IranWeekday(t time.Time) int {
	return (int(t.In(Tehran).Weekday()) + 1) % 7
}

var monthNames = []string{"", "فروردین", "اردیبهشت", "خرداد", "تیر", "مرداد", "شهریور", "مهر", "آبان", "آذر", "دی", "بهمن", "اسفند"}

func MonthName(m int) string {
	if m < 1 || m > 12 {
		return ""
	}
	return monthNames[m]
}

// MonthsBetween counts whole Solar Hijri months from (y1,m1) to (y2,m2).
func MonthsBetween(y1, m1, y2, m2 int) int { return (y2-y1)*12 + (m2 - m1) }

func StartOfDay(t time.Time) time.Time {
	t = t.In(Tehran)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Tehran)
}
