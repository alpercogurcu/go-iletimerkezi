package iletimerkezi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"time"
)

const maxResponseBytes = 4 << 20

// Client, İleti Merkezi API istemcisidir. Eşzamanlı kullanım için güvenlidir.
type Client struct {
	cfg    Config
	http   *http.Client
	logger *slog.Logger
	base   string
}

// NewClient, yeni bir istemci oluşturur.
func NewClient(cfg Config) (*Client, error) {
	if cfg.APIKey == "" || cfg.APIHash == "" {
		return nil, fmt.Errorf("%w: APIKey ve APIHash zorunludur", ErrInvalidRequest)
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: cfg.Timeout}
	}
	lg := cfg.Logger
	if lg == nil {
		lg = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Client{cfg: cfg, http: hc, logger: lg, base: cfg.BaseURL}, nil
}

// flexString, JSON'da string, sayı veya null olarak gelebilen değerleri string okur.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = ""
		return nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*f = flexString(strings.TrimSpace(v))
		return nil
	}
	*f = flexString(s)
	return nil
}

// oneOrMany, tek nesne veya dizi olarak gelebilen alanlar içindir.
type oneOrMany[T any] []T

func (o *oneOrMany[T]) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*o = nil
		return nil
	}
	if s[0] == '[' {
		var v []T
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*o = v
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*o = []T{v}
	return nil
}

type envelope struct {
	Response *struct {
		Status struct {
			Code    flexString `json:"code"`
			Message string     `json:"message"`
		} `json:"status"`
		Order   json.RawMessage `json:"order"`
		Balance json.RawMessage `json:"balance"`
		Senders json.RawMessage `json:"senders"`
		Count   flexString      `json:"count"`
		Orders  json.RawMessage `json:"orders"`
	} `json:"response"`
}

// call, isteği gönderir ve başarılı (200) yanıtın response nesnesini döner.
// Hata sınıflandırması:
//   - İstek yazılamadıysa: ErrNotSent
//   - Yazıldı ama yanıt yok/çözümlenemedi/gövdesiz 5xx: ErrUncertain
//   - API durum kodu 200 değil: *APIError
func (c *Client) call(ctx context.Context, endpoint string, req any) (*envelope, error) {
	body, err := json.Marshal(map[string]any{"request": req})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+endpoint+"/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotSent, err)
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json")

	written := false
	hreq = hreq.WithContext(httptrace.WithClientTrace(hreq.Context(), &httptrace.ClientTrace{
		WroteRequest: func(i httptrace.WroteRequestInfo) {
			if i.Err == nil {
				written = true
			}
		},
	}))

	start := time.Now()
	resp, err := c.http.Do(hreq)
	if err != nil {
		c.logger.Warn("iletimerkezi istek hatası", "endpoint", endpoint, "duration", time.Since(start), "sent", written)
		if !written {
			return nil, fmt.Errorf("%w: %w", ErrNotSent, scrub(err))
		}
		return nil, fmt.Errorf("%w: %w", ErrUncertain, scrub(err))
	}
	defer resp.Body.Close()
	raw, rerr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	dur := time.Since(start)
	if rerr != nil {
		c.logger.Warn("iletimerkezi yanıt okunamadı", "endpoint", endpoint, "duration", dur, "http_status", resp.StatusCode)
		return nil, fmt.Errorf("%w: yanıt okunamadı: %w", ErrUncertain, scrub(rerr))
	}

	var env envelope
	if jerr := json.Unmarshal(raw, &env); jerr != nil || env.Response == nil {
		c.logger.Warn("iletimerkezi çözümlenemeyen yanıt", "endpoint", endpoint, "duration", dur, "http_status", resp.StatusCode)
		if resp.StatusCode >= 500 || resp.StatusCode < 400 {
			return nil, fmt.Errorf("%w: HTTP %d, çözümlenemeyen yanıt", ErrUncertain, resp.StatusCode)
		}
		return nil, &APIError{Code: resp.StatusCode, HTTPStatus: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}

	code, cerr := strconv.Atoi(string(env.Response.Status.Code))
	if cerr != nil {
		c.logger.Warn("iletimerkezi geçersiz durum kodu", "endpoint", endpoint, "duration", dur, "http_status", resp.StatusCode)
		return nil, fmt.Errorf("%w: HTTP %d, durum kodu okunamadı", ErrUncertain, resp.StatusCode)
	}
	c.logger.Debug("iletimerkezi yanıt", "endpoint", endpoint, "duration", dur, "http_status", resp.StatusCode, "api_code", code)
	if code != 200 {
		return nil, &APIError{Code: code, Message: env.Response.Status.Message, HTTPStatus: resp.StatusCode}
	}
	return &env, nil
}

// scrub, url.Error içindeki URL'yi atıp yalnızca altta yatan hatayı döndürür.
func scrub(err error) error {
	var ue interface{ Unwrap() error }
	if errors.As(err, &ue) {
		if in := ue.Unwrap(); in != nil {
			return in
		}
	}
	return err
}

func (c *Client) auth() map[string]string {
	return map[string]string{"key": c.cfg.APIKey, "hash": c.cfg.APIHash}
}
