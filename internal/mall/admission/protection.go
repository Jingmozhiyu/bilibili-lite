package admission

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"bilibili-lite/internal/mall/biz"
	"bilibili-lite/internal/mall/conf"

	"github.com/go-kratos/kratos/v3/errors"
	kratosHTTP "github.com/go-kratos/kratos/v3/transport/http"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

var (
	admissionDescriptor = prometheus.NewDesc("mall_admission_total", "Admission decisions, not committed orders.", []string{"layer", "result"}, nil)
	inflightDescriptor  = prometheus.NewDesc("mall_purchase_inflight", "Active purchase transaction slots.", nil, nil)
	capacityDescriptor  = prometheus.NewDesc("mall_purchase_capacity", "Maximum concurrent purchase transaction slots.", nil, nil)
)

// bucketScript atomically refills and consumes a shared token bucket.
// Redis time keeps instances on the same clock; the result is retry delay in milliseconds.
var bucketScript = redis.NewScript(`
local time = redis.call('TIME')
local now = time[1] * 1000 + math.floor(time[2] / 1000)
local state = redis.call('HMGET', KEYS[1], 'tokens', 'at')
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local tokens = tonumber(state[1]) or burst
local at = tonumber(state[2]) or now

tokens = math.min(burst, tokens + math.max(0, now - at) * rate / 1000)
local retry = 0
if tokens >= 1 then
    tokens = tokens - 1
else
    retry = math.ceil((1 - tokens) * 1000 / rate)
end

redis.call('HSET', KEYS[1], 'tokens', tokens, 'at', now)
redis.call('PEXPIRE', KEYS[1], math.ceil(burst / rate * 1000) + 1000)
return retry
`)

// accountScript checks cooldown before the one-second eligibility-check window.
// Rejected requests return without writing, so repeated clicks cannot renew a ban.
var accountScript = redis.NewScript(`
local time = redis.call('TIME')
local now = time[1] * 1000 + math.floor(time[2] / 1000)
local state = redis.call('HMGET', KEYS[1], 'cooldown', 'next')
local cooldown = tonumber(state[1]) or 0
local next = tonumber(state[2]) or 0

if cooldown > now then
    return {3, cooldown - now}
end
if next > now then
    return {900001, next - now}
end

redis.call('HSET', KEYS[1], 'next', now + 1000)
redis.call('PEXPIRE', KEYS[1], 1000)
return {0, 0}
`)

// cooldownScript preserves the longer deadline when concurrent failures set different cooldowns.
var cooldownScript = redis.NewScript(`
local time = redis.call('TIME')
local now = time[1] * 1000 + math.floor(time[2] / 1000)
local previous = tonumber(redis.call('HGET', KEYS[1], 'cooldown')) or 0
local untilAt = math.max(previous, now + tonumber(ARGV[1]))

redis.call('HSET', KEYS[1], 'cooldown', untilAt)
redis.call('PEXPIRE', KEYS[1], untilAt - now + 1000)
return 1
`)

// Protection owns atomic shared Redis limits and a per-process transaction bulkhead.
// Redis availability is required for NEW purchases only, never for payment/query/refund.
type Protection struct {
	redis  *redis.Client
	config *conf.Config
	slots  chan struct{}
	mu     sync.Mutex
	counts map[string]uint64
}

func New(client *redis.Client, config *conf.Config) *Protection {
	return &Protection{
		redis:  client,
		config: config,
		slots:  make(chan struct{}, config.InnerConcurrency),
		counts: make(map[string]uint64),
	}
}

// Filter applies outer limits only to new purchases, leaving order recovery and payment reachable.
func (p *Protection) Filter(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/mall/v1/orders" {
			next.ServeHTTP(w, r)
			return
		}

		// Run before JSON decoding/JWT verification. Bound Redis wait even for slow clients.
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		retry, err := p.bucket(ctx, "entry", p.config.EntryRate, p.config.EntryBurst)
		if err != nil {
			kratosHTTP.DefaultErrorEncoder(w, r, p.unavailable("outer"))
			return
		}
		if retry > 0 {
			p.observe("outer", "429")
			writeError(w, r, biz.PurchaseError(429, "429", "当前购票人数较多，请稍后再试", retry))
			return
		}
		if p.config.IPLimit {
			// Deliberately do not trust X-Forwarded-For supplied by arbitrary clients.
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			retry, err = p.bucket(ctx, fmt.Sprintf("ip:%x", sha256.Sum256([]byte(ip))), p.config.IPRate, p.config.IPBurst)
			if err != nil {
				kratosHTTP.DefaultErrorEncoder(w, r, p.unavailable("outer"))
				return
			}
			if retry > 0 {
				p.observe("outer", "412")
				writeError(w, r, biz.PurchaseError(429, "412", "请求过于频繁，请稍后再试", retry))
				return
			}
		}
		p.observe("outer", "admitted")
		next.ServeHTTP(w, r)
	})
}

func (p *Protection) Check(ctx context.Context, buyerID uint64, product string) error {
	values, err := accountScript.Run(ctx, p.redis, []string{p.key(buyerID, product)}).Int64Slice()
	if err != nil {
		return p.unavailable("middle")
	}
	if values[0] != 0 {
		code := strconv.FormatInt(values[0], 10)
		p.observe("middle", code)
		message := "操作太快，请稍后再试"
		if values[0] == 3 {
			message = "抢票冷却中，请稍后再试"
		}
		return biz.PurchaseError(429, code, message, values[1])
	}
	p.observe("middle", "admitted")
	return nil
}

func (p *Protection) Cooldown(ctx context.Context, buyerID uint64, product string, d time.Duration) error {
	if _, err := cooldownScript.Run(ctx, p.redis, []string{p.key(buyerID, product)}, d.Milliseconds()).Result(); err != nil {
		return p.unavailable("middle")
	}
	if d == 2*time.Second {
		p.observe("inner", "219")
	}
	return nil
}

// Acquire reserves a per-instance transaction slot without waiting. The caller must defer release.
// 尝试占用当前服务实例的一个交易处理名额；满了就立即失败，不排队。成功后，调用方必须用 defer 保证退出时归还名额。
func (p *Protection) Acquire() (func(), bool) {
	select {
	case p.slots <- struct{}{}:
		p.observe("inner", "admitted")
		return func() {
			<-p.slots
		}, true
	default:
		p.observe("inner", "221")
		return nil, false
	}
}

// ErrorEncoder keeps HTTP statuses valid while exposing the agreed business code.
func ErrorEncoder(w http.ResponseWriter, r *http.Request, err error) {
	writeError(w, r, err)
}

// Describe and Collect preserve the existing metric names used by load reports.
func (p *Protection) Describe(ch chan<- *prometheus.Desc) {
	ch <- admissionDescriptor
	ch <- inflightDescriptor
	ch <- capacityDescriptor
}

func (p *Protection) Collect(ch chan<- prometheus.Metric) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, count := range p.counts {
		labels := strings.SplitN(key, "/", 2)
		ch <- prometheus.MustNewConstMetric(admissionDescriptor, prometheus.CounterValue, float64(count), labels...)
	}
	ch <- prometheus.MustNewConstMetric(inflightDescriptor, prometheus.GaugeValue, float64(len(p.slots)))
	ch <- prometheus.MustNewConstMetric(capacityDescriptor, prometheus.GaugeValue, float64(cap(p.slots)))
}

func (p *Protection) ObserveResult(result string) {
	p.observe("purchase", result)
}

func (p *Protection) observe(layer, result string) {
	p.mu.Lock()
	p.counts[layer+"/"+result]++
	p.mu.Unlock()
}

func (p *Protection) unavailable(layer string) error {
	p.observe(layer, "unavailable")
	return errors.ServiceUnavailable("MALL_ADMISSION_UNAVAILABLE", "抢票服务暂不可用，请稍后重试")
}

func (p *Protection) key(buyerID uint64, product string) string {
	return fmt.Sprintf("%s:account:%d:%x", p.config.RedisPrefix, buyerID, sha256.Sum256([]byte(product)))
}

func (p *Protection) bucket(ctx context.Context, key string, rate, burst int) (int64, error) {
	return bucketScript.Run(ctx, p.redis, []string{p.config.RedisPrefix + ":" + key}, rate, burst).Int64()
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	ke := errors.FromError(err)
	if n, err := strconv.ParseInt(ke.Metadata["retry_after_ms"], 10, 64); err == nil && n > 0 {
		w.Header().Set("Retry-After", strconv.FormatInt(int64(math.Ceil(float64(n)/1000)), 10))
	}
	kratosHTTP.DefaultErrorEncoder(w, r, err)
}
