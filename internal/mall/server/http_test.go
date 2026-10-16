package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bilibili-lite/internal/mall/admission"
	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/mall/conf"
	"bilibili-lite/internal/mall/observability"
	"bilibili-lite/internal/mall/service"
	"bilibili-lite/internal/platform/auth"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"
)

type fakeUsecase struct {
	service.MallUsecase
	product biz.Product
}

func (f *fakeUsecase) CreateProduct(_ context.Context, p biz.Product) (*biz.Product, error) {
	f.product = p
	p.ID = "product"
	return &p, nil
}

func TestProtobufJSONOverHTTP(t *testing.T) {
	f := &fakeUsecase{}
	tokens, e := auth.NewJWTManager(&auth.Config{
		Issuer:     "test",
		Secret:     strings.Repeat("x", 32),
		AccessTTL:  time.Hour,
		RefreshTTL: 24 * time.Hour,
	})
	if e != nil {
		t.Fatal(e)
	}
	pair, e := tokens.Issue(1, true)
	if e != nil {
		t.Fatal(e)
	}
	srv := NewHTTPServer(&conf.Config{}, auth.NewAuthenticator(tokens), service.NewMallService(f), nil, observability.New())
	req := httptest.NewRequest("POST", "/api/mall/v1/products", strings.NewReader(`{"name":"BW","saleStartsAt":"2026-01-01T00:00:00Z","ticketTypes":[{"name":"普通票","priceCents":"12800","available":"100"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("HTTP %d: %s", w.Code, w.Body.String())
	}
	if len(f.product.TicketTypes) != 1 || f.product.TicketTypes[0].PriceCents != 12800 || f.product.SaleStartsAt.Year() != 2026 {
		t.Fatalf("decoded incorrectly: %+v", f.product)
	}
	var body map[string]any
	if e = json.Unmarshal(w.Body.Bytes(), &body); e != nil {
		t.Fatal(e)
	}
	if body["saleStartsAt"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("wrong timestamp response: %s", w.Body.String())
	}
}

func TestMetricsWrapOuterProtection(t *testing.T) {
	// A closed client deterministically fails before decoding or authentication.
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	_ = client.Close()
	config := &conf.Config{InnerConcurrency: 1}
	metrics := observability.New()
	tokens, err := auth.NewJWTManager(&auth.Config{Issuer: "test", Secret: strings.Repeat("x", 32), AccessTTL: time.Hour, RefreshTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	server := NewHTTPServer(config, auth.NewAuthenticator(tokens), service.NewMallService(&fakeUsecase{}), admission.New(client, config), metrics)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest("POST", "/api/mall/v1/orders", nil))
	if response.Code != 503 {
		t.Fatalf("status = %d, want 503", response.Code)
	}
	if count := testutil.ToFloat64(metrics.Requests.WithLabelValues("purchase", "503")); count != 1 {
		t.Fatalf("outer failure escaped HTTP instrumentation: count=%v", count)
	}
	scrape := httptest.NewRecorder()
	server.ServeHTTP(scrape, httptest.NewRequest("GET", "/metrics", nil))
	if scrape.Code != 200 || !strings.Contains(scrape.Body.String(), "mall_admission_total") || !strings.Contains(scrape.Body.String(), "mall_http_requests_total") {
		t.Fatalf("incomplete scrape: %s", scrape.Body.String())
	}
}
