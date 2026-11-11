package warranty

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evinki/platform/internal/platform/jalali"
)

// Token is one piece of a guarantee-code formula. A formula is an ordered list of tokens, e.g.
//
//	[year(4,jalali)] [text FZ] [month] [number 24] [serial(5)]   ->   1405FZ0924 00123
//
// Types: text (prefix/postfix/separator, letters), number (a fixed run of digits), year (2 or 4 digits,
// jalali or gregorian), month (2 digits), day (2 digits), serial (n digits, the running number).
type Token struct {
	Type     string `json:"type"`
	Value    string `json:"value,omitempty"`    // text / number
	Digits   int    `json:"digits,omitempty"`   // year (2|4) / serial (1..12)
	Calendar string `json:"calendar,omitempty"` // year: jalali (default) | gregorian
}

type Formula []Token

func (t Token) cal() string {
	if t.Calendar == "gregorian" {
		return "gregorian"
	}
	return "jalali"
}

// Validate refuses formulas that cannot work (no serial, two years, empty text ...).
func (f Formula) Validate() error {
	if len(f) == 0 {
		return fmt.Errorf("فرمول خالی است")
	}
	count := map[string]int{}
	for _, t := range f {
		count[t.Type]++
		switch t.Type {
		case "text":
			if strings.TrimSpace(t.Value) == "" {
				return fmt.Errorf("متن ثابت نباید خالی باشد")
			}
		case "number":
			if t.Value == "" || strings.Trim(t.Value, "0123456789") != "" {
				return fmt.Errorf("عدد ثابت فقط شامل رقم است")
			}
		case "year":
			if t.Digits != 2 && t.Digits != 4 {
				return fmt.Errorf("سال باید ۲ یا ۴ رقمی باشد")
			}
		case "month", "day":
		case "serial":
			if t.Digits < 1 || t.Digits > 12 {
				return fmt.Errorf("سریال باید بین ۱ تا ۱۲ رقم باشد")
			}
		default:
			return fmt.Errorf("بخش ناشناخته: %s", t.Type)
		}
	}
	if count["serial"] != 1 {
		return fmt.Errorf("فرمول باید دقیقاً یک بخش «سریال» داشته باشد")
	}
	for _, k := range []string{"year", "month", "day"} {
		if count[k] > 1 {
			return fmt.Errorf("هر بخش تاریخ فقط یک بار می‌تواند بیاید")
		}
	}
	if count["month"] > 0 && count["year"] == 0 {
		return fmt.Errorf("ماه بدون سال معنا ندارد")
	}
	if count["day"] > 0 && count["month"] == 0 {
		return fmt.Errorf("روز بدون ماه معنا ندارد")
	}
	return nil
}

// Regex returns the matching expression (anchored, upper-case codes) for the formula.
func (f Formula) Regex() string {
	var b strings.Builder
	b.WriteString("^")
	for _, t := range f {
		switch t.Type {
		case "text":
			b.WriteString("(?:" + regexp.QuoteMeta(strings.ToUpper(t.Value)) + ")")
		case "number":
			b.WriteString("(?:" + t.Value + ")")
		case "year":
			b.WriteString(fmt.Sprintf(`(\d{%d})`, t.Digits))
		case "month":
			b.WriteString(`(0[1-9]|1[0-2])`)
		case "day":
			b.WriteString(`(0[1-9]|[12]\d|3[01])`)
		case "serial":
			b.WriteString(fmt.Sprintf(`(\d{%d})`, t.Digits))
		}
	}
	b.WriteString("$")
	return b.String()
}

// Render builds a code for a manufacture date and serial number.
func (f Formula) Render(date time.Time, serial int64) string {
	jy, jm, jd := jalali.Date(date)
	gy, gm, gd := date.Year(), int(date.Month()), date.Day()
	var b strings.Builder
	for _, t := range f {
		switch t.Type {
		case "text":
			b.WriteString(strings.ToUpper(t.Value))
		case "number":
			b.WriteString(t.Value)
		case "year":
			y := jy
			if t.cal() == "gregorian" {
				y = gy
			}
			s := strconv.Itoa(y)
			if t.Digits == 2 {
				s = s[len(s)-2:]
			}
			b.WriteString(s)
		case "month":
			m := jm
			if f.calendar() == "gregorian" {
				m = gm
			}
			b.WriteString(fmt.Sprintf("%02d", m))
		case "day":
			d := jd
			if f.calendar() == "gregorian" {
				d = gd
			}
			b.WriteString(fmt.Sprintf("%02d", d))
		case "serial":
			b.WriteString(fmt.Sprintf("%0*d", t.Digits, serial))
		}
	}
	return b.String()
}

// calendar of the date parts: month/day follow the year token's calendar.
func (f Formula) calendar() string {
	for _, t := range f {
		if t.Type == "year" {
			return t.cal()
		}
	}
	return "jalali"
}

// Parsed is what a code revealed about the device.
type Parsed struct {
	Year, Month, Day int // in the formula's calendar; 0 when the formula has no such part
	Calendar         string
	Serial           string
}

// Parse matches a code against the formula.
func (f Formula) Parse(code string) (*Parsed, bool) {
	re, err := regexp.Compile(f.Regex())
	if err != nil {
		return nil, false
	}
	m := re.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(code)))
	if m == nil {
		return nil, false
	}
	p := &Parsed{Calendar: f.calendar()}
	i := 1
	for _, t := range f {
		switch t.Type {
		case "year":
			y, _ := strconv.Atoi(m[i])
			if t.Digits == 2 {
				if p.Calendar == "gregorian" {
					y += 2000
				} else if y >= 70 {
					y += 1300
				} else {
					y += 1400
				}
			}
			p.Year = y
			i++
		case "month":
			p.Month, _ = strconv.Atoi(m[i])
			i++
		case "day":
			p.Day, _ = strconv.Atoi(m[i])
			i++
		case "serial":
			p.Serial = m[i]
			i++
		}
	}
	return p, true
}

// ManufactureDate turns what the code says into one date, choosing the option that is best for the customer
// when the code is not exact: no month -> end of the year, no day -> last day of the month. The second result
// tells whether the date is an estimate.
func (p *Parsed) ManufactureDate() (time.Time, bool, bool) {
	if p.Year == 0 {
		return time.Time{}, false, false
	}
	approx := false
	m, d := p.Month, p.Day
	if m == 0 {
		m, approx = 12, true
	}
	if m < 1 || m > 12 {
		return time.Time{}, false, false
	}
	if p.Calendar == "gregorian" {
		last := time.Date(p.Year, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if d == 0 {
			d, approx = last, true
		}
		if d < 1 || d > last {
			return time.Time{}, false, false
		}
		return time.Date(p.Year, time.Month(m), d, 0, 0, 0, 0, time.UTC), approx, true
	}
	last := jalaliMonthLen(p.Year, m)
	if d == 0 {
		d, approx = last, true
	}
	if d < 1 || d > last {
		return time.Time{}, false, false
	}
	return dateOnly(jalali.FromJalali(p.Year, m, d)), approx, true
}

// ───────── Jalali date arithmetic ─────────

func jalaliMonthLen(y, m int) int {
	var next time.Time
	if m == 12 {
		next = jalali.FromJalali(y+1, 1, 1)
	} else {
		next = jalali.FromJalali(y, m+1, 1)
	}
	return int(next.Sub(jalali.FromJalali(y, m, 1)).Hours()/24 + 0.5)
}

// dateOnly keeps the calendar day (as seen in Tehran) and drops the time zone.
func dateOnly(t time.Time) time.Time {
	l := t.In(jalali.Tehran)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.UTC)
}

// AddMonths adds Solar Hijri months, keeping the day of month (clamped to the target month's length).
func AddMonths(t time.Time, n int) time.Time {
	y, m, d := jalali.Date(time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, jalali.Tehran))
	total := y*12 + (m - 1) + n
	ny, nm := total/12, total%12+1
	if l := jalaliMonthLen(ny, nm); d > l {
		d = l
	}
	return dateOnly(jalali.FromJalali(ny, nm, d))
}

// manufactureOfSeason: a season-only (old) code names the season's closing month; the customer-friendly
// date is the last day of that month.
func endOfJalaliMonth(y, m int) time.Time {
	return dateOnly(jalali.FromJalali(y, m, jalaliMonthLen(y, m)))
}

// Example renders a sample code (today, serial 1) so error messages and the builder can show the shape.
func (f Formula) Example() string { return f.Render(time.Now(), 1) }
