package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/xuri/excelize/v2"

	"github.com/evinki/platform/internal/platform/auth"
	"github.com/evinki/platform/internal/platform/db"
	"github.com/evinki/platform/internal/platform/httpx"
)

// Bulk onboarding of technicians from a spreadsheet (.xlsx or .csv), as the old guarantee system did.
// Rows are processed on their own: a bad row never stops the good ones.

const maxImportRows = 2000

var importColumns = map[string]string{
	"fullname": "full_name", "full name": "full_name", "name": "full_name", "نام": "full_name", "نام کامل": "full_name", "نام و نام خانوادگی": "full_name",
	"username": "username", "user name": "username", "user": "username", "نام کاربری": "username",
	"password": "password", "pass": "password", "pwd": "password", "رمز عبور": "password", "رمز": "password", "کلمه عبور": "password", "گذرواژه": "password",
	"phone": "phone", "phone number": "phone", "mobile": "phone", "tel": "phone", "موبایل": "phone", "تلفن": "phone", "شماره تماس": "phone", "شماره موبایل": "phone",
	"national id": "national_id", "nationalid": "national_id", "nid": "national_id", "کد ملی": "national_id", "شماره ملی": "national_id",
	"province": "province", "استان": "province", "city": "city", "شهر": "city",
	"address": "address", "addr": "address", "آدرس": "address", "نشانی": "address",
}

func normHeader(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("_", " ", "-", " ", "ي", "ی", "ك", "ک").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

type importRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

func readTable(name string, data []byte) ([][]string, error) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".xlsx"):
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, fmt.Errorf("فایل هیچ برگه‌ای ندارد")
		}
		return f.GetRows(sheets[0])
	case strings.HasSuffix(lower, ".csv"), strings.HasSuffix(lower, ".txt"):
		data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
		cr := csv.NewReader(bytes.NewReader(data))
		cr.FieldsPerRecord, cr.TrimLeadingSpace = -1, true
		var out [][]string
		for {
			rec, err := cr.Read()
			if err == io.EOF {
				return out, nil
			}
			if err != nil {
				return nil, err
			}
			out = append(out, rec)
		}
	}
	return nil, fmt.Errorf("فقط فایل .xlsx یا .csv پذیرفته می‌شود")
}

func (m *mod) importTechnicians(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		return httpx.BadRequest("فایل دریافت نشد یا بزرگ‌تر از ۵ مگابایت است")
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return httpx.BadRequest("فایل را انتخاب کنید")
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	records, err := readTable(hdr.Filename, data)
	if err != nil {
		return httpx.BadRequest("خواندن فایل ممکن نشد: " + err.Error())
	}
	var rows [][]string
	for _, rec := range records {
		if strings.TrimSpace(strings.Join(rec, "")) != "" {
			rows = append(rows, rec)
		}
	}
	if len(rows) < 2 {
		return httpx.BadRequest("فایل ردیف داده ندارد؛ ردیف عنوان را نگه دارید و هر تکنسین را در یک سطر بنویسید")
	}
	cols := make([]string, len(rows[0]))
	known := 0
	for i, h := range rows[0] {
		if f, ok := importColumns[normHeader(h)]; ok {
			cols[i] = f
			known++
		}
	}
	if known == 0 {
		return httpx.BadRequest("عنوان ستون‌ها شناخته نشد؛ دست‌کم «نام»، «نام کاربری» و «رمز عبور» لازم است (نمونه را دانلود کنید)")
	}
	if len(rows)-1 > maxImportRows {
		return httpx.BadRequest(fmt.Sprintf("حداکثر %d ردیف در هر بارگذاری", maxImportRows))
	}
	created, skipped := 0, 0
	errs := []importRowError{}
	for i, rec := range rows[1:] {
		row := map[string]string{}
		for c, f := range cols {
			if f != "" && c < len(rec) {
				row[f] = strings.TrimSpace(rec[c])
			}
		}
		id, e := m.insertTechnician(r.Context(), techIn{FullName: row["full_name"], Phone: httpx.Phone(row["phone"]), Username: row["username"], Password: row["password"],
			NationalID: row["national_id"], Province: row["province"], City: row["city"], Address: row["address"]})
		switch {
		case e == nil:
			created++
			m.Audit.Log(r, "technician.create", "technician", id, row["username"])
		case db.IsUnique(e):
			skipped++ // that username already exists
		default:
			msg := e.Error()
			if he, ok := e.(*httpx.Error); ok {
				msg = he.Message
			}
			errs = append(errs, importRowError{Row: i + 2, Message: msg})
		}
	}
	httpx.OK(w, map[string]any{"total": len(rows) - 1, "created": created, "skipped": skipped, "errors": errs})
	return nil
}

func (m *mod) importTemplate(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="technicians-template.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"نام و نام خانوادگی", "نام کاربری", "رمز عبور", "موبایل", "کد ملی", "استان", "شهر", "آدرس"})
	_ = cw.Write([]string{"علی رضایی", "ali.rezaei", "Pass12345", "09121234567", "0012345678", "تهران", "تهران", "خیابان آزادی"})
	cw.Flush()
	return nil
}

type techIn struct {
	FullName, Phone, Username, Password, NationalID, Province, City, Address string
}

// insertTechnician creates the user, gives it the technician role and an approved technician profile.
func (m *mod) insertTechnician(ctx context.Context, in techIn) (int64, error) {
	if in.Username == "" {
		in.Username = in.Phone
	}
	if strings.TrimSpace(in.FullName) == "" || in.Username == "" || len(in.Password) < 8 {
		return 0, httpx.BadRequest("نام، نام کاربری و گذرواژه حداقل ۸ نویسه لازم است")
	}
	h, err := auth.HashPassword(in.Password)
	if err != nil {
		return 0, err
	}
	var id int64
	err = m.DB.Tx(ctx, func(tx pgx.Tx) error {
		var e error
		id, e = db.Scalar[int64](ctx, tx, `INSERT INTO users(username,full_name,phone,password_hash,must_change_password) VALUES($1,$2,$3,$4,TRUE) RETURNING id`, in.Username, in.FullName, in.Phone, h)
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO user_roles(user_id,role_code) VALUES($1,'technician')`, id); e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO technicians(user_id,national_id,province,city,address,status) VALUES($1,$2,$3,$4,$5,'approved')`, id, in.NationalID, in.Province, in.City, in.Address)
		return e
	})
	return id, err
}
