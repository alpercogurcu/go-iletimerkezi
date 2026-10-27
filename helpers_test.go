package iletimerkezi

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeTR(t *testing.T) {
	ok := map[string]string{
		"5312345678":          "5312345678",
		"05312345678":         "5312345678",
		"905312345678":        "5312345678",
		"+905312345678":       "5312345678",
		"0090 531 234 56 78":  "5312345678",
		"+90 (531) 234-56-78": "5312345678",
		" 0531.234.56.78 ":    "5312345678",
	}
	for in, want := range ok {
		got, err := NormalizeTR(in)
		if err != nil || got != want {
			t.Errorf("%q → %q, %v (beklenen %q)", in, got, err, want)
		}
	}
	for _, in := range []string{"", "abc", "4312345678", "531234567", "53123456789", "+44 7911 123456", "02121234567", "5312345a78", "905312345"} {
		_, err := NormalizeTR(in)
		if !errors.Is(err, ErrInvalidRecipients) {
			t.Errorf("%q geçersiz olmalı, err=%v", in, err)
		}
		if err != nil && in != "" && strings.Contains(err.Error(), in) {
			t.Errorf("hata metni numarayı içeriyor: %v", err)
		}
	}
}

func TestMaskNumber(t *testing.T) {
	cases := map[string]string{
		"5312345678":   "531*****78",
		"905312345678": "905*******78",
		"12345":        "*****",
		"":             "",
	}
	for in, want := range cases {
		if got := MaskNumber(in); got != want {
			t.Errorf("%q → %q (beklenen %q)", in, got, want)
		}
	}
}

func TestEstimateParts(t *testing.T) {
	r := strings.Repeat
	cases := []struct {
		name string
		text string
		want int
	}{
		{"boş", "", 0},
		{"gsm 155", r("a", 155), 1},
		{"gsm 156", r("a", 156), 2},
		{"gsm 306", r("a", 306), 2},
		{"gsm 307", r("a", 307), 3},
		{"genişletme karakteri 2 birim", r("a", 154) + "€", 2},
		{"gsm içi türkçe ü ö Ü Ç", "üöÜÇ", 1},
		{"türkçe ş 150", r("ş", 150), 1},
		{"türkçe ş 151", r("ş", 151), 2},
		{"türkçe 296", r("ş", 296), 2},
		{"türkçe 297", r("ş", 297), 3},
		{"türkçe karma gsm+ş 150", r("a", 149) + "ş", 1},
		{"unicode emoji 65", r("😀", 32) + "a", 1}, // 65 UTF-16 birimi
		{"unicode emoji 66", r("😀", 33), 2},       // 66 birim
		{"unicode 126", r("😀", 63), 2},            // 126 birim
		{"unicode 127", r("😀", 63) + "a", 3},
		{"unicode+türkçe karışık unicode", "şğ😀", 1},
		{"7 parça gsm", r("a", 7*153), 7},
		{"8 parça gsm", r("a", 7*153+1), 8},
	}
	for _, tc := range cases {
		if got := EstimateParts(tc.text); got != tc.want {
			t.Errorf("%s: %d (beklenen %d)", tc.name, got, tc.want)
		}
	}
}
