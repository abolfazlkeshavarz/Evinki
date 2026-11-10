package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/modules/catalog"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/seeddata"
)

type seedProduct struct {
	Slug            string            `json:"slug"`
	SKU             string            `json:"sku"`
	Name            string            `json:"name"`
	NameEN          string            `json:"name_en"`
	Category        string            `json:"category"`
	Summary         string            `json:"summary"`
	Description     string            `json:"description"`
	Specs           map[string]string `json:"specs"`
	Images          []string          `json:"images"`
	PartCode        string            `json:"part_code"`
	Stock           int               `json:"stock"`
	GuaranteeMonths int               `json:"guarantee_months"`
	GoldenMonths    int               `json:"golden_months"`
}

// seedCatalog loads the real products and photos of evinki.com (embedded in the binary).
// Safe to re-run: existing rows keep whatever an administrator changed (prices, stock, texts);
// only missing rows and missing photos are filled in. Everything is "price on request" like the
// public site - set prices in the admin panel to open online purchase.
func seedCatalog(ctx context.Context, d *app.Deps) error {
	raw, err := seeddata.FS.ReadFile("products.json")
	if err != nil {
		return err
	}
	var data struct {
		Categories []struct {
			Slug, Name string
			NameEN     string `json:"name_en"`
			Sort       int
		} `json:"categories"`
		Products []seedProduct `json:"products"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	_ = os.MkdirAll(d.Cfg.UploadDir, 0o755)

	cats := map[string]int64{}
	for _, c := range data.Categories {
		id, err := db.Scalar[int64](ctx, d.DB, `INSERT INTO categories(slug,name,name_en,sort_order) VALUES($1,$2,$3,$4)
			ON CONFLICT (slug) DO UPDATE SET slug=EXCLUDED.slug RETURNING id`, c.Slug, c.Name, c.NameEN, c.Sort)
		if err != nil {
			return err
		}
		cats[c.Slug] = id
	}

	urls := map[string]string{}
	imageURL := func(name string) (string, error) {
		if u, ok := urls[name]; ok {
			return u, nil
		}
		path := "seed-" + name
		if id, err := db.Scalar[string](ctx, d.DB, `SELECT id::text FROM files WHERE path=$1`, path); err == nil {
			urls[name] = "/api/v1/files/" + id
			return urls[name], nil
		}
		b, err := seeddata.FS.ReadFile("images/" + name)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(d.Cfg.UploadDir, path), b, 0o644); err != nil {
			return "", err
		}
		id, err := db.Scalar[string](ctx, d.DB, `INSERT INTO files(path,original,mime,size_bytes,purpose) VALUES($1,$2,$3,$4,'catalog') RETURNING id::text`,
			path, name, mime.TypeByExtension(filepath.Ext(name)), len(b))
		if err != nil {
			return "", err
		}
		urls[name] = "/api/v1/files/" + id
		return urls[name], nil
	}

	var vacuumID int64
	partIDs := []int64{}
	for _, p := range data.Products {
		imgs := []string{}
		for _, n := range p.Images {
			u, err := imageURL(n)
			if err != nil {
				return fmt.Errorf("image %s: %w", n, err)
			}
			imgs = append(imgs, u)
		}
		imgJSON, _ := json.Marshal(imgs)
		specs := p.Specs
		if specs == nil {
			specs = map[string]string{}
		}
		specJSON, _ := json.Marshal(specs)
		code, prefix := "simple", ""
		if p.SKU == "FZ530" {
			code, prefix = "evinki_vacuum", "FZ"
		}
		id, err := db.Scalar[int64](ctx, d.DB, `INSERT INTO products(category_id,sku,slug,name,name_en,summary,description,specs,images,price_on_request,stock_qty,
				guarantee_months,golden_months,code_format,code_prefix)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,TRUE,$10,$11,$12,$13,$14)
			ON CONFLICT (sku) DO UPDATE SET sku=EXCLUDED.sku RETURNING id`,
			cats[p.Category], p.SKU, p.Slug, p.Name, p.NameEN, p.Summary, p.Description, specJSON, imgJSON, p.Stock, p.GuaranteeMonths, p.GoldenMonths, code, prefix)
		if err != nil {
			return err
		}
		// photos / texts only where the admin has not provided their own yet
		if _, err := d.DB.Exec(ctx, `UPDATE products SET images=$2 WHERE id=$1 AND images='[]'::jsonb`, id, imgJSON); err != nil {
			return err
		}
		if _, err := d.DB.Exec(ctx, `UPDATE products SET specs=$2 WHERE id=$1 AND specs='{}'::jsonb`, id, specJSON); err != nil {
			return err
		}
		if p.PartCode == "" {
			vacuumID = id
			continue
		}
		// the same item as a spare part, so warehouse, technicians and the shop share one catalogue
		pid, err := db.Scalar[int64](ctx, d.DB, `INSERT INTO parts(code,name,image,note,qr_token) VALUES($1,$2,$3,$4,$5)
			ON CONFLICT (code) DO UPDATE SET code=EXCLUDED.code RETURNING id`, p.PartCode, p.Name, firstOr(imgs, ""), p.Summary, catalog.NewQRToken())
		if err != nil {
			return err
		}
		_, _ = d.DB.Exec(ctx, `UPDATE parts SET image=$2 WHERE id=$1 AND image=''`, pid, firstOr(imgs, ""))
		partIDs = append(partIDs, pid)
	}
	if vacuumID != 0 {
		for _, pid := range partIDs {
			if _, err := d.DB.Exec(ctx, `INSERT INTO part_compat(part_id,product_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, pid, vacuumID); err != nil {
				return err
			}
		}
	}
	fmt.Printf("catalogue loaded from evinki.com: %d products, %d photos, %d categories (all 'price on request')\n", len(data.Products), len(urls), len(cats))
	return nil
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}
