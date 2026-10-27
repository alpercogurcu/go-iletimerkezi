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
	Number string
	Status MessageStatus
}

// Report, GetReport'un bir sayfalık sonucudur.
type Report struct {
	OrderID  string
	Status   OrderStatus
	Messages []ReportMessage
}

// ReportCounts, bir rapor sayfasındaki mesaj durumlarının sayımıdır.
type ReportCounts struct {
	Total, Sent, Failed, Pending int
}

// Counts, bu sayfadaki mesajları duruma göre sayar (sayfalama varsa yalnızca bu sayfayı kapsar).
func (r *Report) Counts() ReportCounts {
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
		ID      flexString `json:"id"`
		Status  flexString `json:"status"`
		Message oneOrMany[struct {
			Number flexString `json:"number"`
			Status flexString `json:"status"`
		}] `json:"message"`
	}
	if len(env.Response.Order) == 0 {
		return nil, fmt.Errorf("%w: yanıtta order yok", ErrUncertain)
	}
	if err := json.Unmarshal(env.Response.Order, &o); err != nil {
		return nil, fmt.Errorf("iletimerkezi: rapor çözümlenemedi: %w", err)
	}
	rep := &Report{OrderID: string(o.ID), Status: OrderStatus(atoiFlex(o.Status))}
	if rep.OrderID == "" {
		rep.OrderID = orderID
	}
	for _, m := range o.Message {
		rep.Messages = append(rep.Messages, ReportMessage{Number: string(m.Number), Status: MessageStatus(atoiFlex(m.Status))})
	}
	return rep, nil
}

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
// toplam, aralıktaki toplam sipariş sayısıdır.
func (c *Client) Summaries(ctx context.Context, start, end time.Time, page int) (orders []OrderSummary, toplam int, err error) {
	if page < 0 || end.Before(start) {
		return nil, 0, fmt.Errorf("%w: sayfa veya tarih aralığı hatalı", ErrInvalidRequest)
	}
	if page == 0 {
		page = 1
	}
	filter := map[string]any{
		"start": start.In(istanbul).Format("2006-01-02"),
		"end":   end.In(istanbul).Format("2006-01-02"),
		"page":  page,
	}
	env, err := c.call(ctx, "get-reports", map[string]any{"authentication": c.auth(), "filter": filter})
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
