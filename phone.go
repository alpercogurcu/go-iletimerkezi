package iletimerkezi

import (
	"fmt"
	"strings"
)

// NormalizeTR, Türkiye cep telefonu numarasını API'nin kabul ettiği "5xxxxxxxxx"
// (10 hane, 5 ile başlar) biçimine çevirir. +90, 90, 0090 ve 0 öneklerini; boşluk,
// tire, nokta ve parantezleri temizler. Geçersizse ErrInvalidRecipients ile
// eşleşen bir hata döner (hata metni numarayı içermez).
func NormalizeTR(s string) (string, error) {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && i == 0:
		case r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' || r == '\t':
		default:
			return "", fmt.Errorf("%w: numarada geçersiz karakter", ErrInvalidRecipients)
		}
	}
	d := b.String()
	switch {
	case len(d) == 14 && strings.HasPrefix(d, "0090"):
		d = d[4:]
	case len(d) == 12 && strings.HasPrefix(d, "90"):
		d = d[2:]
	case len(d) == 11 && strings.HasPrefix(d, "0"):
		d = d[1:]
	}
	if len(d) != 10 || d[0] != '5' {
		return "", fmt.Errorf("%w: geçerli bir Türkiye cep numarası değil", ErrInvalidRecipients)
	}
	return d, nil
}

// MaskNumber, numarayı loglamaya uygun biçimde maskeler: "5312345678" → "531*****78".
// Kısa değerler tamamen maskelenir.
func MaskNumber(n string) string {
	r := []rune(n)
	if len(r) < 6 {
		return strings.Repeat("*", len(r))
	}
	for i := 3; i < len(r)-2; i++ {
		r[i] = '*'
	}
	return string(r)
}
