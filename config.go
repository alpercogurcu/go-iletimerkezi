// Package iletimerkezi, İleti Merkezi (https://www.iletimerkezi.com) SMS servisinin
// JSON v1 API'si için framework bağımsız, yalnızca standart kütüphane kullanan bir
// istemci kütüphanesidir.
package iletimerkezi

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	// DefaultBaseURL, İleti Merkezi JSON API v1 kök adresidir.
	DefaultBaseURL = "https://api.iletimerkezi.com/v1"
	// DefaultTimeout, varsayılan HTTP istemcisinin zaman aşımı süresidir.
	DefaultTimeout = 30 * time.Second
)

// Config, istemci ayarlarını tutar.
type Config struct {
	// APIKey, panelden alınan API anahtarıdır (zorunlu). Olduğu gibi gönderilir.
	APIKey string
	// APIHash, panelden alınan hash değeridir (zorunlu). Kütüphane hash HESAPLAMAZ;
	// panelde üretilen değer olduğu gibi gönderilir.
	APIHash string
	// Sender, SendRequest.Sender boş bırakıldığında kullanılan varsayılan SMS başlığıdır
	// (panelde tanımlı ve onaylı, en çok 11 karakter, Türkçe karakter içermeyen).
	Sender string
	// BaseURL, API kök adresidir (varsayılan: DefaultBaseURL). Test sunucuları için değiştirilebilir.
	BaseURL string
	// HTTPClient, özel bir HTTP istemcisi vermek için kullanılır (opsiyonel).
	// Verilirse Timeout alanı yok sayılır; zaman aşımını bu istemci belirler.
	HTTPClient *http.Client
	// Timeout, varsayılan HTTPClient oluşturulurken kullanılır (varsayılan: 30s).
	Timeout time.Duration
	// Logger, loglama için kullanılır (opsiyonel). Yalnızca uç nokta adı, süre, HTTP
	// durum kodu, API durum kodu ve sipariş id'si loglanır; numara, metin, key ve hash ASLA.
	Logger *slog.Logger
	// SkipLengthCheck true ise Send, 7 SMS parçasını aşan metinleri istek atmadan
	// reddetmez (EstimateParts bir tahmindir; sağlayıcının kuralı farklıysa kapatın).
	SkipLengthCheck bool
}

// String, kimlik bilgilerini gizleyerek Config'i yazdırır (yanlışlıkla loglanmasına karşı).
func (c Config) String() string {
	return fmt.Sprintf("iletimerkezi.Config{BaseURL:%q Sender:%q APIKey:[gizli] APIHash:[gizli]}", c.BaseURL, c.Sender)
}

// GoString, %#v için String ile aynı gizli çıktıyı verir.
func (c Config) GoString() string { return c.String() }
