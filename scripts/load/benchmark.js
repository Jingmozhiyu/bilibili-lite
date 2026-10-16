// Run only against scripts/load/compose.yml: these JWTs use an isolated test issuer.
import http from 'k6/http'
import crypto from 'k6/crypto'
import encoding from 'k6/encoding'
import exec from 'k6/execution'
import {Counter, Rate, Trend} from 'k6/metrics'

const mode = __ENV.MODE || 'purchase'
const resultCodes = ['success', '429', '412', '900001', '3', '100009', '100019', '219', '221', 'system']
const outcomes = new Counter('purchase_outcomes')
const systemErrors = new Rate('system_errors')
const orderLatency = new Trend('successful_order_ms', true)
const lifecycleLatency = new Trend('successful_lifecycle_ms', true)
const completed = new Counter('completed_payments')
const refunded = new Counter('completed_refunds')
const thresholds = {system_errors: ['rate<0.01'], dropped_iterations: ['count==0']}
for (const code of resultCodes) thresholds[`purchase_outcomes{result:${code}}`] = ['count>=0']
export const options = {
    scenarios: {
        buyers: {
            executor: 'constant-arrival-rate',
            rate: Number(__ENV.RATE || 50),
            timeUnit: '1s',
            duration: __ENV.DURATION || '30s',
            preAllocatedVUs: Number(__ENV.VUS || 100),
            maxVUs: Number(__ENV.MAX_VUS || 500),
            gracefulStop: '15s',
        },
    },
    summaryTrendStats: ['avg', 'med', 'p(95)', 'p(99)', 'max'],
    thresholds,
}
http.setResponseCallback(http.expectedStatuses(200, 409, 429))

function token(buyer) {
    const header = encoding.b64encode(JSON.stringify({alg: 'HS256', typ: 'JWT'}), 'rawurl')
    const payload = encoding.b64encode(JSON.stringify({
        iss: 'mall-benchmark', sub: String(buyer), token_type: 'access', exp: 4102444800,
    }), 'rawurl')
    const content = `${header}.${payload}`
    const signature = crypto.hmac('sha256', 'benchmark-only-secret-never-use-in-normal-environments', content, 'base64rawurl')
    return `${content}.${signature}`
}

function responseBody(response) {
    try {
        return response.json()
    } catch {
        return {}
    }
}

export default function () {
    const iteration = exec.scenario.iterationInTest
    const buyer = __ENV.ACCOUNTS ? iteration % Number(__ENV.ACCOUNTS) + 1 : iteration + 1
    const headers = {Authorization: `Bearer ${token(buyer)}`, 'Content-Type': 'application/json'}
    const start = Date.now()
    const response = http.post(`${__ENV.MALL_BASE_URL}/api/mall/v1/orders`, JSON.stringify({
        productId: 'bench-event', ticketTypeId: `bench-tier-${iteration % Number(__ENV.TIERS || 1)}`,
        requestId: `bench-${__ENV.RUN_ID}-${iteration}`,
    }), {headers, tags: {name: 'create_order'}, timeout: '10s'})
    const body = responseBody(response)
    const success = response.status === 200 && body.id && body.state === 'pending'
    const code = success ? 'success' : body.metadata?.business_code || 'system'
    outcomes.add(1, {result: code})
    systemErrors.add(!success && !resultCodes.slice(1, -1).includes(code))
    if (!success) return
    orderLatency.add(response.timings.duration)
    if (mode !== 'lifecycle') return

    const payment = http.post(`${__ENV.MALL_BASE_URL}/api/mall/v1/orders/${body.id}/pay`, '{}', {
        headers, tags: {name: 'pay_order'}, timeout: '10s',
    })
    const paid = payment.status === 200 && responseBody(payment).state === 'paid'
    systemErrors.add(!paid)
    if (!paid) return
    completed.add(1)
    lifecycleLatency.add(Date.now() - start)
    if (iteration % 10 === 0) {
        const refund = http.post(`${__ENV.MALL_BASE_URL}/api/mall/v1/orders/${body.id}/refund`, '{}', {
            headers, tags: {name: 'refund_order'}, timeout: '10s',
        })
        const ok = refund.status === 200 && responseBody(refund).state === 'refunded'
        systemErrors.add(!ok)
        if (ok) refunded.add(1)
    }
}

export function handleSummary(data) {
    return {[`/results/${__ENV.RUN_ID}/summary.json`]: JSON.stringify(data, null, 2)}
}
