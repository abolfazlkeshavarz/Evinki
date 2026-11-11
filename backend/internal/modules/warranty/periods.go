package warranty

import (
	"encoding/json"
	"time"
)

// periods is the outcome of the guarantee rules for one device.
type periods struct {
	purchase, expiry       time.Time
	goldenStart, goldenEnd *time.Time
	mfg                    *time.Time // manufacture date read from the code (or typed by an admin)
	approx                 bool       // only a month / season was known
	basis                  string     // what the expiry was counted from: manufacture | purchase
}

// computePeriods applies the product's guarantee rules.
//
//	basis "manufacture": expiry = manufacture date + N months (falls back to the purchase date when the code has no date)
//	basis "purchase":    expiry = purchase date + N months
//	basis "best":        whichever of the two is later (the option that is better for the customer)
//
// When the code only names a month or a season, the manufacture date is the LAST day of that month, which is the
// customer-friendly choice. The golden window always starts at the purchase date and never outlives the guarantee.
// Months are Solar Hijri months. An expiry before the purchase date (a device sold long after it was made) is
// clamped to the purchase date, i.e. the guarantee shows as expired.
func computePeriods(purchase time.Time, mfg *time.Time, approx bool, months, golden int, basis string) periods {
	p := periods{purchase: purchase, mfg: mfg, approx: approx}
	fromPurchase := AddMonths(purchase, months)
	switch {
	case mfg == nil || basis == "purchase":
		p.expiry, p.basis = fromPurchase, "purchase"
	case basis == "best":
		fromMfg := AddMonths(*mfg, months)
		if fromMfg.After(fromPurchase) {
			p.expiry, p.basis = fromMfg, "manufacture"
		} else {
			p.expiry, p.basis = fromPurchase, "purchase"
		}
	default:
		p.expiry, p.basis = AddMonths(*mfg, months), "manufacture"
	}
	if p.expiry.Before(purchase) {
		p.expiry = purchase
	}
	if golden > 0 {
		s, e := purchase, AddMonths(purchase, golden)
		if e.After(p.expiry) {
			e = p.expiry
		}
		p.goldenStart, p.goldenEnd = &s, &e
	}
	return p
}

// mfgFromCode reads the manufacture date out of a code, if the product's rules allow it.
func mfgFromCode(r ProductRules, code string) (*time.Time, bool) {
	c := CheckCode(r, code)
	if !c.Valid || c.ManufactureDate == "" {
		return nil, false
	}
	t, err := time.Parse("2006-01-02", c.ManufactureDate)
	if err != nil {
		return nil, false
	}
	return &t, c.DateApprox
}

func parseFormula(raw json.RawMessage) Formula {
	var f Formula
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &f)
	}
	if f.Validate() != nil {
		return nil
	}
	return f
}
