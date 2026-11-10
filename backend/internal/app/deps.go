// Package app holds the dependencies every module receives.
package app

import (
	"github.com/evinki/platform/internal/config"
	"github.com/evinki/platform/internal/platform/audit"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/payment"
)

type Deps struct {
	Cfg   *config.Config
	DB    *db.DB
	Auth  *auth.Service
	Audit *audit.Logger
	Pay   payment.Gateway
}
