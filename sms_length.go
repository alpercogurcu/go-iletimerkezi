package iletimerkezi

import "unicode/utf16"

// MaxParts, tek bir SMS metni için API'nin kabul ettiği en çok parça sayısıdır.
const MaxParts = 7

// gsmBasic, GSM 03.38 temel karakter kümesidir (1 septet). Türkçe'de yalnızca
// Ç, Ü, Ö, ü, ö bu kümededir; ş, Ş, ğ, Ğ, ı, İ, ç değildir.
const gsmBasic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"

// gsmExt, 2 septet tutan genişletme karakterleridir.
const gsmExt = "^{}\\[~]|€\f"

var gsmBasicSet, gsmExtSet = func() (map[rune]bool, map[rune]bool) {
	b, e := map[rune]bool{}, map[rune]bool{}
	for _, r := range gsmBasic {
		b[r] = true
	}
	for _, r := range gsmExt {
		e[r] = true
	}
	return b, e
}()

// EstimateParts, metnin kaç SMS parçasına bölüneceğini TAHMİN eder.
//
// Tüm karakterler GSM-7 kümesindeyse tek parça 160, çok parçada parça başına 153
// septet; aksi halde (ş, ğ, ı, İ, ç, emoji vb.) UCS-2 varsayılır: tek parça 70, çok
// parçada parça başına 67 karakter. Sağlayıcının operatör tarafı kodlaması (örn.
// Türkçe kaydırma tablosu) farklı olabilir; sonuç kesin değil, yaklaşık değerdir.
// Boş metin için 0 döner.
func EstimateParts(text string) int {
	if text == "" {
		return 0
	}
	septets, gsm := 0, true
	for _, r := range text {
		switch {
		case gsmBasicSet[r]:
			septets++
		case gsmExtSet[r]:
			septets += 2
		default:
			gsm = false
		}
		if !gsm {
			break
		}
	}
	if gsm {
		if septets <= 160 {
			return 1
		}
		return (septets + 152) / 153
	}
	units := len(utf16.Encode([]rune(text)))
	if units <= 70 {
		return 1
	}
	return (units + 66) / 67
}
