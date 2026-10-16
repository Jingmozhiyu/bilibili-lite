package admission

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"bilibili-lite/internal/mall/conf"
	"github.com/go-kratos/kratos/v3/errors"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

func protection(t *testing.T) *Protection {
	t.Helper()
	addr := os.Getenv("MALL_TEST_REDIS_ADDRESS")
	if addr == "" {
		t.Skip("set MALL_TEST_REDIS_ADDRESS for real Redis tests")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, MaxRetries: -1})
	t.Cleanup(func() {
		client.Close()
	})
	if e := client.Ping(context.Background()).Err(); e != nil {
		t.Fatal(e)
	}
	return New(client, &conf.Config{
		RedisPrefix:      "test:" + uuid.NewString(),
		EntryRate:        1,
		EntryBurst:       1,
		InnerConcurrency: 2,
		IPRate:           1,
		IPBurst:          1,
	})
}

func TestSharedAccountAtomicityAndCooldown(t *testing.T) {
	p := protection(t)
	other := New(p.redis, p.config)
	c := context.Background()
	g := new(errgroup.Group)
	g.SetLimit(20)
	var allowed atomic.Int64
	for i := 0; i < 50; i++ {
		g.Go(func() error {
			target := p
			if i%2 == 0 {
				target = other
			}
			e := target.Check(c, 1, "bw")
			if e == nil {
				allowed.Add(1)
				return nil
			}
			if errors.FromError(e).Metadata["business_code"] != "900001" {
				return e
			}
			return nil
		})
	}
	if e := g.Wait(); e != nil {
		t.Fatal(e)
	}
	if allowed.Load() != 1 {
		t.Fatalf("admitted %d checks", allowed.Load())
	}
	if e := other.Check(c, 2, "bw"); e != nil {
		t.Fatal(e)
	}
	if e := other.Cooldown(c, 1, "bw", 2*time.Second); e != nil {
		t.Fatal(e)
	}
	before := p.redis.PTTL(c, p.key(1, "bw")).Val()
	for i := 0; i < 20; i++ {
		e := p.Check(c, 1, "bw")
		if errors.FromError(e).Metadata["business_code"] != "3" {
			t.Fatal(e)
		}
	}
	after := p.redis.PTTL(c, p.key(1, "bw")).Val()
	if after > before {
		t.Fatalf("blocked requests extended cooldown %v -> %v", before, after)
	}
	// Expiry uses actual Redis TTL and Redis server time, not an injected client clock.
	if e := p.redis.PExpire(c, p.key(1, "bw"), time.Millisecond).Err(); e != nil {
		t.Fatal(e)
	}
	time.Sleep(5 * time.Millisecond)
	if e := other.Check(c, 1, "bw"); e != nil {
		t.Fatal(e)
	}
}

func TestOuterAndBypassPaths(t *testing.T) {
	p := protection(t)
	called := 0
	h := p.Filter(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(204)
	}))
	req := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, path, nil)
		h.ServeHTTP(w, r)
		return w
	}
	if w := req("POST", "/api/mall/v1/orders"); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if w := req("POST", "/api/mall/v1/orders"); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("outer limit: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/mall/v1/orders/id/pay", "/api/mall/v1/orders/id/refund"} {
		if w := req("POST", path); w.Code != 204 {
			t.Fatal(w.Code)
		}
	}
	if called != 3 {
		t.Fatal(called)
	}
}

func TestInnerBoundAndRelease(t *testing.T) {
	p := New(nil, &conf.Config{InnerConcurrency: 2})
	a, ok := p.Acquire()
	if !ok {
		t.Fatal("first")
	}
	b, ok := p.Acquire()
	if !ok {
		t.Fatal("second")
	}
	if _, ok = p.Acquire(); ok {
		t.Fatal("over capacity")
	}
	a()
	c, ok := p.Acquire()
	if !ok {
		t.Fatal("not released")
	}
	b()
	c()
}

func TestRedisFailureClosed(t *testing.T) {
	p := protection(t)
	p.redis.Close()
	if e := p.Check(context.Background(), 1, "bw"); errors.Code(e) != 503 {
		t.Fatal(e)
	}
	h := p.Filter(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("allowed purchase with Redis down")
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/api/mall/v1/orders", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}

func TestIPLimitOptionalAndNoForwardedHeaderTrust(t *testing.T) {
	p := protection(t)
	p.config.EntryBurst = 100
	p.config.IPLimit = true
	h := p.Filter(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(204)
	}))
	for i := 0; i < 2; i++ {
		r := httptest.NewRequest("POST", "/api/mall/v1/orders", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		r.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i+2))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := 204
		if i == 1 {
			want = 429
		}
		if w.Code != want {
			t.Fatalf("%s: %d", strconv.Itoa(i), w.Code)
		}
	}
}
