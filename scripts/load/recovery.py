#!/usr/bin/env python3
"""HTTP recovery probes against the isolated benchmark only.
Expiry is accelerated by SQL and is explicitly not a wall-clock 10-minute test.
"""
import base64
from concurrent.futures import ThreadPoolExecutor
import hashlib
import hmac
import json
import subprocess
import time
import urllib.error
import urllib.request

from run import ROOT, COMPOSE, sql

BASE = 'http://127.0.0.1:28001/api/mall/v1'
SECRET = b'benchmark-only-secret-never-use-in-normal-environments'

def token(buyer, admin=False):
    def encode(value):
        return base64.urlsafe_b64encode(json.dumps(value).encode()).decode().rstrip('=')
    payload = {'iss': 'mall-benchmark', 'sub': str(buyer), 'token_type': 'access', 'exp': 4102444800, 'is_admin': admin}
    content = encode({'alg': 'HS256', 'typ': 'JWT'}) + '.' + encode(payload)
    signature = base64.urlsafe_b64encode(hmac.new(SECRET, content.encode(), hashlib.sha256).digest()).decode().rstrip('=')
    return content + '.' + signature

def request(path, buyer, body=None, admin=False):
    req = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
        headers={'Authorization': 'Bearer ' + token(buyer, admin), 'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return response.status, json.load(response)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)

def wait_order(order_id, buyer, expected):
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        status, body = request('/orders/' + order_id, buyer)
        if status == 200 and body['state'] == expected:
            return body
        time.sleep(0.2)
    raise AssertionError((expected, status, body))

def wait_ready():
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        try:
            status, _ = request('/products', 900000)
            if status == 200:
                return
        except OSError:
            pass
        time.sleep(0.2)
    raise AssertionError('Benchmark mall did not become ready after restart')

def main():
    results = {}
    status, product = request('/products', 900000, {'name': 'Recovery probe', 'saleStartsAt': '2020-01-01T00:00:00Z',
        'ticketTypes': [{'name': name, 'priceCents': '12800', 'available': '20'} for name in ['normal', 'vip']]}, True)
    assert status == 200, (status, product)
    tiers = product['ticketTypes']
    def buy(buyer, request_id, tier=0):
        return request('/orders', buyer, {'productId': product['id'], 'ticketTypeId': tiers[tier]['id'], 'requestId': request_id})
    # Same account concurrently chooses two tiers; only one order may be committed.
    with ThreadPoolExecutor(max_workers=20) as pool:
        responses = list(pool.map(lambda i: buy(900001, f'cross-tier-{i}', i % 2), range(20)))
    successes = [body for status, body in responses if status == 200]
    assert len(successes) == 1, responses
    results['cross_tier_20_requests_one_order'] = True
    time.sleep(1.1)
    status, body = buy(900001, 'new-attempt')
    assert body.get('metadata', {}).get('business_code') == '100019', body
    results['new_request_blocked_by_existing_order'] = True

    status, pending = buy(900002, 'stable-key')
    assert status == 200, pending
    time.sleep(1.1)
    status, repeated = buy(900002, 'stable-key')
    assert status == 200 and repeated['id'] == pending['id'], repeated
    results['same_key_same_order'] = True
    # Test-fixture time travel, preserving the production 10-minute configuration.
    sql(f"UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE id='{pending['id']}';")
    wait_order(pending['id'], 900002, 'closed')
    time.sleep(1.1)
    status, closed = buy(900002, 'stable-key')
    assert status == 200 and closed['state'] == 'closed', closed
    time.sleep(1.1)
    status, replacement = buy(900002, 'replacement-key', 1)
    assert status == 200 and replacement['id'] != pending['id'], replacement
    results['accelerated_expiry_releases_claim_and_allows_new_key'] = True

    # A real Redis interruption must block new orders without blocking query/payment/refund.
    subprocess.run(COMPOSE + ['stop', 'redis'], check=True, capture_output=True)
    try:
        status, body = buy(900003, 'during-outage')
        assert status == 503, (status, body)
        status, body = request('/orders/' + replacement['id'], 900002)
        assert status == 200, body
        status, body = request('/orders/' + replacement['id'] + '/pay', 900002, {})
        assert status == 200 and body['state'] == 'paid', body
        status, body = request('/orders/' + replacement['id'] + '/refund', 900002, {})
        assert status == 200 and body['state'] == 'refunded', body
        results['redis_outage_new_order_503_query_pay_refund_available'] = True
    finally:
        subprocess.run(COMPOSE + ['start', 'redis'], check=True, capture_output=True)
    time.sleep(2)
    status, body = buy(900002, 'after-refund')
    assert body.get('metadata', {}).get('business_code') == '100019', body
    results['refund_blocks_repurchase'] = True
    # Real process restart must retain pending orders and restore worker operation.
    subprocess.run(COMPOSE + ['restart', 'mall'], check=True, capture_output=True)
    wait_ready()
    status, restored = request('/orders/' + successes[0]['id'], 900001)
    assert status == 200 and restored['state'] == 'pending', restored
    status, paid = request('/orders/' + restored['id'] + '/pay', 900001, {})
    assert status == 200 and paid['state'] == 'paid', paid
    results['process_restart_preserves_pending_order_and_payment'] = True
    # Reproduce durable states left by interruptions. These are explicit SQL fault
    # fixtures, not claims that an external provider was killed at an exact instruction.
    status, interrupted = buy(900004, 'interrupted-payment')
    assert status == 200, interrupted
    status, late = buy(900005, 'late-payment')
    assert status == 200, late
    subprocess.run(COMPOSE + ['stop', 'mall'], check=True, capture_output=True)
    try:
        sql(f"UPDATE orders SET payment_state='processing' WHERE id IN ('{interrupted['id']}','{late['id']}');")
        sql(f"INSERT INTO simulated_payments(order_id,reference,amount) SELECT id,'fault:'||id,price_cents FROM orders WHERE id IN ('{interrupted['id']}','{late['id']}');")
        sql(f"UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE id='{late['id']}';")
    finally:
        subprocess.run(COMPOSE + ['start', 'mall'], check=True, capture_output=True)
    wait_ready()
    wait_order(interrupted['id'], 900004, 'paid')
    deadline = time.monotonic() + 15
    while time.monotonic() < deadline:
        status, compensated = request('/orders/' + late['id'], 900005)
        if status == 200 and compensated['state'] == 'closed' and compensated['paymentState'] == 'refunded':
            break
        time.sleep(0.2)
    else:
        raise AssertionError(compensated)
    results['injected_charge_before_order_commit_reconciled'] = True
    results['injected_late_charge_after_expiry_refunded'] = True

    audit = json.loads(sql((ROOT / 'scripts/load/audit.sql').read_text()))
    assert all(value == 0 for key, value in audit.items() if key not in ['orders','states','inventory']), audit
    output = ROOT / 'docs/load-results/recovery'
    output.mkdir(exist_ok=True)
    (output / 'results.json').write_text(json.dumps({'checks': results, 'audit': audit}, indent=2))
    print(json.dumps(results), flush=True)

if __name__ == '__main__':
    main()
