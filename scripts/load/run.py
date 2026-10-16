#!/usr/bin/env python3
"""Reproducible local benchmark. All mutations target the fixed isolated Compose project."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import threading
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'scripts/load/compose.yml')]
PG = 'bili-mall-bench-postgres-1'

def command(args, **kwargs):
    return subprocess.run(args, check=True, text=True, capture_output=True, cwd=ROOT, **kwargs).stdout

def sql(query):
    return command(['docker', 'exec', '-i', PG, 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'bench', '-d', 'bench'], input=query).strip()

def metrics():
    return urllib.request.urlopen('http://127.0.0.1:28001/metrics', timeout=5).read().decode()

def main():
    if (ROOT / 'scripts/load/.paused').exists():
        raise SystemExit('Benchmark paused; remove scripts/load/.paused to resume.')
    parser = argparse.ArgumentParser()
    parser.add_argument('name')
    parser.add_argument('--rate', type=int, default=50)
    parser.add_argument('--duration', default='30s')
    parser.add_argument('--stock', type=int, default=1000000)
    parser.add_argument('--vus', type=int, default=100)
    parser.add_argument('--tiers', type=int, default=1)
    parser.add_argument('--accounts', type=int, default=0)
    parser.add_argument('--mode', default='purchase', choices=['purchase', 'lifecycle'])
    args = parser.parse_args()
    if not args.name.replace('-', '').replace('_', '').isalnum():
        parser.error('name must contain only letters, numbers, hyphens or underscores')
    output = ROOT / 'docs/load-results' / args.name
    output.mkdir(parents=True, exist_ok=False)
    # Clear only benchmark tables and short-lived benchmark Redis limits between runs.
    sql('CREATE EXTENSION IF NOT EXISTS pg_stat_statements; TRUNCATE products, ticket_types, orders, purchase_claims, inventory_events, simulated_payments RESTART IDENTITY CASCADE;')
    sql(f"""INSERT INTO products(id,name,sale_starts_at) VALUES ('bench-event','Benchmark BW',now()-interval '1 minute');
        INSERT INTO ticket_types(id,product_id,name,price_cents,available)
        SELECT 'bench-tier-'||i,'bench-event','Tier '||i,12800,{args.stock} FROM generate_series(0,{args.tiers-1}) i;
        INSERT INTO inventory_events(ticket_type_id,operation_id,kind,available_delta,reserved_delta,sold_delta,withheld_delta)
        SELECT id,'initial:'||id,'initial',available,0,0,0 FROM ticket_types;
        SELECT pg_stat_statements_reset();""")
    command(['docker', 'exec', 'bili-mall-bench-redis-1', 'redis-cli', 'FLUSHDB'])
    (output / 'config.json').write_text(json.dumps(vars(args), indent=2))
    (output / 'metrics-before.txt').write_text(metrics())
    config = json.loads(command(COMPOSE + ['config', '--format', 'json']))
    (output / 'compose.json').write_text(json.dumps(config, indent=2))
    runtime = json.loads(command(['docker', 'inspect', '--format', '{{json .Config.Env}}', 'bili-mall-bench-mall-1']))
    public_settings = {key: value for entry in runtime for key, value in [entry.split('=', 1)]
                       if key in ['MALL_ENTRY_RATE', 'MALL_ENTRY_BURST', 'MALL_INNER_CONCURRENCY', 'MALL_IP_LIMIT_ENABLED']}
    public_settings['workload_sha256'] = hashlib.sha256((ROOT / 'scripts/load/benchmark.js').read_bytes()).hexdigest()
    (output / 'runtime-config.json').write_text(json.dumps(public_settings, indent=2))
    stop = threading.Event()
    def monitor():
        with (output / 'samples.jsonl').open('w') as file:
            while not stop.is_set():
                sample = {'time': time.time()}
                try:
                    raw = command(['docker', 'stats', '--no-stream', '--format', '{{json .}}'])
                    sample['containers'] = [entry for line in raw.splitlines()
                                            if (entry := json.loads(line))['Name'].startswith('bili-mall-bench-')]
                    sample['db_waits'] = json.loads(sql("SELECT coalesce(json_agg(s),'[]') FROM (SELECT state,wait_event_type,wait_event,count(*) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() GROUP BY 1,2,3) s;"))
                except Exception as error:
                    sample['error'] = str(error)
                file.write(json.dumps(sample) + '\n')
                file.flush()
                stop.wait(1)
    thread = threading.Thread(target=monitor)
    thread.start()
    start = time.time()
    envs = {'RUN_ID': args.name, 'RATE': str(args.rate), 'DURATION': args.duration,
            'TIERS': str(args.tiers), 'MODE': args.mode, 'VUS': str(args.vus), 'MAX_VUS': str(max(500, args.vus))}
    if args.accounts:
        envs['ACCOUNTS'] = str(args.accounts)
    cmd = COMPOSE + ['run', '--rm', '--no-deps', '-T']
    for key, value in envs.items():
        cmd += ['-e', f'{key}={value}']
    cmd += ['k6', 'run', '--quiet', '/scripts/benchmark.js']
    try:
        result = subprocess.run(cmd, cwd=ROOT, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        (output / 'k6.log').write_text(result.stdout)
    finally:
        stop.set()
        thread.join()
    (output / 'metrics-after.txt').write_text(metrics())
    audit = json.loads(sql((ROOT / 'scripts/load/audit.sql').read_text()))
    (output / 'audit.json').write_text(json.dumps(audit, indent=2))
    statements = sql("SELECT coalesce(json_agg(s),'[]') FROM (SELECT query,calls,total_exec_time,mean_exec_time,rows,shared_blks_hit,shared_blks_read FROM pg_stat_statements WHERE query NOT LIKE '%pg_stat%' ORDER BY total_exec_time DESC LIMIT 20) s;")
    (output / 'postgres-statements.json').write_text(json.dumps(json.loads(statements), indent=2))
    metadata = {'start': start, 'end': time.time(), 'k6_exit_code': result.returncode}
    (output / 'run.json').write_text(json.dumps(metadata, indent=2))
    if not (output / 'summary.json').exists():
        raise RuntimeError(result.stdout)
    summary = json.loads((output / 'summary.json').read_text())['metrics']
    counts = {code: int(summary.get(f'purchase_outcomes{{result:{code}}}', {}).get('values', {}).get('count', 0))
              for code in ['success','429','900001','3','100009','100019','219','221','system']}
    print(json.dumps({'run': args.name, 'counts': counts, 'latency': summary.get('successful_order_ms', {}).get('values'),
                      'dropped': summary.get('dropped_iterations', {}).get('values'), 'audit': audit,
                      'exit': result.returncode}, ensure_ascii=False), flush=True)
    if counts['success'] != audit['orders']:
        raise RuntimeError(f"HTTP successes {counts['success']} differ from committed orders {audit['orders']}")
    if args.mode == 'lifecycle':
        states = audit['states'] or {}
        completed = int(summary.get('completed_payments', {}).get('values', {}).get('count', 0))
        refunded = int(summary.get('completed_refunds', {}).get('values', {}).get('count', 0))
        if states.get('paid', 0) + states.get('refunded', 0) != completed or states.get('refunded', 0) != refunded:
            raise RuntimeError('HTTP payment/refund completions disagree with committed states')
    failures = {k: v for k, v in audit.items() if k not in ['orders','states','inventory'] and v}
    if failures:
        raise RuntimeError(f'Consistency audit failed: {failures}')

if __name__ == '__main__':
    main()
