#!/usr/bin/env python3
"""Produce a compact table from saved k6 summaries; no hand-entered benchmark numbers."""
import csv
import json
import re
import statistics
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2] / 'docs/load-results'
rows = []
for summary_path in sorted(ROOT.glob('*/summary.json')):
    directory = summary_path.parent
    config = json.loads((directory / 'config.json').read_text())
    metrics = json.loads(summary_path.read_text())['metrics']
    duration = float(config['duration'].removesuffix('s'))
    def values(name):
        return metrics.get(name, {}).get('values', {})
    success = values('purchase_outcomes{result:success}').get('count', 0)
    total = values('purchase_outcomes').get('count', 0)
    audit = json.loads((directory / 'audit.json').read_text())
    row = {'run': directory.name, 'target_rps': config['rate'], 'seconds': duration,
           'attempts': total, 'orders': success, 'orders_per_second': round(success/duration, 2),
           'success_percent': round(100*success/total, 3) if total else 0,
           'p95_ms': round(values('successful_order_ms').get('p(95)', 0), 3),
           'p99_ms': round(values('successful_order_ms').get('p(99)', 0), 3),
           'outer_429': values('purchase_outcomes{result:429}').get('count', 0),
           'inner_221': values('purchase_outcomes{result:221}').get('count', 0),
           'system_error_rate': values('system_errors').get('rate', 0),
           'dropped_iterations': values('dropped_iterations').get('count', 0),
           'paid': values('completed_payments').get('count', 0),
           'refunded': values('completed_refunds').get('count', 0),
           'audit_violations': sum(v for k,v in audit.items() if k not in ['orders','states','inventory'])}
    rows.append(row)
if rows:
    with (ROOT / 'summary.csv').open('w') as file:
        writer = csv.DictWriter(file, fieldnames=list(rows[0]))
        writer.writeheader()
        writer.writerows(rows)
    print('| Run | offered/s | committed/s | success % | P95 ms | P99 ms | 429 | 221 | system error rate | dropped |')
    print('|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|')
    for r in rows:
        print(f"| {r['run']} | {r['target_rps']} | {r['orders_per_second']} | {r['success_percent']} | {r['p95_ms']} | {r['p99_ms']} | {r['outer_429']} | {r['inner_221']} | {r['system_error_rate']} | {r['dropped_iterations']} |")

# Resource samples are coarse observations. 100% CPU means one logical CPU.
resources = {}
for path in sorted(ROOT.glob('*/samples.jsonl')):
    samples = {}
    for row in map(json.loads, path.read_text().splitlines()):
        for container in row.get('containers', []):
            name = ('k6' if '-k6-run-' in container['Name'] else
                    container['Name'].removeprefix('bili-mall-bench-').removesuffix('-1'))
            values = samples.setdefault(name, {'cpu_percent': [], 'memory_mib': []})
            values['cpu_percent'].append(float(container['CPUPerc'].rstrip('%')))
            match = re.match(r'([0-9.]+)([A-Za-z]+)', container['MemUsage'])
            if match:
                units = {'MiB': 1, 'GiB': 1024, 'KiB': 1/1024, 'B': 1/(1024*1024)}
                values['memory_mib'].append(float(match[1]) * units[match[2]])
    resources[path.parent.name] = {
        name: {key: {'min': min(values), 'max': max(values), 'mean': round(statistics.mean(values), 3),
                     'first': values[0], 'last': values[-1]}
               for key, values in record.items() if values}
        for name, record in samples.items()
    }
(ROOT / 'resource-summary.json').write_text(json.dumps(resources, indent=2))
