#!/usr/bin/env python3
"""Deterministically exercise 219/221 cooldowns by holding a test inventory row lock.
This is fault injection, not a capacity benchmark. The row lock lasts three seconds.
"""
from concurrent.futures import ThreadPoolExecutor
import json
import subprocess
import time

from recovery import request
from run import ROOT, PG, metrics, sql


def product(stock):
    status, body = request('/products', 910000, {
        'name': 'Cooldown probe', 'saleStartsAt': '2020-01-01T00:00:00Z',
        'ticketTypes': [{'name': 'normal', 'priceCents': '12800', 'available': str(stock)}],
    }, True)
    assert status == 200, body
    return body


def buy(item, buyer, key):
    return request('/orders', buyer, {'productId': item['id'],
        'ticketTypeId': item['ticketTypes'][0]['id'], 'requestId': key})


def hold_stock(item):
    process = subprocess.Popen(['docker', 'exec', '-i', PG, 'psql', '-X', '-qAt',
        '-v', 'ON_ERROR_STOP=1', '-U', 'bench', '-d', 'bench'],
        stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    process.stdin.write(f"BEGIN; SELECT id FROM ticket_types WHERE id='{item['ticketTypes'][0]['id']}' FOR UPDATE; SELECT 'locked'; SELECT pg_sleep(3); COMMIT;\n")
    process.stdin.close()
    while True:
        line = process.stdout.readline().strip()
        if line == 'locked':
            return process
        if process.poll() is not None:
            raise RuntimeError(process.stderr.read())


def wait_inflight(expected):
    deadline = time.monotonic() + 2
    while time.monotonic() < deadline:
        if f'mall_purchase_inflight {expected}\n' in metrics():
            return
        time.sleep(0.02)
    raise AssertionError(f'Expected {expected} in-flight purchases')


def code(response):
    return response[1].get('metadata', {}).get('business_code')


def main():
    output = ROOT / 'docs/load-results/cooldown'
    output.mkdir(exist_ok=False)
    results = {}
    # Two prechecks see the last ticket before either conditional update can commit.
    item = product(1)
    holder = hold_stock(item)
    try:
        with ThreadPoolExecutor(max_workers=2) as pool:
            pending = [(buyer, pool.submit(buy, item, buyer, 'race')) for buyer in (910001, 910002)]
            wait_inflight(2)
            outcomes = [(buyer, future.result()) for buyer, future in pending]
        holder.wait(timeout=5)
        assert sorted(response[0] for _, response in outcomes) == [200, 409], outcomes
        loser = next(buyer for buyer, response in outcomes if code(response) == '219')
        blocked = buy(item, loser, 'cooldown-retry')
        assert code(blocked) == '3', blocked
        time.sleep(2.1)
        after = buy(item, loser, 'after-cooldown')
        assert code(after) == '100009', after
        results['219_then_code3_then_soldout_after_2_seconds'] = True
    finally:
        holder.wait(timeout=5)

    # All eight transaction slots are occupied by blocked stock updates; the ninth must fail fast.
    item = product(20)
    holder = hold_stock(item)
    try:
        with ThreadPoolExecutor(max_workers=8) as pool:
            pending = [pool.submit(buy, item, buyer, 'occupy-slot') for buyer in range(910010, 910018)]
            wait_inflight(8)
            rejected = buy(item, 910020, 'capacity-rejection')
            rejected_at = time.monotonic()
            assert code(rejected) == '221', rejected
            assert code(buy(item, 910020, 'immediate-retry')) == '3'
            for future in pending:
                assert future.result()[0] == 200
        # A second rejected retry must not extend the original cooldown window.
        assert code(buy(item, 910020, 'later-retry')) == '3'
        time.sleep(max(0, 5.15-(time.monotonic()-rejected_at)))
        admitted = buy(item, 910020, 'after-five-seconds')
        assert admitted[0] == 200, admitted
        results['221_then_code3_then_success_after_5_seconds_without_extension'] = True
    finally:
        holder.wait(timeout=5)
    audit = json.loads(sql((ROOT / 'scripts/load/audit.sql').read_text()))
    assert all(v == 0 for k,v in audit.items() if k not in ['orders','states','inventory']), audit
    (output / 'results.json').write_text(json.dumps({'checks': results, 'audit': audit}, indent=2))
    print(json.dumps(results), flush=True)

if __name__ == '__main__':
    main()
