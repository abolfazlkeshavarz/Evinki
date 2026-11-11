package warranty

import (
	"testing"
	"time"

	"github.com/evinki/platform/internal/platform/jalali"
)

func d(y, m, dd int) time.Time { return time.Date(y, time.Month(m), dd, 0, 0, 0, 0, time.UTC) }

func jd(y, m, dd int) time.Time { return dateOnly(jalali.FromJalali(y, m, dd)) }

func TestFormulaRoundTrip(t *testing.T) {
	f := Formula{{Type: "year", Digits: 4}, {Type: "text", Value: "fz"}, {Type: "month"}, {Type: "number", Value: "24"}, {Type: "serial", Digits: 5}}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 11, 12, 0, 0, 0, jalali.Tehran) // 1405/08/20
	code := f.Render(at, 123)
	if code != "1405FZ082400123" {
		t.Fatalf("got %s", code)
	}
	p, ok := f.Parse(code)
	if !ok || p.Year != 1405 || p.Month != 8 || p.Serial != "00123" {
		t.Fatalf("parse %+v %v", p, ok)
	}
	md, approx, ok := p.ManufactureDate()
	if !ok || !approx || !md.Equal(jd(1405, 8, 30)) { // month only -> last day of Aban
		t.Fatalf("mfg %v approx=%v ok=%v", md, approx, ok)
	}
	if _, ok := f.Parse("1405FZ132400123"); ok {
		t.Fatal("month 13 must not match")
	}
	if _, ok := f.Parse("1405fz082400123"); !ok {
		t.Fatal("codes are case-insensitive")
	}
}

func TestFormulaFullDateAndTwoDigitYear(t *testing.T) {
	f := Formula{{Type: "text", Value: "EV-"}, {Type: "year", Digits: 2}, {Type: "month"}, {Type: "day"}, {Type: "text", Value: "-"}, {Type: "serial", Digits: 4}}
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	p, ok := f.Parse("EV-050815-0007")
	if !ok || p.Year != 1405 || p.Day != 15 {
		t.Fatalf("%+v", p)
	}
	md, approx, ok := p.ManufactureDate()
	if !ok || approx || !md.Equal(jd(1405, 8, 15)) {
		t.Fatalf("mfg %v %v %v", md, approx, ok)
	}
	// Esfand 30 only exists in leap years; 1405 is not one
	if _, _, ok := (&Parsed{Year: 1405, Month: 12, Day: 30, Calendar: "jalali"}).ManufactureDate(); ok {
		t.Fatal("1405/12/30 does not exist")
	}
}

func TestFormulaGregorian(t *testing.T) {
	f := Formula{{Type: "year", Digits: 4, Calendar: "gregorian"}, {Type: "month"}, {Type: "serial", Digits: 3}}
	p, ok := f.Parse("202602001")
	if !ok {
		t.Fatal("no match")
	}
	md, _, _ := p.ManufactureDate()
	if !md.Equal(d(2026, 2, 28)) {
		t.Fatalf("%v", md)
	}
}

func TestFormulaValidate(t *testing.T) {
	bad := []Formula{
		{},
		{{Type: "text", Value: "A"}}, // no serial
		{{Type: "month"}, {Type: "serial", Digits: 3}},                // month without year
		{{Type: "year", Digits: 3}, {Type: "serial", Digits: 3}},      // bad year width
		{{Type: "serial", Digits: 3}, {Type: "serial", Digits: 3}},    // two serials
		{{Type: "number", Value: "12a"}, {Type: "serial", Digits: 3}}, // not digits
	}
	for i, f := range bad {
		if f.Validate() == nil {
			t.Errorf("case %d should be invalid", i)
		}
	}
}

func TestAddMonthsJalali(t *testing.T) {
	// 1405/08/30 + 18 months = 1407/02/31 -> clamped? Ordibehesht has 31 days
	if got := AddMonths(jd(1405, 8, 30), 18); !got.Equal(jd(1407, 2, 30)) {
		t.Fatalf("%v", got)
	}
	// clamp: 1405/06/31 + 6 months -> 1405/12/29 (Esfand 1405 has 29 days)
	if got := AddMonths(jd(1405, 6, 31), 6); !got.Equal(jd(1405, 12, 29)) {
		t.Fatalf("%v", got)
	}
}

func TestComputePeriods(t *testing.T) {
	mfg := jd(1404, 6, 31)      // made at the end of Shahrivar 1404
	purchase := jd(1405, 2, 10) // sold 8 months later
	m := computePeriods(purchase, &mfg, true, 18, 3, "manufacture")
	if !m.expiry.Equal(AddMonths(mfg, 18)) || m.basis != "manufacture" {
		t.Fatalf("manufacture basis: %v %s", m.expiry, m.basis)
	}
	p := computePeriods(purchase, &mfg, true, 18, 3, "purchase")
	if !p.expiry.Equal(AddMonths(purchase, 18)) || p.basis != "purchase" {
		t.Fatalf("purchase basis: %v", p.expiry)
	}
	b := computePeriods(purchase, &mfg, true, 18, 3, "best")
	if !b.expiry.Equal(AddMonths(purchase, 18)) {
		t.Fatalf("best must take the later date: %v", b.expiry)
	}
	// no date in the code -> falls back to purchase
	n := computePeriods(purchase, nil, false, 18, 3, "manufacture")
	if n.basis != "purchase" || !n.expiry.Equal(AddMonths(purchase, 18)) {
		t.Fatalf("fallback: %+v", n)
	}
	// device sold after its guarantee already ran out: clamp to the purchase date, never before it
	old := jd(1402, 1, 1)
	late := computePeriods(jd(1405, 1, 1), &old, false, 18, 3, "manufacture")
	if !late.expiry.Equal(jd(1405, 1, 1)) || late.goldenEnd == nil || late.goldenEnd.After(late.expiry) {
		t.Fatalf("clamp: %+v", late)
	}
	// golden never outlives the guarantee
	if m.goldenEnd.After(m.expiry) {
		t.Fatal("golden after expiry")
	}
}

func TestSeasonalCodeUsesSeasonEnd(t *testing.T) {
	// 1404 serial 2000 belongs to the autumn batch (closing month Azar = 9)
	c := CheckCode(ProductRules{Format: "evinki_vacuum", Prefix: "FZ"}, "1404FZ2400102000")
	if !c.Valid || c.Month != 9 || !c.SeasonOnly || c.ManufactureDate == "" || !c.DateApprox {
		t.Fatalf("%+v", c)
	}
	want := jd(1404, 9, 30).Format("2006-01-02")
	if c.ManufactureDate != want {
		t.Fatalf("got %s want %s", c.ManufactureDate, want)
	}
}
