package legacy

import (
	"regexp"
	"strings"
)

var modelRe = regexp.MustCompile(`(?:^|[^A-Z0-9])([A-Z]{1,4}-?[0-9]{2,4}[A-Z]?)(?:$|[^A-Z0-9])`)

// ModelKey pulls the model code ("FZ530") out of any product name / sku / prefix so the same device is
// recognised across systems ("جاروبرقی خانگی اوینکی FZ530", "Evinki FZ-530 Performance", "FZ530").
func ModelKey(parts ...string) string {
	for _, p := range parts {
		p = strings.ToUpper(strings.Map(func(r rune) rune {
			if r >= '۰' && r <= '۹' {
				return r - '۰' + '0'
			}
			return r
		}, p))
		if m := modelRe.FindStringSubmatch(p); m != nil {
			return strings.ReplaceAll(m[1], "-", "")
		}
	}
	return ""
}

// findProductByModel returns the id of a live shop product with the same model code.
func (im *Importer) findProductByModel(key string) (int64, bool) {
	if key == "" {
		return 0, false
	}
	rows, err := im.tx.Query(im.ctx, `SELECT id, sku, name, name_en FROM products WHERE deleted_at IS NULL AND sku NOT LIKE 'LEG-%' ORDER BY id`)
	if err != nil {
		return 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var sku, name, nameEn string
		if rows.Scan(&id, &sku, &name, &nameEn) == nil && (ModelKey(sku) == key || ModelKey(nameEn) == key || ModelKey(name) == key) {
			return id, true
		}
	}
	return 0, false
}

// findCategoryByName matches categories whose names are equal after Arabic/Persian letter normalisation.
func (im *Importer) findCategoryByName(name string) (int64, bool) {
	want := NormName(name)
	if want == "" {
		return 0, false
	}
	rows, err := im.tx.Query(im.ctx, `SELECT id, name FROM categories ORDER BY id`)
	if err != nil {
		return 0, false
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n string
		if rows.Scan(&id, &n) == nil && NormName(n) == want {
			return id, true
		}
	}
	return 0, false
}
