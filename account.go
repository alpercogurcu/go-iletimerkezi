package iletimerkezi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// BalanceInfo, hesap bakiyesidir. Hesap TL kredisiyle çalışıyorsa Amount, SMS
// kredisiyle çalışıyorsa SMS anlamlıdır.
type BalanceInfo struct {
	// Amount, TL bakiyesidir.
	Amount float64
	// SMS, kalan SMS kredisidir.
	SMS int
}

// Balance, hesap bakiyesini döner (ücretsiz uç nokta).
func (c *Client) Balance(ctx context.Context) (*BalanceInfo, error) {
	env, err := c.call(ctx, "get-balance", map[string]any{"authentication": c.auth()})
	if err != nil {
		return nil, err
	}
	var b struct {
		Amount flexString `json:"amount"`
		SMS    flexString `json:"sms"`
	}
	if len(env.Response.Balance) == 0 || json.Unmarshal(env.Response.Balance, &b) != nil {
		return nil, fmt.Errorf("iletimerkezi: bakiye yanıtı çözümlenemedi")
	}
	out := &BalanceInfo{}
	if s := strings.ReplaceAll(string(b.Amount), ",", "."); s != "" {
		if out.Amount, err = strconv.ParseFloat(s, 64); err != nil {
			return nil, fmt.Errorf("iletimerkezi: bakiye tutarı okunamadı")
		}
	}
	if s := string(b.SMS); s != "" {
		if out.SMS, err = strconv.Atoi(s); err != nil {
			return nil, fmt.Errorf("iletimerkezi: SMS kredisi okunamadı")
		}
	}
	return out, nil
}

// Senders, hesapta tanımlı ve onaylı SMS başlıklarını döner (ücretsiz uç nokta).
func (c *Client) Senders(ctx context.Context) ([]string, error) {
	env, err := c.call(ctx, "get-sender", map[string]any{"authentication": c.auth()})
	if err != nil {
		return nil, err
	}
	var s struct {
		Sender oneOrMany[string] `json:"sender"`
	}
	if len(env.Response.Senders) == 0 {
		return []string{}, nil
	}
	if err := json.Unmarshal(env.Response.Senders, &s); err != nil {
		return nil, fmt.Errorf("iletimerkezi: başlık listesi çözümlenemedi: %w", err)
	}
	if s.Sender == nil {
		return []string{}, nil
	}
	return []string(s.Sender), nil
}

// Cancel, ileri tarihli (gönderimi henüz başlamamış) bir siparişi iptal eder.
// Gönderimi başlamış sipariş için ErrOrderNotFound (455) döner.
func (c *Client) Cancel(ctx context.Context, orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return fmt.Errorf("%w: sipariş id'si boş", ErrInvalidRequest)
	}
	_, err := c.call(ctx, "cancel-order", map[string]any{
		"authentication": c.auth(),
		"order":          map[string]any{"id": orderID},
	})
	return err
}
