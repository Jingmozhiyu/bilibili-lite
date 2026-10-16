// k6 run -e MALL_BASE_URL=http://127.0.0.1:8001 -e MALL_PRODUCT_ID=bw-2026 \
//   -e MALL_TICKET_TYPE_ID=bw-2026-1 -e MALL_TOKENS_FILE=/tmp/mall-tokens.json scripts/load/mall.js
import http from 'k6/http'
import exec from 'k6/execution'
import {Counter, Rate, Trend} from 'k6/metrics'

const tokens = JSON.parse(open(__ENV.MALL_TOKENS_FILE))
if (!Array.isArray(tokens) || !tokens.length || tokens.some((t) => typeof t !== 'string')) throw new Error('Provide a JSON array of test access tokens')
if (!__ENV.MALL_PRODUCT_ID || !__ENV.MALL_TICKET_TYPE_ID) throw new Error('Product and ticket type are required')
const outcomes = new Counter('mall_purchase_results')
const systemErrors = new Rate('mall_system_errors')
const successfulOrderLatency = new Trend('mall_successful_order_duration', true)
export const options = {
    scenarios: {
        purchase: {
            executor: 'constant-arrival-rate',
            rate: Number(__ENV.MALL_RATE || 50),
            timeUnit: '1s',
            duration: __ENV.MALL_DURATION || '30s',
            preAllocatedVUs: 20,
            maxVUs: 200
        }
    },
    thresholds: {mall_system_errors: ['rate<0.01']},
}
http.setResponseCallback(http.expectedStatuses(200, 409, 429))
export default function () {
    const iteration = exec.scenario.iterationInTest
    const token = tokens[iteration % tokens.length]
    const response = http.post(`${__ENV.MALL_BASE_URL || 'http://127.0.0.1:8001'}/api/mall/v1/orders`, JSON.stringify({
        productId: __ENV.MALL_PRODUCT_ID, ticketTypeId: __ENV.MALL_TICKET_TYPE_ID,
        requestId: `load-${exec.scenario.startTime}-${iteration}`,
    }), {
        headers: {Authorization: `Bearer ${token}`, 'Content-Type': 'application/json'},
        tags: {name: 'mall_create_order'}
    })
    let result = response.status === 200 ? 'success' : String(response.status)
    try {
        result = response.json('metadata.business_code') || result
    } catch { /* Non-JSON failures count as system errors below. */
    }
    outcomes.add(1, {result})
    systemErrors.add(![200, 409, 429].includes(response.status))
    if (response.status === 200) successfulOrderLatency.add(response.timings.duration)
}
