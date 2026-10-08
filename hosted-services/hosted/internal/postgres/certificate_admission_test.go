package postgres_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"portico.local/hosted/internal/certificates"
)

func TestCertificateOrderBudgetIsSharedAndNeverExceeded(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	const perMinute = 5
	var mu sync.Mutex
	admitted, refused := 0, 0
	var wg sync.WaitGroup
	// Several Hosted workers racing for the same minute must never admit more than the budget.
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wait, e := store.AdmitCertificateCreation(ctx, certificates.Order{ID: fmt.Sprintf("cert_%d", i), Issuer: "google-production"}, perMinute)
			if e != nil {
				t.Error(e)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if wait > 0 {
				refused++
				if wait > time.Minute+time.Second {
					t.Error("a refused order was told to wait longer than the window", wait)
				}
			} else {
				admitted++
			}
		}()
	}
	wg.Wait()
	if admitted != perMinute || refused != 24-perMinute {
		t.Fatal("admitted", admitted, "refused", refused)
	}
	// Another issuer has its own budget.
	if wait, e := store.AdmitCertificateCreation(ctx, certificates.Order{ID: "other", Issuer: "letsencrypt-production"}, perMinute); e != nil || wait != 0 {
		t.Fatal(wait, e)
	}
	// The window reopens as admissions age out of the last minute.
	if _, e := store.DB.Exec(`UPDATE certificate_order_admissions SET admitted_at=admitted_at-interval '61 seconds' WHERE issuer='google-production'`); e != nil {
		t.Fatal(e)
	}
	if wait, e := store.AdmitCertificateCreation(ctx, certificates.Order{ID: "later", Issuer: "google-production"}, perMinute); e != nil || wait != 0 {
		t.Fatal("the budget did not reopen", wait, e)
	}
}
