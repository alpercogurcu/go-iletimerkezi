package iletimerkezi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTest, verilen handler ile bir test istemcisi döner.
func newTest(t *testing.T, h http.HandlerFunc, mod ...func(*Config)) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cfg := Config{APIKey: "test-key", APIHash: "test-hash", Sender: "TESTBASLIK", BaseURL: srv.URL}
	for _, m := range mod {
		m(&cfg)
	}
	c, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func reply(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

func okSend(id string) string {
	return `{"response":{"status":{"code":"200","message":"İşlem başarılı"},"order":{"id":"` + id + `"}}}`
}

func TestNewClientValidation(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Fatal("anahtarsız istemci oluşmamalı")
	}
}

func TestConfigStringHidesSecrets(t *testing.T) {
	s := Config{APIKey: "gizli-key", APIHash: "gizli-hash"}.String()
	if strings.Contains(s, "gizli-key") || strings.Contains(s, "gizli-hash") {
		t.Fatal("kimlik bilgisi sızdı: " + s)
	}
}

func TestSendSuccessAndWireFormat(t *testing.T) {
	var got map[string]any
	var path string
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(200, okSend("12345"))(w, r)
	})
	res, err := c.Send(context.Background(), SendRequest{
		Text:       "Merhaba",
		Recipients: []string{"+90 (531) 234-56-78", "05312345678", "5312345679"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.OrderID != "12345" {
		t.Fatalf("order id: %q", res.OrderID)
	}
	if path != "/send-sms/json" {
		t.Fatalf("path: %s", path)
	}
	req := got["request"].(map[string]any)
	auth := req["authentication"].(map[string]any)
	if auth["key"] != "test-key" || auth["hash"] != "test-hash" {
		t.Fatalf("auth: %v", auth)
	}
	order := req["order"].(map[string]any)
	if order["sender"] != "TESTBASLIK" || order["iys"] != "0" || order["iysList"] != "BIREYSEL" {
		t.Fatalf("order: %v", order)
	}
	if sd := order["sendDateTime"].([]any); len(sd) != 0 {
		t.Fatalf("sendDateTime boş dizi olmalı: %v", sd)
	}
	msg := order["message"].(map[string]any)
	if msg["text"] != "Merhaba" {
		t.Fatalf("text: %v", msg["text"])
	}
	nums := msg["receipents"].(map[string]any)["number"].([]any)
	if len(nums) != 2 || nums[0] != "5312345678" || nums[1] != "5312345679" { // tekilleştirildi
		t.Fatalf("numbers: %v", nums)
	}
}

func TestSendMultipleMessagesAndScheduleAndIYS(t *testing.T) {
	var got map[string]any
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(200, okSend("7"))(w, r)
	})
	at := time.Date(2026, 3, 5, 6, 30, 0, 0, time.UTC) // 09:30 İstanbul
	_, err := c.Send(context.Background(), SendRequest{
		Sender: "BASKA",
		Messages: []Message{
			{Text: "bir", Recipients: []string{"5311111111"}},
			{Text: "iki", Recipients: []string{"5322222222"}},
		},
		SendAt: &at, IYS: IYSOn, IYSList: IYSTacir,
	})
	if err != nil {
		t.Fatal(err)
	}
	order := got["request"].(map[string]any)["order"].(map[string]any)
	if order["sender"] != "BASKA" || order["iys"] != "1" || order["iysList"] != "TACIR" {
		t.Fatalf("order: %v", order)
	}
	if sd := order["sendDateTime"].([]any); len(sd) != 1 || sd[0] != "05/03/2026 09:30" {
		t.Fatalf("sendDateTime: %v", sd)
	}
	if arr, ok := order["message"].([]any); !ok || len(arr) != 2 {
		t.Fatalf("message dizi olmalı: %v", order["message"])
	}
}

func TestFormatSendTime(t *testing.T) {
	tm := time.Date(2026, 12, 31, 21, 5, 0, 0, time.UTC) // İstanbul'da 01/01/2027 00:05
	if g := FormatSendTime(tm); g != "01/01/2027 00:05" {
		t.Fatalf("got %q", g)
	}
}

func TestSendLocalValidationDoesNotHitNetwork(t *testing.T) {
	hits := 0
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) { hits++; reply(200, okSend("1"))(w, r) })
	long := strings.Repeat("a", 1200) // 1200/153 > 7
	cases := []struct {
		name string
		req  SendRequest
		want error
	}{
		{"uzun", SendRequest{Text: long, Recipients: []string{"5311111111"}}, ErrMessageTooLong},
		{"boş metin", SendRequest{Text: "  ", Recipients: []string{"5311111111"}}, ErrEmptyMessage},
		{"alıcı yok", SendRequest{Text: "x"}, ErrInvalidRecipients},
		{"kötü numara", SendRequest{Text: "x", Recipients: []string{"12345"}}, ErrInvalidRecipients},
		{"iys liste yok", SendRequest{Text: "x", Recipients: []string{"5311111111"}, IYS: IYSOn}, ErrIYSConfig},
		{"ikisi birden", SendRequest{Text: "x", Recipients: []string{"5311111111"}, Messages: []Message{{Text: "y", Recipients: []string{"5311111111"}}}}, ErrInvalidRequest},
	}
	for _, tc := range cases {
		_, err := c.Send(context.Background(), tc.req)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: err=%v, beklenen %v", tc.name, err, tc.want)
		}
	}
	if hits != 0 {
		t.Fatalf("yerel doğrulama ağa çıktı: %d", hits)
	}
}

func TestSendNoSenderRejected(t *testing.T) {
	c, _ := newTest(t, reply(200, okSend("1")), func(c *Config) { c.Sender = "" })
	_, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
	if !errors.Is(err, ErrInvalidSender) {
		t.Fatalf("err=%v", err)
	}
}

func TestSkipLengthCheck(t *testing.T) {
	c, _ := newTest(t, reply(200, okSend("1")), func(c *Config) { c.SkipLengthCheck = true })
	if _, err := c.Send(context.Background(), SendRequest{Text: strings.Repeat("a", 1200), Recipients: []string{"5311111111"}}); err != nil {
		t.Fatal(err)
	}
}

func TestAPIErrorCodes(t *testing.T) {
	cases := map[int]error{
		400: ErrBadRequest, 401: ErrUnauthorized, 402: ErrInsufficientBalance, 404: ErrMethodNotFound,
		422: ErrValidation, 450: ErrSenderNotAllowed, 451: ErrDuplicateOrder, 452: ErrInvalidRecipients,
		453: ErrMessageTooLong, 454: ErrEmptyMessage, 455: ErrOrderNotFound, 456: ErrNotYetDue,
		457: ErrInvalidDate, 458: ErrInvalidDate, 468: ErrIYSConfig, 469: ErrIYSConfig, 470: ErrIYSConfig,
		503: ErrServiceUnavailable,
	}
	for code, sentinel := range cases {
		body := `{"response":{"status":{"code":"` + itoa(code) + `","message":"hata"}}}`
		c, _ := newTest(t, reply(code, body))
		_, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
		var ae *APIError
		if !errors.As(err, &ae) || ae.Code != code {
			t.Fatalf("%d: *APIError bekleniyordu: %v", code, err)
		}
		if !errors.Is(err, sentinel) {
			t.Errorf("%d: %v ile eşleşmeli", code, sentinel)
		}
		if errors.Is(err, ErrUncertain) {
			t.Errorf("%d: API hatası belirsiz sayılmamalı", code)
		}
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestNumericStatusCodeAndOrderID(t *testing.T) {
	c, _ := newTest(t, reply(200, `{"response":{"status":{"code":200,"message":"ok"},"order":{"id":428}}}`))
	res, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
	if err != nil || res.OrderID != "428" {
		t.Fatalf("res=%v err=%v", res, err)
	}
	c2, _ := newTest(t, reply(401, `{"response":{"status":{"code":401,"message":"x"}}}`))
	if _, err := c2.Balance(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err=%v", err)
	}
}

func TestSuccessWithoutOrderIDIsUncertain(t *testing.T) {
	c, _ := newTest(t, reply(200, `{"response":{"status":{"code":"200","message":"ok"}}}`))
	_, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("err=%v", err)
	}
}

func TestTimeoutIsUncertain(t *testing.T) {
	release := make(chan struct{})
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}, func(c *Config) { c.Timeout = 80 * time.Millisecond })
	defer close(release)
	_, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("err=%v", err)
	}
	var ae *APIError
	if errors.As(err, &ae) {
		t.Fatal("zaman aşımı APIError olmamalı")
	}
}

func TestContextCancelAfterSendIsUncertain(t *testing.T) {
	started := make(chan struct{})
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	_, err := c.Send(ctx, SendRequest{Text: "x", Recipients: []string{"5311111111"}})
	if !errors.Is(err, ErrUncertain) || !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestConnectionDroppedIsUncertain(t *testing.T) {
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	})
	_, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("err=%v", err)
	}
}

func TestEmptyBody5xxIsUncertain(t *testing.T) {
	for _, code := range []int{500, 502, 503} {
		c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) })
		_, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
		if !errors.Is(err, ErrUncertain) {
			t.Fatalf("%d: err=%v", code, err)
		}
	}
}

func TestConnectionRefusedIsNotSent(t *testing.T) {
	c, srv := newTest(t, reply(200, okSend("1")))
	srv.Close()
	_, err := c.Send(context.Background(), SendRequest{Text: "x", Recipients: []string{"5311111111"}})
	if !errors.Is(err, ErrNotSent) || errors.Is(err, ErrUncertain) {
		t.Fatalf("err=%v", err)
	}
}

func TestGetReportFlexParse(t *testing.T) {
	var got map[string]any
	body := `{"response":{"status":{"code":200,"message":"ok"},"order":{"id":428,"status":"113","message":[
	 {"number":5301234567,"status":110},{"number":"5301234568","status":"111"},{"number":5301234569,"status":112}]}}}`
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(200, body)(w, r)
	})
	rep, err := c.GetReport(context.Background(), "428", 2, 50)
	if err != nil {
		t.Fatal(err)
	}
	o := got["request"].(map[string]any)["order"].(map[string]any)
	if o["id"] != "428" || o["page"] != float64(2) || o["rowCount"] != float64(50) {
		t.Fatalf("istek: %v", o)
	}
	if rep.OrderID != "428" || rep.Status != OrderSending || len(rep.Messages) != 3 {
		t.Fatalf("rep: %+v", rep)
	}
	if rep.Messages[0].Number != "5301234567" || rep.Messages[1].Status != MessageSent {
		t.Fatalf("mesajlar: %+v", rep.Messages)
	}
	if cn := rep.Counts(); cn != (ReportCounts{Total: 3, Sent: 1, Failed: 1, Pending: 1}) {
		t.Fatalf("counts: %+v", cn)
	}
}

func TestGetReportSingleMessageObject(t *testing.T) {
	c, _ := newTest(t, reply(200, `{"response":{"status":{"code":200},"order":{"id":"1","status":114,"message":{"number":"5301234567","status":111}}}}`))
	rep, err := c.GetReport(context.Background(), "1", 0, 0)
	if err != nil || len(rep.Messages) != 1 || rep.Status != OrderDone {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
}

func TestGetReportErrors(t *testing.T) {
	c, _ := newTest(t, reply(200, `{"response":{"status":{"code":455,"message":"Sipariş bulunamadı"}}}`))
	if _, err := c.GetReport(context.Background(), "9", 0, 0); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("err=%v", err)
	}
	if _, err := c.GetReport(context.Background(), "", 0, 0); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err=%v", err)
	}
	if _, err := c.GetReport(context.Background(), "1", 1, 5000); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err=%v", err)
	}
}

func TestBalance(t *testing.T) {
	c, _ := newTest(t, reply(200, `{"response":{"status":{"code":200,"message":"ok"},"balance":{"amount":"12,50","sms":"18343"}}}`))
	b, err := c.Balance(context.Background())
	if err != nil || b.Amount != 12.5 || b.SMS != 18343 {
		t.Fatalf("b=%+v err=%v", b, err)
	}
}

func TestSenders(t *testing.T) {
	c, _ := newTest(t, reply(200, `{"response":{"status":{"code":200,"message":"ok"},"senders":{"sender":["BIR","IKI"]}}}`))
	s, err := c.Senders(context.Background())
	if err != nil || len(s) != 2 || s[0] != "BIR" {
		t.Fatalf("s=%v err=%v", s, err)
	}
	c2, _ := newTest(t, reply(200, `{"response":{"status":{"code":200,"message":"ok"},"senders":{"sender":"TEK"}}}`))
	s, err = c2.Senders(context.Background())
	if err != nil || len(s) != 1 || s[0] != "TEK" {
		t.Fatalf("s=%v err=%v", s, err)
	}
}

func TestCancel(t *testing.T) {
	var path string
	var got map[string]any
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(200, `{"response":{"status":{"code":"200","message":"ok"}}}`)(w, r)
	})
	if err := c.Cancel(context.Background(), "428"); err != nil {
		t.Fatal(err)
	}
	if path != "/cancel-order/json" || got["request"].(map[string]any)["order"].(map[string]any)["id"] != "428" {
		t.Fatalf("path=%s got=%v", path, got)
	}
	c2, _ := newTest(t, reply(200, `{"response":{"status":{"code":"455","message":"x"}}}`))
	if err := c2.Cancel(context.Background(), "1"); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestSummaries(t *testing.T) {
	var got map[string]any
	c, _ := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		reply(200, `{"response":{"status":{"code":200,"message":"ok"},"count":1,"orders":[{"id":89,"status":113,"total":2,"delivered":1,"undelivered":0,"waiting":1,"submitAt":"a","sendAt":"b","sender":"X"}]}}`)(w, r)
	})
	s := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	orders, n, err := c.Summaries(context.Background(), s, s.Add(24*time.Hour), 0)
	if err != nil || n != 1 || len(orders) != 1 || orders[0].Waiting != 1 || orders[0].ID != "89" {
		t.Fatalf("orders=%+v n=%d err=%v", orders, n, err)
	}
	f := got["request"].(map[string]any)["filter"].(map[string]any)
	if f["start"] != "2026-01-02" || f["end"] != "2026-01-03" || f["page"] != float64(1) {
		t.Fatalf("filter=%v", f)
	}
}

type captureHandler struct{ lines []string }

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	s := r.Message
	r.Attrs(func(a slog.Attr) bool { s += " " + a.Key + "=" + a.Value.String(); return true })
	h.lines = append(h.lines, s)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func TestLoggerNeverLeaksSensitiveData(t *testing.T) {
	h := &captureHandler{}
	c, _ := newTest(t, reply(200, okSend("99")), func(c *Config) { c.Logger = slog.New(h) })
	_, err := c.Send(context.Background(), SendRequest{Text: "ÇOK GİZLİ METİN", Recipients: []string{"5312345678"}})
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(h.lines, "\n")
	for _, bad := range []string{"test-key", "test-hash", "5312345678", "GİZLİ"} {
		if strings.Contains(all, bad) {
			t.Fatalf("log %q içeriyor:\n%s", bad, all)
		}
	}
	if !strings.Contains(all, "order_id=99") {
		t.Fatalf("order_id loglanmalı:\n%s", all)
	}
}
