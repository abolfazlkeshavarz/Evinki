// Package seeddata embeds the product catalogue and photos taken from evinki.com, so a fresh
// installation can show the real products with `evinkictl seed-catalog` (no internet needed).
package seeddata

import "embed"

//go:embed products.json images/*
var FS embed.FS
