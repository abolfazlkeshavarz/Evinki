// Package iam: sign-in (password, SMS code), the current user, staff user
// management, roles and the audit trail.
package iam

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/evinki/platform/internal/app"
	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
	"github.com/evinki/platform/internal/platform/sms"
)

func Mount(r chi.Router, d *app.Deps) {
	m := &mod{d}

	// public
	r.Post("/auth/login", httpx.H(m.login))
	r.Post("/auth/otp/request", httpx.H(m.otpRequest))
	r.Post("/auth/otp/verify", httpx.H(m.otpVerify))
	r.Post("/auth/refresh", httpx.H(m.refresh))
	r.Post("/auth/logout", httpx.H(m.logout))

	// signed in
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/auth/me", httpx.H(m.me))
		r.Post("/auth/change-password", httpx.H(m.changePassword))
		r.Get("/notifications", httpx.H(m.notifications))
		r.Post("/notifications/{id}/read", httpx.H(m.readNotification))
		r.Post("/notifications/read-all", httpx.H(m.readAll))
		r.Post("/push-tokens", httpx.H(m.pushToken))
	})

	// administration
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PUsers))
		r.Get("/admin/users", httpx.H(m.listUsers))
		r.Post("/admin/users", httpx.H(m.createUser))
		r.Patch("/admin/users/{id}", httpx.H(m.updateUser))
		r.Post("/admin/users/{id}/reset-password", httpx.H(m.resetPassword))
		r.Put("/admin/users/{id}/roles", httpx.H(m.setRoles))
		r.Delete("/admin/users/{id}", httpx.H(m.deleteUser))
	})
	r.Group(func(r chi.Router) {
		r.Use(auth.Require(auth.PUsers, auth.PRoles))
		r.Get("/admin/roles", httpx.H(m.listRoles))
		r.Get("/admin/permissions", httpx.H(m.listPermissions))
	})
	r.With(auth.Require(auth.PRoles)).Put("/admin/roles/{code}/permissions", httpx.H(m.setRolePerms))
	r.With(auth.Require(auth.PAudit)).Get("/admin/audit", httpx.H(m.audit))
}

type mod struct{ *app.Deps }

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Client   string `json:"client"`
}

func clientName(c string) string {
	switch c {
	case "customer-app", "technician-app", "manager-app", "web":
		return c
	}
	return "web"
}

func (m *mod) login(w http.ResponseWriter, r *http.Request) error {
	var in loginReq
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	t, err := m.Auth.Login(r.Context(), in.Username, in.Password, clientName(in.Client), r.UserAgent(), auth.ClientIP(r))
	if err != nil {
		return err
	}
	httpx.OK(w, t)
	return nil
}

// otpRequest: the customer enters a phone; a code is sent. A customer account
// is created on first verified login, so shopping never needs a registration form.
func (m *mod) otpRequest(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Phone string `json:"phone"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	phone := httpx.Phone(in.Phone)
	if !httpx.ValidMobile(phone) {
		return httpx.BadRequest("شماره موبایل نامعتبر است")
	}
	code, err := m.Auth.RequestOTP(r.Context(), phone)
	if err != nil {
		return err
	}
	if err := sms.Enqueue(r.Context(), m.DB, phone, "otp", code); err != nil {
		return err
	}
	resp := map[string]any{"ok": true, "expires_in": 180}
	if !m.Cfg.IsProd() && !m.Cfg.SMSEnabled {
		resp["dev_code"] = code // lets developers sign in without an SMS provider
	}
	httpx.OK(w, resp)
	return nil
}

func (m *mod) otpVerify(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Phone  string `json:"phone"`
		Code   string `json:"code"`
		Name   string `json:"name"`
		Client string `json:"client"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	phone := httpx.Phone(in.Phone)
	if err := m.Auth.VerifyOTP(r.Context(), phone, in.Code); err != nil {
		return err
	}
	ctx := r.Context()
	uid, err := db.Scalar[int64](ctx, m.DB, `SELECT id FROM users WHERE phone=$1 AND deleted_at IS NULL ORDER BY id LIMIT 1`, phone)
	if err == db.ErrNotFound {
		err = m.DB.Tx(ctx, func(tx pgxTx) error {
			var e error
			uid, e = db.Scalar[int64](ctx, tx, `INSERT INTO users(username,full_name,phone) VALUES($1,$2,$1) RETURNING id`, phone, strings.TrimSpace(in.Name))
			if e != nil {
				return e
			}
			_, e = tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_code) VALUES($1,'customer')`, uid)
			return e
		})
	}
	if err != nil {
		return err
	}
	// A customer who already has guarantees on file (imported or registered by
	// phone) is linked to this account automatically.
	_, _ = m.DB.Exec(ctx, `UPDATE customers SET user_id=$1 WHERE phone=$2 AND user_id IS NULL`, uid, phone)
	t, err := m.Auth.IssueForUser(ctx, uid, clientName(in.Client), r.UserAgent(), auth.ClientIP(r))
	if err != nil {
		return err
	}
	httpx.OK(w, t)
	return nil
}

func (m *mod) refresh(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		RefreshToken string `json:"refresh_token"`
		Client       string `json:"client"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	t, err := m.Auth.Refresh(r.Context(), in.RefreshToken, clientName(in.Client), r.UserAgent(), auth.ClientIP(r))
	if err != nil {
		return err
	}
	httpx.OK(w, t)
	return nil
}

func (m *mod) logout(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = httpx.Decode(r, &in)
	m.Auth.Logout(r.Context(), in.RefreshToken)
	httpx.NoContent(w)
	return nil
}

func (m *mod) me(w http.ResponseWriter, r *http.Request) error {
	p := auth.From(r.Context())
	type row struct {
		Phone string `db:"phone"`
		Must  bool   `db:"must_change_password"`
		Email string `db:"email"`
	}
	u, err := db.One[row](r.Context(), m.DB, `SELECT phone, must_change_password, email FROM users WHERE id=$1`, p.UserID)
	if err != nil {
		return err
	}
	httpx.OK(w, auth.UserInfo{ID: p.UserID, Username: p.Username, FullName: p.Name, Phone: u.Phone, Roles: p.Roles,
		Permissions: p.PermList(), MustChangePassword: u.Must})
	return nil
}

func (m *mod) changePassword(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := checkPassword(in.New); err != nil {
		return err
	}
	p := auth.From(r.Context())
	type row struct {
		Hash   string `db:"password_hash"`
		Legacy string `db:"legacy_hash"`
	}
	u, err := db.One[row](r.Context(), m.DB, `SELECT password_hash, legacy_hash FROM users WHERE id=$1`, p.UserID)
	if err != nil {
		return err
	}
	// Accounts created by phone code have no password yet; they may set one freely.
	if u.Hash != "" || u.Legacy != "" {
		if ok, _ := auth.VerifyPassword(u.Hash, u.Legacy, in.Current); !ok {
			return httpx.E(400, "wrong_password", "گذرواژه فعلی نادرست است")
		}
	}
	h, err := auth.HashPassword(in.New)
	if err != nil {
		return err
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE users SET password_hash=$2, legacy_hash='', must_change_password=FALSE, updated_at=NOW() WHERE id=$1`, p.UserID, h); err != nil {
		return err
	}
	m.Auth.RevokeAll(r.Context(), p.UserID)
	httpx.NoContent(w)
	return nil
}

func checkPassword(pw string) error {
	if len(pw) < 8 {
		return httpx.BadRequest("گذرواژه باید حداقل ۸ نویسه باشد")
	}
	return nil
}

// ───────── notifications ─────────

type notif struct {
	ID        int64      `db:"id" json:"id"`
	Kind      string     `db:"kind" json:"kind"`
	Title     string     `db:"title" json:"title"`
	Body      string     `db:"body" json:"body"`
	Link      string     `db:"link" json:"link"`
	ReadAt    *time.Time `db:"read_at" json:"read_at"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
}

func (m *mod) notifications(w http.ResponseWriter, r *http.Request) error {
	p := auth.From(r.Context())
	items, err := db.All[notif](r.Context(), m.DB, `SELECT id,kind,title,body,link,read_at,created_at FROM notifications
		WHERE user_id=$1 ORDER BY id DESC LIMIT 50`, p.UserID)
	if err != nil {
		return err
	}
	unread, _ := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL`, p.UserID)
	httpx.OK(w, map[string]any{"items": items, "unread": unread})
	return nil
}

func (m *mod) readNotification(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE notifications SET read_at=NOW() WHERE id=$1 AND user_id=$2`, id, auth.From(r.Context()).UserID)
	httpx.NoContent(w)
	return err
}

func (m *mod) readAll(w http.ResponseWriter, r *http.Request) error {
	_, err := m.DB.Exec(r.Context(), `UPDATE notifications SET read_at=NOW() WHERE user_id=$1 AND read_at IS NULL`, auth.From(r.Context()).UserID)
	httpx.NoContent(w)
	return err
}

func (m *mod) pushToken(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Platform string `json:"platform"`
		Token    string `json:"token"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	_, err := m.DB.Exec(r.Context(), `INSERT INTO push_tokens(user_id,platform,token) VALUES($1,$2,$3)
		ON CONFLICT (token) DO UPDATE SET user_id=EXCLUDED.user_id, platform=EXCLUDED.platform`,
		auth.From(r.Context()).UserID, in.Platform, in.Token)
	httpx.NoContent(w)
	return err
}

// ───────── users ─────────

type userOut struct {
	ID          int64      `db:"id" json:"id"`
	Username    string     `db:"username" json:"username"`
	FullName    string     `db:"full_name" json:"full_name"`
	Phone       string     `db:"phone" json:"phone"`
	Email       string     `db:"email" json:"email"`
	IsActive    bool       `db:"is_active" json:"is_active"`
	Roles       []string   `db:"roles" json:"roles"`
	LastLoginAt *time.Time `db:"last_login_at" json:"last_login_at"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
}

const userSel = `SELECT u.id,u.username,u.full_name,u.phone,u.email,u.is_active,u.last_login_at,u.created_at,
	COALESCE((SELECT array_agg(role_code ORDER BY role_code) FROM user_roles WHERE user_id=u.id),'{}') AS roles
	FROM users u WHERE u.deleted_at IS NULL`

func (m *mod) listUsers(w http.ResponseWriter, r *http.Request) error {
	q, role := httpx.Q(r, "q"), httpx.Q(r, "role")
	pg := httpx.Paging(r, 100)
	where, args := ` AND ($1='' OR u.username ILIKE '%'||$1||'%' OR u.full_name ILIKE '%'||$1||'%' OR u.phone LIKE '%'||$1||'%')
		AND ($2='' OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id=u.id AND ur.role_code=$2))`, []any{q, role}
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM users u WHERE u.deleted_at IS NULL`+where, args...)
	if err != nil {
		return err
	}
	items, err := db.All[userOut](r.Context(), m.DB, userSel+where+` ORDER BY u.id DESC LIMIT $3 OFFSET $4`, append(args, pg.Limit, pg.Offset)...)
	if err != nil {
		return err
	}
	httpx.OK(w, httpx.List[userOut]{Items: items, Total: total, Page: pg.Page, Limit: pg.Limit})
	return nil
}

type userIn struct {
	Username string   `json:"username"`
	FullName string   `json:"full_name"`
	Phone    string   `json:"phone"`
	Email    string   `json:"email"`
	Password string   `json:"password"`
	Roles    []string `json:"roles"`
}

func (m *mod) createUser(w http.ResponseWriter, r *http.Request) error {
	var in userIn
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || len(in.Roles) == 0 {
		return httpx.BadRequest("نام کاربری و حداقل یک نقش لازم است")
	}
	if err := checkPassword(in.Password); err != nil {
		return err
	}
	if err := m.guardRoles(r, in.Roles); err != nil {
		return err
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	var id int64
	err = m.DB.Tx(r.Context(), func(tx pgxTx) error {
		var e error
		id, e = db.Scalar[int64](r.Context(), tx, `INSERT INTO users(username,full_name,phone,email,password_hash,must_change_password)
			VALUES($1,$2,$3,$4,$5,TRUE) RETURNING id`, in.Username, in.FullName, httpx.Phone(in.Phone), in.Email, h)
		if e != nil {
			return e
		}
		return setRolesTx(r, tx, id, in.Roles)
	})
	if err != nil {
		return err
	}
	m.Audit.Log(r, "user.create", "user", id, in.Username)
	u, _ := db.One[userOut](r.Context(), m.DB, userSel+` AND u.id=$1`, id)
	httpx.Created(w, u)
	return nil
}

// guardRoles: only a super admin may hand out super_admin or admin.
func (m *mod) guardRoles(r *http.Request, roles []string) error {
	p := auth.From(r.Context())
	for _, role := range roles {
		if (role == "super_admin" || role == "admin") && !p.HasRole("super_admin") {
			return httpx.E(403, "forbidden", "فقط مدیر ارشد می‌تواند این نقش را بدهد")
		}
	}
	return nil
}

func (m *mod) updateUser(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		FullName *string `json:"full_name"`
		Phone    *string `json:"phone"`
		Email    *string `json:"email"`
		IsActive *bool   `json:"is_active"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := m.guardTarget(r, id); err != nil {
		return err
	}
	var phone *string
	if in.Phone != nil {
		v := httpx.Phone(*in.Phone)
		phone = &v
	}
	_, err = m.DB.Exec(r.Context(), `UPDATE users SET full_name=COALESCE($2,full_name), phone=COALESCE($3,phone),
		email=COALESCE($4,email), is_active=COALESCE($5,is_active), updated_at=NOW() WHERE id=$1 AND deleted_at IS NULL`,
		id, in.FullName, phone, in.Email, in.IsActive)
	if err != nil {
		return err
	}
	if in.IsActive != nil && !*in.IsActive {
		m.Auth.RevokeAll(r.Context(), id)
	}
	m.Audit.Log(r, "user.update", "user", id, "")
	u, err := db.One[userOut](r.Context(), m.DB, userSel+` AND u.id=$1`, id)
	if err != nil {
		return err
	}
	httpx.OK(w, u)
	return nil
}

// guardTarget keeps non-super admins away from privileged accounts.
func (m *mod) guardTarget(r *http.Request, id int64) error {
	if auth.From(r.Context()).HasRole("super_admin") {
		return nil
	}
	n, _ := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM user_roles WHERE user_id=$1 AND role_code IN ('super_admin','admin')`, id)
	if n > 0 {
		return httpx.E(403, "forbidden", "حساب مدیران فقط توسط مدیر ارشد قابل تغییر است")
	}
	return nil
}

func (m *mod) resetPassword(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Password   string `json:"password"`
		MustChange bool   `json:"must_change"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if err := checkPassword(in.Password); err != nil {
		return err
	}
	if err := m.guardTarget(r, id); err != nil {
		return err
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		return err
	}
	if _, err = m.DB.Exec(r.Context(), `UPDATE users SET password_hash=$2, legacy_hash='', must_change_password=$3, updated_at=NOW() WHERE id=$1`, id, h, in.MustChange); err != nil {
		return err
	}
	m.Auth.RevokeAll(r.Context(), id)
	m.Audit.Log(r, "user.reset_password", "user", id, "")
	httpx.NoContent(w)
	return nil
}

func (m *mod) setRoles(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Roles []string `json:"roles"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if len(in.Roles) == 0 {
		return httpx.BadRequest("حداقل یک نقش لازم است")
	}
	if err := m.guardRoles(r, in.Roles); err != nil {
		return err
	}
	if err := m.guardTarget(r, id); err != nil {
		return err
	}
	if err := m.DB.Tx(r.Context(), func(tx pgxTx) error { return setRolesTx(r, tx, id, in.Roles) }); err != nil {
		return err
	}
	m.Auth.RevokeAll(r.Context(), id) // new roles apply at next sign-in
	m.Audit.Log(r, "user.roles", "user", id, strings.Join(in.Roles, ","))
	httpx.NoContent(w)
	return nil
}

func (m *mod) deleteUser(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.ID(r, "id")
	if err != nil {
		return err
	}
	if id == auth.From(r.Context()).UserID {
		return httpx.BadRequest("حساب خودتان را نمی‌توانید حذف کنید")
	}
	if err := m.guardTarget(r, id); err != nil {
		return err
	}
	// Soft delete + free the username for reuse; history keeps pointing at the row.
	_, err = m.DB.Exec(r.Context(), `UPDATE users SET deleted_at=NOW(), is_active=FALSE WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	m.Auth.RevokeAll(r.Context(), id)
	m.Audit.Log(r, "user.delete", "user", id, "")
	httpx.NoContent(w)
	return nil
}

// ───────── roles ─────────

func (m *mod) listRoles(w http.ResponseWriter, r *http.Request) error {
	type role struct {
		Code        string   `db:"code" json:"code"`
		TitleFA     string   `db:"title_fa" json:"title_fa"`
		TitleEN     string   `db:"title_en" json:"title_en"`
		Permissions []string `db:"permissions" json:"permissions"`
		Users       int      `db:"users" json:"users"`
	}
	items, err := db.All[role](r.Context(), m.DB, `SELECT r.code,r.title_fa,r.title_en,
		COALESCE((SELECT array_agg(permission ORDER BY permission) FROM role_permissions WHERE role_code=r.code),'{}') AS permissions,
		(SELECT COUNT(*) FROM user_roles ur JOIN users u ON u.id=ur.user_id AND u.deleted_at IS NULL WHERE ur.role_code=r.code)::int AS users
		FROM roles r ORDER BY r.code`)
	if err != nil {
		return err
	}
	httpx.OK(w, items)
	return nil
}

func (m *mod) listPermissions(w http.ResponseWriter, r *http.Request) error {
	type perm struct {
		Key   string `json:"key"`
		Title string `json:"title"`
	}
	out := []perm{}
	for _, p := range auth.AllPermissions {
		out = append(out, perm{p, auth.PermissionTitles[p]})
	}
	httpx.OK(w, out)
	return nil
}

func (m *mod) setRolePerms(w http.ResponseWriter, r *http.Request) error {
	code := chi.URLParam(r, "code")
	if code == "super_admin" {
		return httpx.BadRequest("دسترسی‌های مدیر ارشد قابل تغییر نیست")
	}
	var in struct {
		Permissions []string `json:"permissions"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	valid := map[string]bool{}
	for _, p := range auth.AllPermissions {
		valid[p] = true
	}
	err := m.DB.Tx(r.Context(), func(tx pgxTx) error {
		if _, e := tx.Exec(r.Context(), `DELETE FROM role_permissions WHERE role_code=$1`, code); e != nil {
			return e
		}
		for _, p := range in.Permissions {
			if !valid[p] {
				return httpx.BadRequest("دسترسی ناشناخته: " + p)
			}
			if _, e := tx.Exec(r.Context(), `INSERT INTO role_permissions(role_code,permission) VALUES($1,$2)`, code, p); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := m.Auth.Reload(r.Context()); err != nil {
		return err
	}
	m.Audit.Log(r, "role.permissions", "role", code, strings.Join(in.Permissions, ","))
	httpx.NoContent(w)
	return nil
}

// ───────── audit ─────────

func (m *mod) audit(w http.ResponseWriter, r *http.Request) error {
	type row struct {
		ID        int64     `db:"id" json:"id"`
		At        time.Time `db:"at" json:"at"`
		ActorName string    `db:"actor_name" json:"actor_name"`
		ActorRole string    `db:"actor_role" json:"actor_role"`
		Action    string    `db:"action" json:"action"`
		Entity    string    `db:"entity" json:"entity"`
		EntityID  string    `db:"entity_id" json:"entity_id"`
		Detail    string    `db:"detail" json:"detail"`
		IP        string    `db:"ip" json:"ip"`
	}
	pg := httpx.Paging(r, 200)
	q, actor := httpx.Q(r, "q"), httpx.Q(r, "actor")
	where := ` WHERE ($1='' OR action ILIKE '%'||$1||'%' OR entity ILIKE '%'||$1||'%' OR detail ILIKE '%'||$1||'%')
		AND ($2='' OR actor_name=$2)`
	total, err := db.Scalar[int](r.Context(), m.DB, `SELECT COUNT(*) FROM audit_log`+where, q, actor)
	if err != nil {
		return err
	}
	items, err := db.All[row](r.Context(), m.DB, `SELECT id,at,actor_name,actor_role,action,entity,entity_id,detail,ip FROM audit_log`+where+
		` ORDER BY id DESC LIMIT $3 OFFSET $4`, q, actor, pg.Limit, pg.Offset)
	if err != nil {
		return err
	}
	httpx.OK(w, httpx.List[row]{Items: items, Total: total, Page: pg.Page, Limit: pg.Limit})
	return nil
}
