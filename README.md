# go-iletimerkezi

[İleti Merkezi](https://www.iletimerkezi.com) SMS servisinin JSON v1 API'si için Go (Golang) dilinde yazılmış, hafif ve yeniden kullanılabilir bir istemci kütüphanesidir.

Herhangi bir web framework'üne (Gin, Echo, Fiber vb.) veya ORM'e bağlı değildir; **yalnızca standart kütüphane** kullanır (dış bağımlılık yok).

API dokümantasyonu: <https://www.toplusmsapi.com/>

## Özellikler

- SMS gönderimi: tek metin / çok alıcı, çok metin / farklı alıcılar, planlı (ileri tarihli) gönderim, İYS ayarı
- Gönderim raporu (sayfalı), tarih aralığı sipariş özeti, ileri tarihli siparişi iptal
- Bakiye ve onaylı başlık (sender) listesi
- Tipli hatalar (`*APIError`) ve `errors.Is` ile kullanılabilen sentinel'ler
- Ağ/zaman aşımı belirsizliği için `ErrUncertain`
- Telefon normalizasyonu (`NormalizeTR`), maskeleme (`MaskNumber`), SMS parça sayısı tahmini (`EstimateParts`)

## Kurulum

```bash
go get github.com/alpercogurcu/go-iletimerkezi
```

Go 1.24+ gerekir.

## Hızlı başlangıç

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	im "github.com/alpercogurcu/go-iletimerkezi"
)

func main() {
	client, err := im.NewClient(im.Config{
		APIKey:  os.Getenv("ILETI_MERKEZI_KEY"),  // panelden alınan anahtar
		APIHash: os.Getenv("ILETI_MERKEZI_HASH"), // panelden alınan hash (olduğu gibi gönderilir)
		Sender:  "BASLIKADI",                     // varsayılan, onaylı SMS başlığı
	})
	if err != nil {
		log.Fatal(err)
	}

	to, err := im.NormalizeTR("+90 (5xx) xxx xx xx") // → "5xxxxxxxxx"
	if err != nil {
		log.Fatal(err)
	}

	res, err := client.Send(context.Background(), im.SendRequest{
		Text:       "Siparişiniz hazır.",
		Recipients: []string{to},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("sipariş id:", res.OrderID)
}
```

### Config

| Alan | Açıklama | Varsayılan |
|------|----------|-----------|
| `APIKey`, `APIHash` | Panelden alınan kimlik bilgileri (zorunlu). Kütüphane hash **hesaplamaz**, olduğu gibi gönderir. | – |
| `Sender` | `SendRequest.Sender` boşsa kullanılan başlık | – |
| `BaseURL` | API kök adresi | `https://api.iletimerkezi.com/v1` |
| `HTTPClient` | Özel HTTP istemcisi (verilirse `Timeout` yok sayılır) | otomatik |
| `Timeout` | Varsayılan istemcinin zaman aşımı | 30 sn |
| `Logger` | `*slog.Logger` (opsiyonel) | kapalı |
| `SkipLengthCheck` | 7 parça yerel kontrolünü kapatır | `false` |

### Farklı alıcılara farklı metinler, planlı gönderim, İYS

```go
at := time.Now().Add(2 * time.Hour) // Europe/Istanbul saatinde "GG/AA/YYYY SS:DD" olarak gönderilir
res, err := client.Send(ctx, im.SendRequest{
	Messages: []im.Message{
		{Text: "Merhaba Ali", Recipients: []string{"5xxxxxxxxx"}},
		{Text: "Merhaba Ayşe", Recipients: []string{"5xxxxxxxxx"}},
	},
	SendAt:  &at,
	IYS:     im.IYSOn,        // ticari ileti ise yasal olarak açık olmalı
	IYSList: im.IYSBireysel, // IYSOn ise zorunlu: BIREYSEL veya TACIR
})
```

Alıcılar `NormalizeTR` ile normalleştirilir ve metin başına tekilleştirilir. `IYS` boşsa `IYSOff` kabul edilir (işlemsel/bilgilendirme SMS'leri için `iys: "0"`).

> **Not:** Planlı gönderim (`SendAt`) canlıda henüz doğrulanmadı. Anlık gönderimde `sendDateTime` boş dizi (`[]`, canlıda doğrulandı), planlı gönderimde tek string (`"GG/AA/YYYY SS:DD"`, resmi SDK ve dokümana göre) gönderilir.

### Rapor, iptal, bakiye, başlıklar

```go
rep, err := client.GetReport(ctx, res.OrderID, 1, 1000) // page, rowCount (0 = API varsayılanı; en çok 1000)
fmt.Println(rep.Status, rep.Counts())                   // 113 gönderiliyor / 114 tamamlandı / 115 gönderilemedi
// rep.Total, Delivered, Undelivered, Waiting, SubmitAt, SendAt (Europe/Istanbul), Sender, Price

err = client.Cancel(ctx, res.OrderID)                   // yalnızca gönderimi başlamamış ileri tarihli sipariş

bal, _ := client.Balance(ctx)                           // bal.Amount (TL), bal.SMS (kredi)
senders, _ := client.Senders(ctx)                       // onaylı başlıklar

orders, toplam, _ := client.Summaries(ctx, start, end, 1) // sipariş başına total/delivered/undelivered/waiting
```

Rapordaki durumlar: mesaj `110` gönderiliyor, `111` gönderildi, `112` gönderilemedi (`MessageSending/MessageSent/MessageFailed`).
Yanıtta sunucu sayaçları (`total`, `delivered`, `undelivered`, `waiting`) varsa `Report.Counts()` onları (tüm sipariş) tercih eder; yoksa yalnızca o sayfadaki mesajları sayar. Rapordaki numaralar `+90…` biçiminde gelebilir; `ReportMessage.NormalizedNumber()` ile `5xxxxxxxxx`'e çevirip gönderdiğiniz numaralarla eşleştirin.

`Summaries` için tarih aralığı en çok 10 gün olabilir (aşılırsa istek atmadan `ErrInvalidDate`; sunucuda 458).

## Hata yönetimi

```go
res, err := client.Send(ctx, req)
switch {
case err == nil:
	// başarılı
case errors.Is(err, im.ErrUncertain):
	// DİKKAT: SMS gitmiş olabilir de olmayabilir. Körlemesine tekrar denemeyin.
case errors.Is(err, im.ErrDuplicateOrder):
	// 451: aynı metin aynı alıcılara kısa süre önce gönderilmiş
case errors.Is(err, im.ErrInsufficientBalance), errors.Is(err, im.ErrUnauthorized):
	// 402 bakiye / 401 kimlik veya IP yetkisi
}

var apiErr *im.APIError
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.Code, apiErr.Message)
}
```

| Kod | Sentinel | Anlam |
|-----|----------|-------|
| – | `ErrHTTP` | API JSON zarfı yerine JSON olmayan HTTP hata yanıtı (WAF/proxy sayfası vb.); `APIError.Code == 0`, `HTTPStatus` dolu. 401/404 sentinel'lerine eşlenmez |
| 400 | `ErrBadRequest` | İstek çözümlenemedi |
| 401 | `ErrUnauthorized` | Üyelik bilgileri hatalı **veya sabit IP tanımlıysa farklı IP'den istek** |
| 402 | `ErrInsufficientBalance` | Bakiye yetersiz |
| 404 | `ErrMethodNotFound` | Yanlış adres/yöntem |
| 422 | `ErrValidation` | Değerler doğrulanamadı |
| 450 | `ErrSenderNotAllowed` | Başlık kullanıma uygun değil |
| 451 | `ErrDuplicateOrder` | Tekrar eden sipariş (aşağıya bakın) |
| 452, 466 | `ErrInvalidRecipients` | Alıcılar hatalı |
| 453 | `ErrMessageTooLong` | Sipariş boyutu aşıldı (7 SMS sınırı) |
| 454 | `ErrEmptyMessage` | Metin boş |
| 455 | `ErrOrderNotFound` | Sipariş yok (veya iptal için gönderimi başlamış) |
| 456 | `ErrNotYetDue` | İleri tarihli siparişin gönderim zamanı gelmedi |
| 457, 458 | `ErrInvalidDate` | Tarih biçimi/aralığı hatalı |
| 459, 460 | `ErrInvalidSender` | Başlık boş / 11 karakterden uzun / Türkçe karakter içeriyor |
| 468–470 | `ErrIYSConfig` | İYS ayarları eksik |
| 503 | `ErrServiceUnavailable` | Servis geçici kapalı |

Tanımlı olmayan kodlar da `*APIError` olarak döner (`Code` alanından okunur). İstek gönderilmeden yakalanan yerel doğrulama hataları (`ErrInvalidRequest`, `ErrInvalidRecipients`, `ErrEmptyMessage`, `ErrMessageTooLong`, `ErrInvalidSender`, `ErrIYSConfig`) aynı sentinel'lerle eşleşir ama `*APIError` değildir.

### Belirsiz sonuç: `ErrUncertain`

SMS gönderimi **idempotent değildir**. İstek sunucuya yazıldıktan sonra yanıt alınamazsa (zaman aşımı, bağlantı kopması, gövdesiz 5xx, çözümlenemeyen yanıt, `200` ama sipariş id'si yok) hata `ErrUncertain` ile eşleşir (altta yatan hata da `errors.Is` ile görülebilir, örn. `context.DeadlineExceeded`). Bu durumda SMS gitmiş olabilir; **otomatik tekrar denemeyin**. Önce `Summaries` ile (son gönderimlere bakarak) veya kendi kayıtlarınızla durumu doğrulayın, sonra karar verin.

İstek sunucuya hiç yazılamadıysa (DNS, bağlantı reddi, iptal edilmiş context) hata `ErrNotSent` ile eşleşir; SMS kesinlikle gitmemiştir, tekrar denemek güvenlidir.

**451 (`ErrDuplicateOrder`)** sunucunun tekrar koruması: aynı metin aynı alıcılara yaklaşık 10 dakika içinde yeniden gönderilmek istenirse reddedilir. Bu genellikle **ilk gönderimin zaten alındığı** anlamına gelir; bir `ErrUncertain` sonrası yeniden denemede 451 almak, ilk isteğin işlendiğinin güçlü bir işaretidir. Siparişi 451 sonrası farklı bir metinle zorla göndermeyin.

## Yardımcılar

```go
im.NormalizeTR("+90 (531) 234-56-78") // "5312345678", nil   (0, 90, +90, 0090 önekleri ve boşluk/tire/parantez temizlenir)
im.MaskNumber("5312345678")          // "531*****78"
im.EstimateParts("Merhaba şehir")    // SMS parça sayısı TAHMİNİ
```

**`EstimateParts` bir tahmindir.** Sağlayıcı tablosuna göre hesaplanır (karakter sayısı; tek parça / çok parçada parça başına):

| Metin | Tek | Çok parça |
|-------|-----|-----------|
| GSM karakter kümesi | 155 | 153 |
| GSM dışı Türkçe karakter içeren (ş, ğ, ı, İ, Ş, Ğ, ç) | 150 | 148 |
| Diğer Unicode (emoji vb.) | 65 | 63 |

Genişletme karakterleri (`^ { } \ [ ~ ] | €`) 2 birim sayılır. Paneldeki **"Evrensel Dil Desteği"** ayarı gerçek kodlamayı etkileyebilir; sonuç kesin değildir. `Send`, tahmini 7 parçayı aşan metni istek atmadan `ErrMessageTooLong` ile reddeder; sağlayıcınızın kuralı farklıysa `Config.SkipLengthCheck` ile kapatın.

## Entegrasyon notları

- **451 (tekrar eden sipariş):** Aynı başlık + aynı alıcı + aynı metin kısa sürede tekrar gönderilirse reddedilir. Aynı alıcıya art arda benzer mesajlar (ör. sipariş bildirimleri) gönderiyorsanız metne benzersiz bilgi (sipariş no, saat, kod) koyun.
- **Hız ve boyut sınırları:** Hesap başına yaklaşık 100 SMS/sn; istek başına en çok 50.000 alıcı. Kütüphane alıcı listesini **parçalamaz**, büyük listeleri kendiniz bölün.
- **Panel ayarları:** Panelde "API kullanımına izin ver" açık olmalıdır; sabit IP kısıtı tanımlıysa farklı IP'den gelen istek `401` döner.
- **Sandbox yok:** Test ortamı bulunmaz; `APITEST` gibi başlıklar yalnızca dokümandaki örneklerdir, gerçek hesapta onaylı başlığınızı kullanın. Canlı testlerde gerçek SMS ve bakiye harcanır.
- **İYS:** İşlemsel (transactional) SMS'lerde `iys "0"` (`IYSOff`, varsayılan); ticari ileti ise yasal olarak `IYSOn` olmalıdır.
- **Yönlendirme:** Varsayılan HTTP istemcisi yönlendirmeleri takip etmez; 3xx yanıtı hata sayılır (gönderimde `ErrUncertain`).

## Güvenlik notları

- Kütüphane **hiçbir zaman** tam telefon numarası, SMS metni, key veya hash loglamaz. `Logger` yalnızca uç nokta adı, süre, HTTP durum kodu, API durum kodu ve sipariş id'sini alır. Kendi loglarınızda numaraları `MaskNumber` ile maskeleyin.
- `Config` ve `*Client` yazdırıldığında (`%v`, `%+v`, `%#v`) ve `slog` ile loglandığında anahtarlar gizlenir.
- `send-sms` isteğinde HTTP/2 transport'unun gövdeyi kendiliğinden yeniden göndermesi engellenir (SMS idempotent değildir); böyle durumlar `ErrUncertain` olarak size döner.
- API anahtarlarını kaynak koda veya repoya koymayın; ortam değişkeni/secret yöneticisi kullanın. Panelde sabit IP tanımlarsanız yalnızca o IP'den istek kabul edilir (aksi halde `401`).
- Hata değerleri istek gövdesini (kimlik bilgisi, numara) içermez.

## Testler

```bash
go vet ./...
go test -race ./...
```

Canlı entegrasyon testi (opsiyonel) yalnızca ücretsiz uçları (`Balance`, `Senders`) çağırır, **asla SMS göndermez**:

```bash
ILETI_MERKEZI_KEY=... ILETI_MERKEZI_HASH=... go test -tags integration -run 'Balance|Senders' -v ./...
```

## Bilinen noktalar

- API'de alıcı alanı `receipents` (yanlış yazım) olarak geçer; kütüphane bunu kendisi yönetir.
- JSON yanıtlarında kodlar string veya sayı olarak gelebilir; kütüphane ikisini de kabul eder.
- `sendDateTime`: anlık gönderimde boş dizi, planlı gönderimde tek string (canlıda doğrulanmadı).

## Lisans

MIT, bkz. [LICENSE](LICENSE).
