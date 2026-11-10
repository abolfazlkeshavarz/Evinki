// Package auth: JWT access tokens, rotating refresh tokens, role-based
// permissions and the middleware that enforces them.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
)

type Principal struct {
	UserID   int64
	Username string
	Name     string
	Roles    []string
	perms    map[string]bool
}

func (p *Principal) Can(perm string) bool {
	if p == nil {
		return false
	}
	return p.perms["*"] || p.perms[perm]
}
func (p *Principal) HasRole(role string) bool {
	if p == nil {
		return false
	}
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}
func (p *Principal) PermList() []string {
	out := make([]string, 0, len(p.perms))
	if p.perms["*"] {
		return append(out, AllPermissions...)
	}
	for k := range p.perms {
		out = append(out, k)
	}
	return out
}
func (p *Principal) MainRole() string {
	if len(p.Roles) == 0 {
		return ""
	}
	return p.Roles[0]
}

type ctxKey struct{}

func From(ctx context.Context) *Principal {
	p, _ := ctx.Value(ctxKey{}).(*Principal)
	return p
}

// Service owns token issuing and the permission cache.
type Service struct {
	DB         *db.DB
	secret     []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration

	mu    sync.RWMutex
	cache map[string]map[string]bool // role -> permissions

	throttle sync.Map // ip|user -> *attempt
}

func New(d *db.DB, secret string, access, refresh time.Duration) *Service {
	return &Service{DB: d, secret: []byte(secret), AccessTTL: access, RefreshTTL: refresh}
}

// SeedRoles inserts the default roles and permissions that do not exist yet.
// Existing rows are left alone, so an admin's edits survive upgrades.
func (s *Service) SeedRoles(ctx context.Context) error {
	for _, r := range DefaultRoles() {
		tag, err := s.DB.Exec(ctx, `INSERT INTO roles(code,title_fa,title_en) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,
			r.Code, r.TitleFA, r.TitleEN)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			continue // role existed already: keep its permissions as edited
		}
		for _, p := range r.Perms {
			if _, err := s.DB.Exec(ctx, `INSERT INTO role_permissions(role_code,permission) VALUES($1,$2) ON CONFLICT DO NOTHING`, r.Code, p); err != nil {
				return err
			}
		}
	}
	return s.Reload(ctx)
}

func (s *Service) Reload(ctx context.Context) error {
	type row struct {
		Role string `db:"role_code"`
		Perm string `db:"permission"`
	}
	rows, err := db.All[row](ctx, s.DB, `SELECT role_code, permission FROM role_permissions`)
	if err != nil {
		return err
	}
	m := map[string]map[string]bool{}
	for _, r := range rows {
		if m[r.Role] == nil {
			m[r.Role] = map[string]bool{}
		}
		m[r.Role][r.Perm] = true
	}
	s.mu.Lock()
	s.cache = m
	s.mu.Unlock()
	return nil
}

func (s *Service) build(userID int64, username, name string, roles []string) *Principal {
	p := &Principal{UserID: userID, Username: username, Name: name, Roles: roles, perms: map[string]bool{}}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range roles {
		for perm := range s.cache[r] {
			p.perms[perm] = true
		}
	}
	return p
}

// ───────── tokens ─────────

type claims struct {
	Name  string   `json:"name"`
	User  string   `json:"usr"`
	Roles []string `json:"roles"`
	jwt.RegisteredClaims
}

type Tokens struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	ExpiresIn    int         `json:"expires_in"`
	TokenType    string      `json:"token_type"`
	User         *UserInfo   `json:"user"`
}

type UserInfo struct {
	ID                 int64    `json:"id"`
	Username           string   `json:"username"`
	FullName           string   `json:"full_name"`
	Phone              string   `json:"phone"`
	Roles              []string `json:"roles"`
	Permissions        []string `json:"permissions"`
	MustChangePassword bool     `json:"must_change_password"`
}

type userRow struct {
	ID       int64  `db:"id"`
	Username string `db:"username"`
	FullName string `db:"full_name"`
	Phone    string `db:"phone"`
	Hash     string `db:"password_hash"`
	Legacy   string `db:"legacy_hash"`
	Must     bool   `db:"must_change_password"`
	Active   bool   `db:"is_active"`
}

const userCols = `id, username, full_name, phone, password_hash, legacy_hash, must_change_password, is_active`

func (s *Service) roles(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT role_code FROM user_roles WHERE user_id=$1 ORDER BY role_code`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Service) issue(ctx context.Context, u userRow, client, ua, ip string) (*Tokens, error) {
	roles, err := s.roles(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	c := claims{Name: u.FullName, User: u.Username, Roles: roles, RegisteredClaims: jwt.RegisteredClaims{
		Subject: fmt.Sprint(u.ID), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(s.AccessTTL)),
	}}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.secret)
	if err != nil {
		return nil, err
	}
	raw := randToken(32)
	if _, err = s.DB.Exec(ctx, `INSERT INTO refresh_tokens(user_id,token_hash,client,user_agent,ip,expires_at)
		VALUES($1,$2,$3,$4,$5,$6)`, u.ID, hashToken(raw), client, trunc(ua, 200), ip, now.Add(s.RefreshTTL)); err != nil {
		return nil, err
	}
	p := s.build(u.ID, u.Username, u.FullName, roles)
	return &Tokens{AccessToken: access, RefreshToken: raw, ExpiresIn: int(s.AccessTTL.Seconds()), TokenType: "Bearer",
		User: &UserInfo{ID: u.ID, Username: u.Username, FullName: u.FullName, Phone: u.Phone, Roles: roles,
			Permissions: p.PermList(), MustChangePassword: u.Must}}, nil
}

// Login verifies a username (or phone) and password.
func (s *Service) Login(ctx context.Context, username, password, client, ua, ip string) (*Tokens, error) {
	username = strings.TrimSpace(httpx.Digits(username))
	key := ip + "|" + strings.ToLower(username)
	if s.locked(key) {
		return nil, httpx.E(429, "too_many_attempts", "تلاش‌های ناموفق زیاد بود؛ چند دقیقه بعد دوباره تلاش کنید")
	}
	u, err := db.One[userRow](ctx, s.DB, `SELECT `+userCols+` FROM users
		WHERE deleted_at IS NULL AND (lower(username)=lower($1) OR (phone<>'' AND phone=$2)) ORDER BY id LIMIT 1`,
		username, httpx.Phone(username))
	bad := httpx.E(401, "invalid_credentials", "نام کاربری یا گذرواژه نادرست است")
	if err != nil {
		if err == db.ErrNotFound {
			s.fail(key)
			return nil, bad
		}
		return nil, err
	}
	ok, upgrade := VerifyPassword(u.Hash, u.Legacy, password)
	if !ok || !u.Active {
		s.fail(key)
		return nil, bad
	}
	s.throttle.Delete(key)
	if upgrade {
		if h, err := HashPassword(password); err == nil {
			_, _ = s.DB.Exec(ctx, `UPDATE users SET password_hash=$2, legacy_hash='' WHERE id=$1`, u.ID, h)
		}
	}
	_, _ = s.DB.Exec(ctx, `UPDATE users SET last_login_at=NOW() WHERE id=$1`, u.ID)
	return s.issue(ctx, u, client, ua, ip)
}

// Refresh rotates a refresh token. Presenting one that was already used means
// it leaked, so every session of that user is revoked.
func (s *Service) Refresh(ctx context.Context, raw, client, ua, ip string) (*Tokens, error) {
	type rt struct {
		ID      int64      `db:"id"`
		UserID  int64      `db:"user_id"`
		Revoked *time.Time `db:"revoked_at"`
		Expires time.Time  `db:"expires_at"`
	}
	t, err := db.One[rt](ctx, s.DB, `SELECT id,user_id,revoked_at,expires_at FROM refresh_tokens WHERE token_hash=$1`, hashToken(raw))
	bad := httpx.E(401, "invalid_refresh", "نشست منقضی شده است؛ دوباره وارد شوید")
	if err != nil {
		return nil, bad
	}
	if t.Revoked != nil {
		_, _ = s.DB.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, t.UserID)
		return nil, bad
	}
	if time.Now().After(t.Expires) {
		return nil, bad
	}
	u, err := db.One[userRow](ctx, s.DB, `SELECT `+userCols+` FROM users WHERE id=$1 AND deleted_at IS NULL`, t.UserID)
	if err != nil || !u.Active {
		return nil, bad
	}
	_, _ = s.DB.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=NOW() WHERE id=$1`, t.ID)
	return s.issue(ctx, u, client, ua, ip)
}

func (s *Service) Logout(ctx context.Context, raw string) {
	_, _ = s.DB.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=NOW() WHERE token_hash=$1 AND revoked_at IS NULL`, hashToken(raw))
}

func (s *Service) RevokeAll(ctx context.Context, uid int64) {
	_, _ = s.DB.Exec(ctx, `UPDATE refresh_tokens SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, uid)
}

// IssueForUser signs a user in without a password (after OTP verification).
func (s *Service) IssueForUser(ctx context.Context, uid int64, client, ua, ip string) (*Tokens, error) {
	u, err := db.One[userRow](ctx, s.DB, `SELECT `+userCols+` FROM users WHERE id=$1 AND deleted_at IS NULL AND is_active`, uid)
	if err != nil {
		return nil, httpx.E(401, "invalid_credentials", "حساب فعال نیست")
	}
	_, _ = s.DB.Exec(ctx, `UPDATE users SET last_login_at=NOW() WHERE id=$1`, uid)
	return s.issue(ctx, u, client, ua, ip)
}

// ───────── middleware ─────────

// Authenticate attaches a Principal when a valid bearer token is present; it
// never rejects, so public routes can share the stack. Require* reject.
func (s *Service) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			var c claims
			tok, err := jwt.ParseWithClaims(strings.TrimPrefix(h, "Bearer "), &c, func(t *jwt.Token) (any, error) {
				return s.secret, nil
			}, jwt.WithValidMethods([]string{"HS256"}))
			if err == nil && tok.Valid {
				var uid int64
				fmt.Sscan(c.Subject, &uid)
				// Roles come from the token, permissions from the live cache: a
				// permission edit applies at once, a role change on next refresh.
				ctx := context.WithValue(r.Context(), ctxKey{}, s.build(uid, c.User, c.Name, c.Roles))
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if From(r.Context()) == nil {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Require admits a principal holding ANY of the listed permissions.
func Require(perms ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := From(r.Context())
			if p == nil {
				httpx.Fail(w, r, httpx.ErrUnauthorized)
				return
			}
			for _, perm := range perms {
				if p.Can(perm) {
					next.ServeHTTP(w, r)
					return
				}
			}
			httpx.Fail(w, r, httpx.ErrForbidden)
		})
	}
}

// ───────── OTP ─────────

// RequestOTP creates a 5-digit code for a phone. The caller sends it by SMS.
func (s *Service) RequestOTP(ctx context.Context, phone string) (string, error) {
	recent, _ := db.Scalar[int](ctx, s.DB, `SELECT COUNT(*) FROM otp_codes WHERE phone=$1 AND created_at > NOW() - INTERVAL '10 minutes'`, phone)
	if recent >= 3 {
		return "", httpx.E(429, "otp_rate", "تعداد درخواست کد زیاد است؛ چند دقیقه صبر کنید")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(90000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%05d", n.Int64()+10000)
	_, err = s.DB.Exec(ctx, `INSERT INTO otp_codes(phone,code_hash,expires_at) VALUES($1,$2,NOW()+INTERVAL '3 minutes')`, phone, hashToken(phone+":"+code))
	return code, err
}

// VerifyOTP checks the newest unused code; at most 5 wrong guesses per code.
func (s *Service) VerifyOTP(ctx context.Context, phone, code string) error {
	type o struct {
		ID       int64     `db:"id"`
		Hash     string    `db:"code_hash"`
		Attempts int       `db:"attempts"`
		Expires  time.Time `db:"expires_at"`
	}
	bad := httpx.E(401, "invalid_otp", "کد نادرست یا منقضی شده است")
	v, err := db.One[o](ctx, s.DB, `SELECT id,code_hash,attempts,expires_at FROM otp_codes
		WHERE phone=$1 AND used_at IS NULL ORDER BY id DESC LIMIT 1`, phone)
	if err != nil || time.Now().After(v.Expires) || v.Attempts >= 5 {
		return bad
	}
	if hashToken(phone+":"+httpx.Digits(strings.TrimSpace(code))) != v.Hash {
		_, _ = s.DB.Exec(ctx, `UPDATE otp_codes SET attempts=attempts+1 WHERE id=$1`, v.ID)
		return bad
	}
	_, _ = s.DB.Exec(ctx, `UPDATE otp_codes SET used_at=NOW() WHERE id=$1`, v.ID)
	return nil
}

// ───────── helpers ─────────

type attempt struct {
	n    int
	last time.Time
}

func (s *Service) fail(key string) {
	v, _ := s.throttle.LoadOrStore(key, &attempt{})
	a := v.(*attempt)
	if time.Since(a.last) > 5*time.Minute {
		a.n = 0
	}
	a.n++
	a.last = time.Now()
}
func (s *Service) locked(key string) bool {
	v, ok := s.throttle.Load(key)
	if !ok {
		return false
	}
	a := v.(*attempt)
	return a.n >= 6 && time.Since(a.last) < 5*time.Minute
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// RandToken is exported for modules that need unguessable ids (survey links, device keys).
func RandToken(n int) string { return randToken(n) }

func hashToken(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
func HashToken(s string) string { return hashToken(s) }

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// ClientIP honours X-Forwarded-For only from the trusted reverse proxy
// (nginx runs in front in every deployment this repo ships).
func ClientIP(r *http.Request) string {
	if x := r.Header.Get("X-Real-IP"); x != "" {
		return x
	}
	if x := r.Header.Get("X-Forwarded-For"); x != "" {
		return strings.TrimSpace(strings.Split(x, ",")[0])
	}
	h := r.RemoteAddr
	if i := strings.LastIndex(h, ":"); i > 0 {
		return h[:i]
	}
	return h
}
