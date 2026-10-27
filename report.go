package iletimerkezi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// OrderStatus, siparişin genel gönderim durumudur.
type OrderStatus int

const (
	OrderSending OrderStatus = 113 // gönderim devam ediyor
	OrderDone    OrderStatus = 114 // gönderim tamamlandı
	OrderFailed  OrderStatus = 115 // sipariş gönderilemedi
)

// MessageStatus, tek bir alıcıya giden mesajın durumudur.
type MessageStatus int

const (
	MessageSending MessageStatus = 110 // gönderiliyor
	MessageSent    MessageStatus = 111 // gönderildi
	MessageFailed  MessageStatus = 112 // gönderilemedi
)

// ReportMessage, rapordaki tek alıcı satırıdır.
type ReportMessage struct {
	// Number, sunucunun döndürdüğü biçimdedir (örn. "+905xxxxxxxxx").
	Number string
	Status MessageStatus
}

// NormalizedNumber, Number'ı NormalizeTR ile "5xxxxxxxxx" biçimine çevirir (eşleştirme için).
func (m ReportMessage) NormalizedNumber() (string, error) { return NormalizeTR(m.Number) }

// Report, GetReport'un bir sayfalık sonucudur.
type Report struct {
	OrderID string
	Status  OrderStatus
	// Sunucu sayaçları (tüm sipariş için; HasTotals false ise yanıtta yoktu).
	HasTotals   bool
	Total       int
	Delivered   int
	Undelivered int
	Waiting     int
	// SubmitAt ve SendAt Europe/Istanbul saatindedir; çözümlenemezse sıfır değerdir.
	SubmitAt time.Time
	SendAt   time.Time
	Sender   string
	// Price, siparişin ücretidir (birimi sağlayıcıya bağlıdır; yanıtta yoksa 0).
	Price float64
	// Messages, bu sayfadaki alıcı satırlarıdır.
	Messages []ReportMessage
}

// ReportCounts, mesaj durumu sayımıdır.
type ReportCounts struct {
	Total, Sent, Failed, Pending int
}

// Counts, sayımı döner. Yanıtta sunucu sayaçları varsa onları (tüm sipariş; Sent=delivered,
// Failed=undelivered, Pending=waiting) tercih eder; yoksa yalnızca bu sayfadaki mesajları sayar.
func (r *Report) Counts() ReportCounts {
	if r.HasTotals {
		return ReportCounts{Total: r.Total, Sent: r.Delivered, Failed: r.Undelivered, Pending: r.Waiting}
	}
	var c ReportCounts
	for _, m := range r.Messages {
		c.Total++
		switch m.Status {
		case MessageSent:
			c.Sent++
		case MessageFailed:
			c.Failed++
		default:
			c.Pending++
		}
	}
	return c
}

func parseIstanbul(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(s), istanbul)
	if err != nil {
		return time.Time{}
	}
	return t
}

func atoiFlex(f flexString) int {
	n, _ := strconv.Atoi(string(f))
	return n
}

func orderID(env *envelope) (string, error) {
	var o struct {
		ID flexString `json:"id"`
	}
	if len(env.Response.Order) == 0 || json.Unmarshal(env.Response.Order, &o) != nil || o.ID == "" {
		return "", fmt.Errorf("sipariş id'si yok")
	}
	return string(o.ID), nil
}

// MaxReportRows, bir rapor sayfasındaki en çok mesaj sayısıdır.
const MaxReportRows = 1000

// GetReport, siparişin gönderim raporunu döner. page ve rowCount 0 ise API varsayılanları
// (1 ve 1000) kullanılır; rowCount en çok 1000'dir.
func (c *Client) GetReport(ctx context.Context, orderID string, page, rowCount int) (*Report, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, fmt.Errorf("%w: sipariş id'si boş", ErrInvalidRequest)
	}
	if page < 0 || rowCount < 0 || rowCount > MaxReportRows {
		return nil, fmt.Errorf("%w: page/rowCount aralık dışı", ErrInvalidRequest)
	}
	order := map[string]any{"id": orderID}
	if page > 0 {
		order["page"] = page
	}
	if rowCount > 0 {
		order["rowCount"] = rowCount
	}
	env, err := c.call(ctx, "get-report", map[string]any{"authentication": c.auth(), "order": order})
	if err != nil {
		return nil, err
	}
	var o struct {
		ID          flexString  `json:"id"`
		Status      flexString  `json:"status"`
		Total       *flexString `json:"total"`
		Delivered   flexString  `json:"delivered"`
		Undelivered flexString  `json:"undelivered"`
		Waiting     flexString  `json:"waiting"`
		SubmitAt    string      `json:"submitAt"`
		SendAt      string      `json:"sendAt"`
		Sender      string      `json:"sender"`
		Price       flexString  `json:"price"`
		Message     oneOrMany[struct {
			Number flexString `json:"number"`
			Status flexString `json:"status"`
		}] `json:"message"`
	}
	if raw := strings.TrimSpace(string(env.Response.Order)); raw == "" || raw == "null" {
		return nil, fmt.Errorf("%w: yanıtta order yok", ErrUnexpectedResponse)
	}
	if err := json.Unmarshal(env.Response.Order, &o); err != nil {
		return nil, fmt.Errorf("%w: rapor çözümlenemedi: %w", ErrUnexpectedResponse, err)
	}
	rep := &Report{
		OrderID: string(o.ID), Status: OrderStatus(atoiFlex(o.Status)),
		Delivered: atoiFlex(o.Delivered), Undelivered: atoiFlex(o.Undelivered), Waiting: atoiFlex(o.Waiting),
		SubmitAt: parseIstanbul(o.SubmitAt), SendAt: parseIstanbul(o.SendAt), Sender: o.Sender,
	}
	if o.Total != nil {
		rep.HasTotals, rep.Total = true, atoiFlex(*o.Total)
	}
	rep.Price, _ = strconv.ParseFloat(strings.ReplaceAll(string(o.Price), ",", "."), 64)
	if rep.OrderID == "" {
		rep.OrderID = orderID
	}
	for _, m := range o.Message {
		rep.Messages = append(rep.Messages, ReportMessage{Number: string(m.Number), Status: MessageStatus(atoiFlex(m.Status))})
	}
	return rep, nil
}

// MaxSummaryDays, Summaries için izin verilen en uzun tarih aralığıdır (gün).
const MaxSummaryDays = 10

// OrderSummary, Summaries'in döndürdüğü sipariş özetidir.
type OrderSummary struct {
	ID          string
	Status      OrderStatus
	Total       int
	Delivered   int
	Undelivered int
	Waiting     int
	SubmitAt    string
	SendAt      string
	Sender      string
}

// Summaries, [start, end] tarih aralığındaki siparişlerin özetini döner (get-reports).
// Tarihler Europe/Istanbul gününe göre "YYYY-AA-GG" biçiminde gönderilir. page 0 ise 1.
// Tarih aralığı 10 günden uzun olamaz (sunucu 458 döner); kütüphane bunu istek atmadan
// ErrInvalidDate ile reddeder. page hem filter içinde hem istek kökünde gönderilir.
// toplam, aralıktaki toplam sipariş sayısıdır.
func (c *Client) Summaries(ctx context.Context, start, end time.Time, page int) (orders []OrderSummary, toplam int, err error) {
	if page < 0 || end.Before(start) {
		return nil, 0, fmt.Errorf("%w: sayfa veya tarih aralığı hatalı", ErrInvalidRequest)
	}
	if page == 0 {
		page = 1
	}
	sd, ed := start.In(istanbul), end.In(istanbul)
	days := int(time.Date(ed.Year(), ed.Month(), ed.Day(), 0, 0, 0, 0, time.UTC).Sub(time.Date(sd.Year(), sd.Month(), sd.Day(), 0, 0, 0, 0, time.UTC)).Hours() / 24)
	if days > MaxSummaryDays {
		return nil, 0, fmt.Errorf("%w: tarih aralığı %d günden uzun olamaz", ErrInvalidDate, MaxSummaryDays)
	}
	filter := map[string]any{
		"start": start.In(istanbul).Format("2006-01-02"),
		"end":   end.In(istanbul).Format("2006-01-02"),
		"page":  page,
	}
	env, err := c.call(ctx, "get-reports", map[string]any{"authentication": c.auth(), "filter": filter, "page": page})
	if err != nil {
		return nil, 0, err
	}
	var list []struct {
		ID          flexString `json:"id"`
		Status      flexString `json:"status"`
		Total       flexString `json:"total"`
		Delivered   flexString `json:"delivered"`
		Undelivered flexString `json:"undelivered"`
		Waiting     flexString `json:"waiting"`
		SubmitAt    string     `json:"submitAt"`
		SendAt      string     `json:"sendAt"`
		Sender      string     `json:"sender"`
	}
	if len(env.Response.Orders) > 0 {
		if err := json.Unmarshal(env.Response.Orders, &list); err != nil {
			return nil, 0, fmt.Errorf("iletimerkezi: özet çözümlenemedi: %w", err)
		}
	}
	for _, o := range list {
		orders = append(orders, OrderSummary{
			ID: string(o.ID), Status: OrderStatus(atoiFlex(o.Status)), Total: atoiFlex(o.Total),
			Delivered: atoiFlex(o.Delivered), Undelivered: atoiFlex(o.Undelivered), Waiting: atoiFlex(o.Waiting),
			SubmitAt: o.SubmitAt, SendAt: o.SendAt, Sender: o.Sender,
		})
	}
	return orders, atoiFlex(env.Response.Count), nil
}
