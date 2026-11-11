package warranty

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evinki/platform/internal/platform/jalali"
)

// CodeCheck is the answer to "is this a plausible guarantee code for this product?".
type CodeCheck struct {
	Valid           bool   `json:"valid"`
	CanRegister     bool   `json:"can_register"`
	Year            int    `json:"year,omitempty"`
	Month           int    `json:"month,omitempty"`
	MonthName       string `json:"month_name,omitempty"`
	Day             int    `json:"day,omitempty"`
	SeasonOnly      bool   `json:"season_only"`
	ManufactureDate string `json:"manufacture_date,omitempty"` // ISO; the best option for the customer when the code is not exact
	DateApprox      bool   `json:"date_approx"`
	ExpiryPreview   string `json:"expiry_date,omitempty"` // what the guarantee would end on (needs purchase_date)
	GoldenPreview   string `json:"golden_expiry_date,omitempty"`
	GuaranteeMonths int    `json:"guarantee_months,omitempty"`
	Basis           string `json:"expiry_basis,omitempty"`
	MonthsSince     int    `json:"months_since_manufacture"`
	Message         string `json:"message"`
	MessageLevel    string `json:"message_level"` // success | warning | error
}

// ProductRules is the slice of a product row that code validation needs.
type ProductRules struct {
	Format  string
	Prefix  string
	Pattern string
	Formula Formula
}

var simpleCode = regexp.MustCompile(`^[A-Za-z0-9\-_]{4,40}$`)

// Seasonal ("old style") codes carry no month: {year}{PREFIX}{D?}240010{serial:4}.
// The month is recovered from which production batch the serial falls into.
// These tables describe one real production line and are not derivable from
// the code; only years with recorded batches parse at all.
type batch struct{ lo, hi, month int } // hi == 0: open ended

var (
	seasonal1403Plain = []batch{{1, 700, 3}, {701, 1370, 6}}
	seasonal1403D     = []batch{{1, 1800, 9}, {1801, 2958, 12}}
	seasonal1404      = []batch{{1, 1200, 3}, {1201, 1900, 6}, {1901, 3400, 9}, {3401, 0, 12}}
)

func seasonalMonth(year int, hasD bool, serial int) (int, bool) {
	var table []batch
	switch year {
	case 1403:
		table = seasonal1403Plain
		if hasD {
			table = seasonal1403D
		}
	case 1404:
		table = seasonal1404
	default:
		return 0, false
	}
	for _, b := range table {
		if serial >= b.lo && (b.hi == 0 || serial <= b.hi) {
			return b.month, true
		}
	}
	return 0, false
}

func CheckCode(rules ProductRules, code string) CodeCheck {
	code = strings.TrimSpace(code)
	bad := func(msg string) CodeCheck { return CodeCheck{Message: msg, MessageLevel: "error"} }
	if code == "" {
		return bad("کد گارانتی نمی‌تواند خالی باشد")
	}
	if len(rules.Formula) > 0 {
		return checkFormula(rules.Formula, code)
	}
	if rules.Pattern != "" {
		re, err := regexp.Compile(rules.Pattern)
		if err == nil && !re.MatchString(code) {
			return bad("قالب کد گارانتی برای این محصول نامعتبر است")
		}
		if err == nil {
			return CodeCheck{Valid: true, CanRegister: true, Message: "کد گارانتی معتبر است", MessageLevel: "success"}
		}
	}
	switch rules.Format {
	case "any":
		return CodeCheck{Valid: true, CanRegister: true, Message: "کد پذیرفته شد", MessageLevel: "success"}
	case "jalali_encoded", "evinki_vacuum":
		return checkEncoded(rules, code)
	}
	if !simpleCode.MatchString(code) {
		return bad("کد باید ۴ تا ۴۰ حرف انگلیسی/عدد باشد")
	}
	return CodeCheck{Valid: true, CanRegister: true, Message: "کد گارانتی معتبر است", MessageLevel: "success"}
}

func checkEncoded(rules ProductRules, code string) CodeCheck {
	prefix := strings.ToUpper(strings.TrimSpace(rules.Prefix))
	code = strings.ToUpper(code)
	if prefix == "" {
		return CodeCheck{Message: "پیشوند کد برای این محصول تنظیم نشده است", MessageLevel: "error"}
	}
	fail := CodeCheck{MessageLevel: "error",
		Message: fmt.Sprintf("فرمت کد نامعتبر است. فرمت صحیح: سال%sماهسریال (مثال: 1405%s0912345)", prefix, prefix)}

	// current: {year:4}{prefix}{month:2}{serial:5}, from 1405 on
	cur := regexp.MustCompile(`^(\d{4})` + regexp.QuoteMeta(prefix) + `(\d{2})(\d{5})$`)
	if m := cur.FindStringSubmatch(code); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if y >= 1405 && mo >= 1 && mo <= 12 {
			return build(y, mo, false)
		}
		return fail
	}
	// old seasonal codes (evinki vacuum line only)
	if rules.Format == "evinki_vacuum" {
		base := strings.TrimSuffix(prefix, "D")
		old := regexp.MustCompile(`^(\d{4})` + regexp.QuoteMeta(base) + `(D?)240010(\d{4})$`)
		if m := old.FindStringSubmatch(code); m != nil {
			y, _ := strconv.Atoi(m[1])
			serial, _ := strconv.Atoi(m[3])
			if mo, ok := seasonalMonth(y, m[2] == "D", serial); ok {
				return build(y, mo, true)
			}
		}
	}
	return fail
}

// build applies the 24-month eligibility window counted from manufacture.
func build(year, month int, seasonOnly bool) CodeCheck {
	ty, tm, _ := jalali.Today()
	since := jalali.MonthsBetween(year, month, ty, tm)
	md := endOfJalaliMonth(year, month)
	c := CodeCheck{Valid: true, Year: year, Month: month, MonthName: jalali.MonthName(month), SeasonOnly: seasonOnly, MonthsSince: since,
		ManufactureDate: md.Format("2006-01-02"), DateApprox: true}
	if since > 24 {
		c.Message = fmt.Sprintf("کد معتبر است اما %d ماه از تولید گذشته و مهلت ثبت گارانتی تمام شده است", since)
		c.MessageLevel = "warning"
		return c
	}
	c.CanRegister = true
	c.Message = fmt.Sprintf("کد معتبر است (تولید: %d / %s)", year, c.MonthName)
	c.MessageLevel = "success"
	return c
}

// checkFormula validates a code against the product's formula and reads the manufacture date from it.
func checkFormula(f Formula, code string) CodeCheck {
	bad := func(msg string) CodeCheck { return CodeCheck{Message: msg, MessageLevel: "error"} }
	p, ok := f.Parse(code)
	if !ok {
		return bad("قالب کد گارانتی نامعتبر است؛ نمونه درست: " + f.Example())
	}
	c := CodeCheck{Valid: true, CanRegister: true, Message: "کد گارانتی معتبر است", MessageLevel: "success", Day: p.Day}
	md, approx, ok := p.ManufactureDate()
	if p.Year == 0 {
		return c // the formula carries no date: nothing to compute from
	}
	if !ok {
		return bad("تاریخ داخل کد معتبر نیست (ماه یا روز نادرست)")
	}
	if md.After(time.Now().AddDate(0, 0, 1)) {
		return bad("تاریخ تولید داخل کد در آینده است")
	}
	jy, jm, _ := jalali.Date(time.Date(md.Year(), md.Month(), md.Day(), 12, 0, 0, 0, jalali.Tehran))
	c.Year, c.Month, c.MonthName = jy, jm, jalali.MonthName(jm)
	c.ManufactureDate, c.DateApprox = md.Format("2006-01-02"), approx
	ty, tm, _ := jalali.Today()
	c.MonthsSince = jalali.MonthsBetween(jy, jm, ty, tm)
	if c.MonthsSince > 24 {
		c.CanRegister = false
		c.Message = fmt.Sprintf("کد معتبر است اما %d ماه از تولید گذشته و مهلت ثبت گارانتی تمام شده است", c.MonthsSince)
		c.MessageLevel = "warning"
		return c
	}
	c.Message = "کد گارانتی معتبر است (تولید: " + jalali.Format(md) + ")"
	if approx {
		c.Message = "کد گارانتی معتبر است (تولید تا پایان " + c.MonthName + " " + strconv.Itoa(jy) + ")"
	}
	return c
}
