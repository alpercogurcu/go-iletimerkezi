package iletimerkezi

import (
	"errors"
	"fmt"
)

// Sentinel hatalar. errors.Is ile kontrol edilir; API'den dönen *APIError de
// ilgili koda göre bunlarla eşleşir.
var (
	// ErrUncertain, istek sunucuya GÖNDERİLDİKTEN sonra yanıt alınamadığında (zaman aşımı,
	// bağlantı kopması, gövdesiz 5xx, çözümlenemeyen yanıt) döner. SMS'in gidip gitmediği
	// BİLİNMEZ; körlemesine tekrar denemeyin, önce GetReport/Summaries ile durumu doğrulayın.
	ErrUncertain = errors.New("iletimerkezi: sonuç belirsiz (istek gönderildi, yanıt alınamadı)")
	// ErrNotSent, istek sunucuya hiç yazılamadığında (DNS, bağlantı reddi, iptal edilmiş
	// context vb.) döner. SMS kesinlikle gönderilmemiştir; tekrar denemek güvenlidir.
	ErrNotSent = errors.New("iletimerkezi: istek sunucuya gönderilemedi")

	ErrBadRequest          = errors.New("iletimerkezi: istek çözümlenemedi (400)")
	ErrUnauthorized        = errors.New("iletimerkezi: üyelik bilgileri hatalı veya IP yetkisiz (401)")
	ErrInsufficientBalance = errors.New("iletimerkezi: bakiye yetersiz (402)")
	ErrMethodNotFound      = errors.New("iletimerkezi: API yöntemi bulunamadı (404)")
	ErrValidation          = errors.New("iletimerkezi: istek değerleri doğrulanamadı (422)")
	ErrSenderNotAllowed    = errors.New("iletimerkezi: gönderilen başlık kullanıma uygun değil (450)")
	// ErrDuplicateOrder (451): aynı metin aynı alıcılara kısa süre (yaklaşık 10 dk) içinde
	// tekrar gönderilmek istendi. Genellikle ilk gönderimin başarılı olduğu anlamına gelir.
	ErrDuplicateOrder    = errors.New("iletimerkezi: tekrar eden sipariş (451)")
	ErrInvalidRecipients = errors.New("iletimerkezi: mesaj alıcıları hatalı (452)")
	// ErrMessageTooLong, metin 7 SMS parçasını aştığında hem yerel doğrulamadan hem de 453'ten döner.
	ErrMessageTooLong     = errors.New("iletimerkezi: mesaj/sipariş boyutu aşıldı (7 SMS sınırı)")
	ErrEmptyMessage       = errors.New("iletimerkezi: mesaj metni boş (454)")
	ErrOrderNotFound      = errors.New("iletimerkezi: sipariş bulunamadı (455)")
	ErrNotYetDue          = errors.New("iletimerkezi: sipariş gönderim tarihi henüz gelmedi (456)")
	ErrInvalidDate        = errors.New("iletimerkezi: tarih biçimi veya aralığı hatalı (457/458)")
	ErrInvalidSender      = errors.New("iletimerkezi: başlık boş, 11 karakterden uzun veya Türkçe karakter içeriyor (459/460)")
	ErrIYSConfig          = errors.New("iletimerkezi: İYS ayarları eksik veya hatalı (468/469/470)")
	ErrServiceUnavailable = errors.New("iletimerkezi: servis geçici olarak kapalı (503)")

	// ErrHTTP, API JSON zarfı yerine JSON olmayan bir HTTP hata yanıtı (WAF/proxy HTML sayfası vb.)
	// geldiğinde döner; *APIError.HTTPStatus dolu, Code 0 olur.
	ErrHTTP = errors.New("iletimerkezi: JSON olmayan HTTP hata yanıtı")
	// ErrUnexpectedResponse, başarılı (200) yanıtın beklenen alanları içermemesidir.
	ErrUnexpectedResponse = errors.New("iletimerkezi: beklenmeyen yanıt biçimi")

	// ErrInvalidRequest, istek gönderilmeden önce yerel doğrulamada yakalanan hatalardır.
	ErrInvalidRequest = errors.New("iletimerkezi: geçersiz istek")
)

// codeSentinels, API durum kodunu sentinel hatalara eşler.
var codeSentinels = map[int][]error{
	400: {ErrBadRequest},
	401: {ErrUnauthorized},
	402: {ErrInsufficientBalance},
	404: {ErrMethodNotFound},
	422: {ErrValidation},
	450: {ErrSenderNotAllowed},
	451: {ErrDuplicateOrder},
	452: {ErrInvalidRecipients},
	453: {ErrMessageTooLong},
	454: {ErrEmptyMessage},
	455: {ErrOrderNotFound},
	456: {ErrNotYetDue},
	457: {ErrInvalidDate},
	458: {ErrInvalidDate},
	459: {ErrInvalidSender},
	460: {ErrInvalidSender},
	466: {ErrInvalidRecipients},
	468: {ErrIYSConfig},
	469: {ErrIYSConfig},
	470: {ErrIYSConfig},
	503: {ErrServiceUnavailable},
}

// APIError, İleti Merkezi'nin döndürdüğü başarısız durum kodunu temsil eder.
type APIError struct {
	// Code, API durum kodudur (örn. 401, 402, 451).
	Code int
	// Message, API'nin döndürdüğü açıklamadır (varsa).
	Message string
	// HTTPStatus, HTTP yanıt kodudur.
	HTTPStatus int
}

// Error error arayüzünü sağlar.
func (e *APIError) Error() string {
	if e.Code == 0 {
		return fmt.Sprintf("iletimerkezi HTTP hatası [%d]", e.HTTPStatus)
	}
	if e.Message != "" {
		return fmt.Sprintf("iletimerkezi hatası [%d]: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("iletimerkezi hatası [%d]", e.Code)
}

// Is, errors.Is ile sentinel eşleşmesini sağlar.
func (e *APIError) Is(target error) bool {
	if e.Code == 0 && target == ErrHTTP {
		return true
	}
	for _, s := range codeSentinels[e.Code] {
		if s == target {
			return true
		}
	}
	return false
}
