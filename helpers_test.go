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
		{"gsm 160", r("a", 160), 1},
		{"gsm 161", r("a", 161), 2},
		{"gsm 306", r("a", 306), 2},
		{"gsm 307", r("a", 307), 3},
		{"genişletme karakteri 2 septet", r("a", 159) + "€", 2},
		{"türkçe ü ö Ü (gsm)", "üöÜÇ", 1},
		{"türkçe ş ucs2 70", r("ş", 70), 1},
		{"türkçe ş ucs2 71", r("ş", 71), 2},
		{"ucs2 134", r("ş", 134), 2},
		{"ucs2 135", r("ş", 135), 3},
		{"emoji 2 birim", r("😀", 36), 2}, // 72 UTF-16 birimi
		{"7 parça gsm", r("a", 1071), 7},
		{"8 parça gsm", r("a", 1072), 8},
	}
	for _, tc := range cases {
		if got := EstimateParts(tc.text); got != tc.want {
			t.Errorf("%s: %d (beklenen %d)", tc.name, got, tc.want)
		}
	}
}
