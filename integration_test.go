//go:build integration

package iletimerkezi

import (
	"context"
	"os"
	"testing"
	"time"
)

// Canlı entegrasyon testi: yalnızca ücretsiz uçları (Balance, Senders) çağırır,
// ASLA Send çağırmaz. Çalıştırma:
//
//	ILETI_MERKEZI_KEY=... ILETI_MERKEZI_HASH=... go test -tags integration -run 'Balance|Senders' -v ./...
func liveClient(t *testing.T) *Client {
	t.Helper()
	key, hash := os.Getenv("ILETI_MERKEZI_KEY"), os.Getenv("ILETI_MERKEZI_HASH")
	if key == "" || hash == "" {
		t.Skip("ILETI_MERKEZI_KEY / ILETI_MERKEZI_HASH tanımlı değil")
	}
	c, err := NewClient(Config{APIKey: key, APIHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLiveBalance(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b, err := c.Balance(ctx)
	if err != nil {
		t.Fatalf("Balance: %v", err)
	}
	t.Logf("bakiye: TL=%.2f SMS=%d", b.Amount, b.SMS)
}

func TestLiveSenders(t *testing.T) {
	c := liveClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, err := c.Senders(ctx)
	if err != nil {
		t.Fatalf("Senders: %v", err)
	}
	t.Logf("başlıklar: %v", s)
}
