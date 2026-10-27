package iletimerkezi

import "unicode/utf16"

// MaxParts, tek bir SMS metni için API'nin kabul ettiği en çok parça sayısıdır.
const MaxParts = 7

// Sağlayıcı tablosu (karakter sayısı; tek parça / çok parçada parça başına):
//
//	GSM karakter kümesi          : 155 / 153
//	Türkçe karakterli (ş ğ ı İ …): 150 / 148
//	Diğer Unicode                : 65 / 63
const (
	gsmSingle, gsmMulti         = 155, 153
	turkishSingle, turkishMulti = 150, 148
	unicodeSingle, unicodeMulti = 65, 63
)

// gsmBasic, GSM 03.38 temel karakter kümesidir (1 birim).
const gsmBasic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

// gsmExt, 2 birim tutan genişletme karakterleridir.
const gsmExt = "^{}\\[~]|€\f"

// turkishOnly, GSM kümesinde olmayan Türkçe karakterlerdir (ç, ö, ü, Ç, Ö, Ü zaten GSM'dedir).
const turkishOnly = "şŞğĞıİç"

var gsmBasicSet, gsmExtSet, turkishSet = func() (map[rune]bool, map[rune]bool, map[rune]bool) {
	b, e, tr := map[rune]bool{}, map[rune]bool{}, map[rune]bool{}
	for _, r := range gsmBasic {
		b[r] = true
	}
	for _, r := range gsmExt {
		e[r] = true
	}
	for _, r := range turkishOnly {
		tr[r] = true
	}
	return b, e, tr
}()

func ceilDiv(n, d int) int { return (n + d - 1) / d }

// EstimateParts, metnin kaç SMS parçasına bölüneceğini TAHMİN eder.
//
// Metin tamamen GSM karakterlerinden oluşuyorsa 155 (çok parçada 153), GSM dışı
// Türkçe karakter (ş, ğ, ı, İ, Ş, Ğ, ç) içeriyorsa 150 (çok parçada 148), başka
// Unicode karakter (emoji vb.) içeriyorsa 65 (çok parçada 63) karakter üzerinden
// hesaplanır. Genişletme karakterleri (^ { } \ [ ~ ] | €) 2 birim sayılır.
// Gerçek kodlama panel ayarlarına (ör. "Evrensel Dil Desteği") ve operatöre göre
// değişebilir; sonuç kesin değil, yaklaşık değerdir. Boş metin için 0 döner.
func EstimateParts(text string) int {
	if text == "" {
		return 0
	}
	units, hasTurkish := 0, false
	for _, r := range text {
		switch {
		case gsmBasicSet[r]:
			units++
		case gsmExtSet[r]:
			units += 2
		case turkishSet[r]:
			units++
			hasTurkish = true
		default:
			n := len(utf16.Encode([]rune(text)))
			if n <= unicodeSingle {
				return 1
			}
			return ceilDiv(n, unicodeMulti)
		}
	}
	single, multi := gsmSingle, gsmMulti
	if hasTurkish {
		single, multi = turkishSingle, turkishMulti
	}
	if units <= single {
		return 1
	}
	return ceilDiv(units, multi)
}
