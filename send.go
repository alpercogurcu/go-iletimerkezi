package iletimerkezi

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// IYSMode, İleti Yönetim Sistemi sorgulama ayarıdır.
type IYSMode string

const (
	// IYSOff: İYS sorgulaması yapılmaz (işlemsel/bilgilendirme SMS'leri; varsayılan).
	IYSOff IYSMode = "0"
	// IYSOn: Alıcılar İYS üzerinden sorgulanır. Ticari ileti gönderiyorsanız yasal olarak
	// açık olmalıdır. IYSList zorunludur.
	IYSOn IYSMode = "1"
)

// IYSList, İYS sorgulamasında kullanılacak liste türüdür.
type IYSList string

const (
	IYSBireysel IYSList = "BIREYSEL"
	IYSTacir    IYSList = "TACIR"
)

// Message, tek bir metin ve onun alıcılarıdır.
type Message struct {
	Text       string
	Recipients []string
}

// SendRequest, bir SMS siparişini tanımlar.
//
// Tek metin için Text + Recipients, farklı alıcılara farklı metinler için Messages
// kullanılır; ikisi birlikte verilemez. Alıcılar NormalizeTR ile normalleştirilir ve
// mesaj başına tekilleştirilir.
type SendRequest struct {
	// Sender boşsa Config.Sender kullanılır.
	Sender     string
	Text       string
	Recipients []string
	Messages   []Message
	// SendAt, planlı gönderim zamanıdır (nil: hemen). Europe/Istanbul saatiyle
	// "GG/AA/YYYY SS:DD" biçiminde gönderilir.
	SendAt *time.Time
	// IYS boşsa IYSOff kabul edilir.
	IYS     IYSMode
	IYSList IYSList
}

// SendResult, başarılı gönderim yanıtıdır.
type SendResult struct {
	// OrderID, siparişin kimliğidir; GetReport ve Cancel için kullanılır.
	OrderID string
}

var istanbul = func() *time.Location {
	if l, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return l
	}
	return time.FixedZone("Europe/Istanbul", 3*60*60) // Türkiye kalıcı UTC+3
}()

// FormatSendTime, planlı gönderim zamanını API'nin beklediği "GG/AA/YYYY SS:DD" biçimine
// Europe/Istanbul saatinde çevirir.
func FormatSendTime(t time.Time) string {
	return t.In(istanbul).Format("02/01/2006 15:04")
}

type wireRecipients struct {
	Number []string `json:"number"`
}
type wireMessage struct {
	Text       string         `json:"text"`
	Recipients wireRecipients `json:"receipents"` // API'deki yazım böyledir
}

// Send, SMS siparişi oluşturur.
//
// İstek gönderildikten sonra yanıt alınamazsa hata ErrUncertain ile eşleşir: SMS'in
// gidip gitmediği bilinmez, körlemesine tekrar denemeyin.
func (c *Client) Send(ctx context.Context, r SendRequest) (*SendResult, error) {
	sender := strings.TrimSpace(r.Sender)
	if sender == "" {
		sender = strings.TrimSpace(c.cfg.Sender)
	}
	if sender == "" {
		return nil, fmt.Errorf("%w: başlık (sender) boş", ErrInvalidSender)
	}

	msgs := r.Messages
	if len(msgs) > 0 && (r.Text != "" || len(r.Recipients) > 0) {
		return nil, fmt.Errorf("%w: Text/Recipients ile Messages birlikte kullanılamaz", ErrInvalidRequest)
	}
	if len(msgs) == 0 {
		msgs = []Message{{Text: r.Text, Recipients: r.Recipients}}
	}

	wire := make([]wireMessage, 0, len(msgs))
	for i, m := range msgs {
		if strings.TrimSpace(m.Text) == "" {
			return nil, fmt.Errorf("mesaj %d: %w", i, ErrEmptyMessage)
		}
		if !c.cfg.SkipLengthCheck && EstimateParts(m.Text) > MaxParts {
			return nil, fmt.Errorf("mesaj %d: %w", i, ErrMessageTooLong)
		}
		if len(m.Recipients) == 0 {
			return nil, fmt.Errorf("mesaj %d: %w: alıcı yok", i, ErrInvalidRecipients)
		}
		seen := make(map[string]bool, len(m.Recipients))
		nums := make([]string, 0, len(m.Recipients))
		for j, raw := range m.Recipients {
			n, err := NormalizeTR(raw)
			if err != nil {
				return nil, fmt.Errorf("mesaj %d alıcı %d: %w", i, j, err)
			}
			if !seen[n] {
				seen[n] = true
				nums = append(nums, n)
			}
		}
		wire = append(wire, wireMessage{Text: m.Text, Recipients: wireRecipients{Number: nums}})
	}

	iys := r.IYS
	if iys == "" {
		iys = IYSOff
	}
	list := r.IYSList
	switch iys {
	case IYSOff:
		if list == "" {
			list = IYSBireysel
		}
	case IYSOn:
		if list != IYSBireysel && list != IYSTacir {
			return nil, fmt.Errorf("%w: İYS açıkken IYSList BIREYSEL veya TACIR olmalı", ErrIYSConfig)
		}
	default:
		return nil, fmt.Errorf("%w: geçersiz IYS değeri", ErrIYSConfig)
	}

	// Anlık gönderimde boş dizi (canlıda doğrulandı); planlı gönderimde tek string
	// "GG/AA/YYYY SS:DD" (resmi SDK ve dokümana göre).
	var sendAt any = []string{}
	if r.SendAt != nil {
		sendAt = FormatSendTime(*r.SendAt)
	}

	var message any = wire
	if len(wire) == 1 {
		message = wire[0]
	}
	req := map[string]any{
		"authentication": c.auth(),
		"order": map[string]any{
			"sender":       sender,
			"sendDateTime": sendAt,
			"iys":          string(iys),
			"iysList":      string(list),
			"message":      message,
		},
	}
	env, err := c.call(ctx, "send-sms", req)
	if err != nil {
		return nil, err
	}
	id, err := orderID(env)
	if err != nil {
		// Sunucu 200 dedi ama sipariş id'si yok: sipariş oluşmuş olabilir.
		return nil, fmt.Errorf("%w: başarılı yanıtta sipariş id'si yok", ErrUncertain)
	}
	c.logger.Info("iletimerkezi sipariş oluşturuldu", "endpoint", "send-sms", "order_id", id)
	return &SendResult{OrderID: id}, nil
}
