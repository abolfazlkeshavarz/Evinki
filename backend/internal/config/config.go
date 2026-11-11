// Package config reads all runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env       string // development | production
	Addr      string
	PublicURL string // address customers use; builds links in SMS and payment callbacks
	DBURL     string

	JWTSecret  string
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	UploadDir     string
	UpdatesDir    string // where uploaded image bundles wait for the host-side updater (scripts/updater.sh)
	MaxUploadSize int64
	CORSOrigins   []string

	SMSEnabled  bool
	SMSUsername string
	SMSPassword string

	PaymentGateway  string // mock | zarinpal
	ZarinpalMerchant string
	ZarinpalSandbox  bool

	// SECRET_KEY of the old attendance system: lets its kiosk API keys keep working after import.
	LegacyDeviceSecret string

	// Seeded on first boot when no user exists.
	BootstrapAdminUser string
	BootstrapAdminPass string
}

// placeholders that ship in .env.example; production refuses to boot with them.
var weakSecrets = []string{"", "change-me", "changeme", "secret", "your-super-secret-jwt-key-change-in-production"}

func Load() (*Config, error) {
	c := &Config{
		Env:       get("APP_ENV", "development"),
		Addr:      get("APP_ADDR", ":8080"),
		PublicURL: strings.TrimRight(get("PUBLIC_URL", "http://localhost:8080"), "/"),
		DBURL:     get("DATABASE_URL", ""),

		JWTSecret:  get("JWT_SECRET", ""),
		AccessTTL:  time.Duration(geti("ACCESS_TTL_MINUTES", 30)) * time.Minute,
		RefreshTTL: time.Duration(geti("REFRESH_TTL_DAYS", 30)) * 24 * time.Hour,

		UploadDir:     get("UPLOAD_DIR", "./uploads"),
		UpdatesDir:    get("UPDATES_DIR", "./updates"),
		MaxUploadSize: int64(geti("MAX_UPLOAD_MB", 10)) << 20,
		CORSOrigins:   splitCSV(get("CORS_ORIGINS", "http://localhost:5173")),

		SMSEnabled:  getb("SMS_ENABLED", false),
		SMSUsername: get("SMS_USERNAME", ""),
		SMSPassword: get("SMS_PASSWORD", ""),

		PaymentGateway:   get("PAYMENT_GATEWAY", "mock"),
		ZarinpalMerchant: get("ZARINPAL_MERCHANT_ID", ""),
		ZarinpalSandbox:  getb("ZARINPAL_SANDBOX", false),

		LegacyDeviceSecret: get("LEGACY_DEVICE_KEY_SECRET", ""),

		BootstrapAdminUser: get("BOOTSTRAP_ADMIN_USER", "admin"),
		BootstrapAdminPass: get("BOOTSTRAP_ADMIN_PASSWORD", ""),
	}
	if c.DBURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if c.Env == "production" {
		for _, w := range weakSecrets {
			if strings.EqualFold(c.JWTSecret, w) {
				return nil, fmt.Errorf("JWT_SECRET is missing or a known placeholder")
			}
		}
		if len(c.JWTSecret) < 32 {
			return nil, fmt.Errorf("JWT_SECRET must be at least 32 characters in production")
		}
		if c.PaymentGateway == "mock" {
			return nil, fmt.Errorf("PAYMENT_GATEWAY=mock is not allowed in production")
		}
	} else if c.JWTSecret == "" {
		c.JWTSecret = "dev-only-secret-dev-only-secret-dev-only"
	}
	return c, nil
}

func (c *Config) IsProd() bool { return c.Env == "production" }

func get(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}
func geti(k string, def int) int {
	if v, err := strconv.Atoi(get(k, "")); err == nil {
		return v
	}
	return def
}
func getb(k string, def bool) bool {
	if v, err := strconv.ParseBool(get(k, "")); err == nil {
		return v
	}
	return def
}
func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
