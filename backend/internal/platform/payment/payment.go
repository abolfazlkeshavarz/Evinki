// Package payment hides the payment gateway behind two calls: Start and Verify.
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type StartResult struct {
	Authority string
	URL       string // where to send the customer
}

type VerifyResult struct {
	OK     bool
	RefID  string
	CardPan string
	Raw    map[string]any
}

type Gateway interface {
	Name() string
	// Start asks the gateway for a payment session. amount is in Toman.
	Start(ctx context.Context, amount int64, callbackURL, description, phone string) (*StartResult, error)
	Verify(ctx context.Context, authority string, amount int64) (*VerifyResult, error)
}

// ───────── Zarinpal v4 ─────────

type Zarinpal struct {
	Merchant string
	Sandbox  bool
}

func (z Zarinpal) Name() string { return "zarinpal" }

func (z Zarinpal) base() string {
	if z.Sandbox {
		return "https://sandbox.zarinpal.com/pg"
	}
	return "https://payment.zarinpal.com/pg"
}

func (z Zarinpal) call(ctx context.Context, path string, in any, out any) error {
	b, _ := json.Marshal(in)
	req, _ := http.NewRequestWithContext(ctx, "POST", z.base()+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return json.Unmarshal(body, out)
}

func (z Zarinpal) Start(ctx context.Context, amount int64, cb, desc, phone string) (*StartResult, error) {
	var out struct {
		Data struct {
			Code      int    `json:"code"`
			Authority string `json:"authority"`
		} `json:"data"`
		Errors any `json:"errors"`
	}
	err := z.call(ctx, "/v4/payment/request.json", map[string]any{
		"merchant_id": z.Merchant, "amount": amount, "currency": "IRT",
		"callback_url": cb, "description": desc, "metadata": map[string]string{"mobile": phone},
	}, &out)
	if err != nil {
		return nil, err
	}
	if out.Data.Code != 100 || out.Data.Authority == "" {
		return nil, fmt.Errorf("gateway refused the request: %v", out.Errors)
	}
	return &StartResult{Authority: out.Data.Authority, URL: z.base() + "/StartPay/" + out.Data.Authority}, nil
}

func (z Zarinpal) Verify(ctx context.Context, authority string, amount int64) (*VerifyResult, error) {
	var out struct {
		Data struct {
			Code    int    `json:"code"`
			RefID   int64  `json:"ref_id"`
			CardPan string `json:"card_pan"`
		} `json:"data"`
	}
	err := z.call(ctx, "/v4/payment/verify.json", map[string]any{
		"merchant_id": z.Merchant, "amount": amount, "authority": authority,
	}, &out)
	if err != nil {
		return nil, err
	}
	// 100 = verified now, 101 = already verified earlier (a repeated callback).
	ok := out.Data.Code == 100 || out.Data.Code == 101
	return &VerifyResult{OK: ok, RefID: fmt.Sprint(out.Data.RefID), CardPan: out.Data.CardPan,
		Raw: map[string]any{"code": out.Data.Code}}, nil
}

// ───────── Mock (development only; refused in production by config) ─────────

type Mock struct{ PublicURL string }

func (Mock) Name() string { return "mock" }
func (m Mock) Start(_ context.Context, _ int64, cb, _, _ string) (*StartResult, error) {
	auth := fmt.Sprintf("MOCK%d", time.Now().UnixNano())
	sep := "?"
	if bytes.ContainsRune([]byte(cb), '?') {
		sep = "&"
	}
	// The "gateway page" is the callback itself with Status=OK, so the whole
	// flow can be exercised without any external service.
	return &StartResult{Authority: auth, URL: cb + sep + "Authority=" + auth + "&Status=OK"}, nil
}
func (Mock) Verify(_ context.Context, authority string, _ int64) (*VerifyResult, error) {
	return &VerifyResult{OK: true, RefID: "MOCK-" + authority[4:], CardPan: "6037****1234"}, nil
}
