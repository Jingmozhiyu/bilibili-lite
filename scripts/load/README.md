# Mall benchmark

This directory runs a separate `bili-mall-bench` Compose project. It never mounts the normal site or mall volumes. Credentials in this directory are disposable test credentials and intentionally differ from the regular application issuer/key.

## Start

```sh
docker compose -f scripts/load/compose.yml up -d --build mall
python3 scripts/load/run.py my-run --rate 200 --duration 30s
```

If a previous session was explicitly paused, remove `scripts/load/.paused` before resuming.

Each run name must be new. `run.py` **clears only the benchmark database tables and benchmark Redis** before preparing the next run. Do not run two experiments concurrently. Results are saved in `docs/load-results/<run>/`.

The default benchmark raises the outer token bucket to 100,000/s so it can measure the transaction path. This is not the normal application default. Account IDs are unique per iteration unless `--accounts` is provided; otherwise eligibility limits would hide transaction capacity. A single ticket tier is used by default to expose hot-row contention. Every successful HTTP response is compared with committed order counts.

## Experiments

```sh
# Full purchase and simulated payment; every tenth buyer also refunds.
python3 scripts/load/run.py lifecycle-example --rate 200 --duration 60s --mode lifecycle

# Many attempts compete for 100 tickets; still audit inventory and claims afterward.
python3 scripts/load/run.py soldout-example --rate 2000 --duration 10s --stock 100

# Repeat attempts from one account to exercise middle-layer limits.
python3 scripts/load/run.py account-example --rate 500 --duration 10s --accounts 1

# Restore normal outer protection, keeping the load much higher than its budget.
export BENCH_ENTRY_RATE=200 BENCH_ENTRY_BURST=40 BENCH_INNER_CONCURRENCY=8
docker compose -f scripts/load/compose.yml up -d mall
python3 scripts/load/run.py protected-example --rate 4000 --duration 30s

# Fault/recovery checks; expiry uses an explicitly accelerated SQL deadline.
python3 scripts/load/recovery.py
python3 scripts/load/summarize.py

# Prepare 1,001 pending orders and measure an accelerated expiry batch.
python3 scripts/load/run.py expiry-fixture-example --rate 100 --duration 10s
python3 scripts/load/expire.py

# Under the normal inner concurrency limit of 8, inject row-lock delays
# to verify both cooldown paths. This is not a throughput benchmark.
python3 scripts/load/cooldown.py
```

`recovery.py` stops/restarts **benchmark Redis and mall only**. It verifies new-purchase failure during Redis interruption, continued query/payment/refund, process restart, idempotency, cross-tier eligibility and accelerated expiry. It also injects durable processing/provider-ledger states while the app is stopped to verify interrupted-charge and late-charge reconciliation. It does not claim to reproduce a real payment-provider network outage or wait ten wall-clock minutes.

## Interpretation

- `successful_order_ms` measures successful HTTP create-order request duration, excluding login and browser rendering.
- `purchase_outcomes` separates committed orders from business rejections. A 429 response is not a successful order.
- `system_errors` covers unexpected outcomes; in lifecycle mode its denominator includes purchase, payment and refund outcome checks, not just buyers.
- `dropped_iterations` shows when the load generator cannot start scheduled work. Do not claim the configured rate was delivered if it is nonzero.
- The reported order rate is committed orders divided by the configured arrival window. End-of-window scheduling can produce one additional iteration; completion may extend into the graceful-stop period.
- `audit.json` checks nonnegative stock, ledger agreement, state-to-stock agreement, duplicate active purchases, request-ID uniqueness, claims and mock payment ledger agreement.
- Samples are coarse observations, not continuous profiling; the monitor also adds small database and Docker overhead.
- `postgres-statements.json` records statement timings including lock waits. SQL timing alone does not identify the complete bottleneck.
- The load generator shares the Docker VM with services. Container CPU percentages use 100% for one logical CPU; the mall quota permits up to 200%.
- No TLS/CDN/browser/login RPC or real payment provider participates. JWT verification remains enabled.

## Stop

```sh
docker compose -f scripts/load/compose.yml down
```

Add `--volumes` only to discard the isolated benchmark data as well. This Compose file uses a distinct project and volume name; never substitute the regular project file in reset commands.

## Archived results

See [the measured report and interview notes](../../docs/bili-mall-benchmark.md).
`python3 scripts/load/summarize.py` rebuilds CSV and resource summaries; redirect its stdout to `docs/load-results/summary.md` for the full table.
`python3 scripts/load/build_report.py` rebuilds the narrative from the named archived experiments, after the recovery/cooldown/expiry probes have passed.

## Live Grafana dashboard (Docker Desktop)

The normal monitoring stack can scrape this isolated stack at `host.docker.internal:28001`.
Open `/d/bili-mall` in Grafana and select **压测环境** at the top. Verify the `mall-bench`
Prometheus target is UP and allow two scrapes before starting a run. Use Last 5 minutes.
The benchmark scrape interval is 5 seconds; rate charts use a 1-minute window, so short
bursts are smoothed. Past runs cannot be reconstructed if Prometheus was not scraping.
Benchmark downtime does not trigger normal-environment alerts.
