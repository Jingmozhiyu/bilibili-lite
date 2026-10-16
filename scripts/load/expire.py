#!/usr/bin/env python3
"""Measure draining a batch of expired orders; deadline is advanced only in the test DB."""
import json
import time

from run import ROOT, sql

def main():
    output = ROOT / 'docs/load-results/expiry-drain'
    output.mkdir(exist_ok=False)
    pending = int(sql("SELECT count(*) FROM orders WHERE state='pending';"))
    if not pending or pending > 2000:
        raise SystemExit('Prepare a small expiry fixture with run.py first (1..2000 pending orders).')
    started = time.monotonic()
    sql("UPDATE orders SET expires_at=clock_timestamp()-interval '1 second' WHERE state='pending';")
    samples = []
    while True:
        left = int(sql("SELECT count(*) FROM orders WHERE state='pending';"))
        samples.append({'seconds': round(time.monotonic()-started, 3), 'pending': left})
        if left == 0 or time.monotonic()-started > 40:
            break
        time.sleep(0.5)
    audit = json.loads(sql((ROOT / 'scripts/load/audit.sql').read_text()))
    result = {'initial_pending': pending, 'drain_seconds': samples[-1]['seconds'],
              'accelerated_deadline': True, 'samples': samples, 'audit': audit}
    (output / 'result.json').write_text(json.dumps(result, indent=2))
    assert left == 0, result
    assert all(v == 0 for k,v in audit.items() if k not in ['orders','states','inventory']), audit
    print(json.dumps({'expired': pending, 'drain_seconds': result['drain_seconds'], 'audit_passed': True}), flush=True)

if __name__ == '__main__':
    main()
