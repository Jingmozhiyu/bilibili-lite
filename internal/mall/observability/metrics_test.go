package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestFilterCountsRejectedPurchaseAndBoundsLabels(t *testing.T) {
	metrics := New()
	handler := metrics.Filter(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/mall/v1/orders", nil))
	if count := testutil.ToFloat64(metrics.Requests.WithLabelValues("purchase", "429")); count != 1 {
		t.Fatalf("outer rejection count = %v, want 1", count)
	}
	for _, path := range []string{"/orders/private-id-a", "/orders/private-id-b"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
	}
	if count := testutil.ToFloat64(metrics.Requests.WithLabelValues("other", "429")); count != 2 {
		t.Fatalf("unknown URLs must share a bounded label; count = %v", count)
	}
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
	if count := testutil.ToFloat64(metrics.Requests.WithLabelValues("other", "429")); count != 2 {
		t.Fatal("scrapes must not inflate business traffic")
	}
}
