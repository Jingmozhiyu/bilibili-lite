#!/usr/bin/env python3
"""Build the final narrative from archived benchmark evidence after all probes pass."""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DATA = ROOT / 'docs/load-results'


def read(run, file='summary.json'):
    return json.loads((DATA / run / file).read_text())


def metric(run, name):
    return read(run)['metrics'].get(name, {}).get('values', {})


def count(run, result):
    return int(metric(run, f'purchase_outcomes{{result:{result}}}').get('count', 0))


def row(run):
    config = read(run, 'config.json')
    duration = float(config['duration'].removesuffix('s'))
    attempts = int(metric(run, 'purchase_outcomes')['count'])
    success = count(run, 'success')
    latency = metric(run, 'successful_order_ms')
    return (f"| `{run}` | {config['rate']} | {duration:g} | {success:,}/{attempts:,} | "
            f"{success/duration:.2f} | {success/attempts*100:.3f}% | {latency['p(99)']:.2f} | "
            f"{int(metric(run,'dropped_iterations').get('count',0))} |")


def main():
    recovery = read('recovery', 'results.json')
    cooldown = read('cooldown', 'results.json')
    expiry = read('expiry-drain', 'result.json')
    assert all(recovery['checks'].values()) and all(cooldown['checks'].values())
    selected = ['capacity-200', 'capacity-800', 'repeat800-a', 'repeat800-b',
                'concurrency8-2000-resumed', 'concurrency32-2000',
                'capacity-4000-preallocated', 'protected-4000', 'lifecycle-200', 'lifecycle-soak180']
    table = '\n'.join(row(name) for name in selected)
    soak = 'lifecycle-soak180'
    paid = int(metric(soak, 'completed_payments')['count'])
    refunded = int(metric(soak, 'completed_refunds')['count'])
    end_to_end = metric(soak, 'successful_lifecycle_ms')['p(99)']
    outer = count('protected-4000', '429')
    offered = metric('protected-4000', 'purchase_outcomes')['count']
    gain = (count('concurrency8-2000-resumed','success') / count('concurrency32-2000','success') - 1) * 100
    tail_reduction = (1-metric('concurrency8-2000-resumed','successful_order_ms')['p(99)'] /
                       metric('concurrency32-2000','successful_order_ms')['p(99)']) * 100
    audit_paths = sorted(DATA.glob('*/audit.json'))
    total_orders = 0
    for path in audit_paths:
        audit = json.loads(path.read_text())
        assert all(v == 0 for k,v in audit.items() if k not in ['orders','states','inventory']), path
        total_orders += audit['orders']
    memory = json.loads((DATA / 'resource-summary.json').read_text())[soak]['mall']['memory_mib']
    report = f'''# bili-mall 实测报告与面试材料

日期：2026-09-26。测试针对当前工作树，业务代码在本次压测中未修改。源文件哈希见 [source-manifest.json](load-results/source-manifest.json)。所有实验使用独立 `bili-mall-bench` 环境，未读写正常主站或会员购业务数据。

## 1. 结论

- 单热点票种、唯一账号持续到达的 **800 请求/秒、30 秒**实验共三轮，实际成功创建约 **794–799 单/秒**，成功率 **99.275%–99.821%**，成功请求 P99 **2.89–4.79 ms**。这三轮均无系统错误、无压测漏发；外层预算被调高以观察交易路径。
- 默认外层预算 **200/s、burst 40** 下，约 **4,000 请求/秒**持续 30 秒，外层拒绝 **{outer:,} 次（{outer/offered*100:.2f}%）**，成功约 200 单/秒，成功下单 P99 约 5.11 ms。不能把 4,000 次入口请求写成 4,000 单/秒。
- 默认保护参数下，**180 个新买家/秒持续 5 分钟**，完成 **{paid:,} 次模拟支付、{refunded:,} 次退票**；下单至支付确认 P99 **{end_to_end:.0f} ms**。该轮下单成功率 100%，无漏发、无系统错误；mall 容器内存采样为 **{memory['min']:.2f}–{memory['max']:.2f} MiB**。具体结果见下表和原始记录。这是短时持续负载验证，不是全天稳定性证明。
- 2,000 请求/秒的对照中，内层并发上限从 32 收紧到 8，观察到成功吞吐增加 **{gain:.1f}%**、成功请求 P99 降低 **{tail_reduction:.1f}%**。原实现默认就是 8；本实验验证配置取舍，不能包装为一次业务代码优化。
- {len(audit_paths)} 轮 k6 实验累计创建 **{total_orders:,} 个测试订单**，各轮数据一致性审计均为零异常。另有故障与冷却探针。该结论仅覆盖本次执行场景，不是对任意并发情况的形式化证明。

## 2. 环境与口径

| 项目 | 配置 |
|---|---|
| 宿主机 | Apple M4 Pro，24 GiB RAM，ARM64 |
| Docker Desktop VM | 12 逻辑 CPU，约 7.75 GiB RAM；Engine 29.7.2 |
| mall | 单实例，上限 2 CPU / 512 MiB；数据库连接池上限 20 |
| PostgreSQL | 16.14，上限 2 CPU / 1 GiB；`fsync=on`、`synchronous_commit=on` |
| Redis | 独立实例，上限 1 CPU / 256 MiB |
| k6 | v2.3.0，镜像按 digest 固定，上限 2 CPU / 1 GiB；与服务共享 VM |
| 请求 | HTTP 容器网络；真实 JWT 校验；独立测试 issuer/key 签发账号，不压登录接口 |
| 数据 | 单商品、默认单票种、足够库存，每次新请求默认使用唯一账号及请求 ID |
| 支付 | 本地持久化模拟渠道，不涉及微信/支付宝网络或真实资金 |

应用和数据库各有 2 CPU 配额，不能写成“整套系统在一台 2 核服务器上达到这些结果”。没有浏览器、TLS、CDN、公网延迟或多实例扩容。首轮和续测跨两个会话；首轮正常应用空闲运行，续测正常应用已被用户停止。镜像及代码哈希、资源配额一致；对照 8/32 使用同一次续测的数据。

k6 使用 `constant-arrival-rate`，不依赖前一个请求完成才发下一次。统计区分入口请求、业务拒绝、成功提交和意外错误。`http_req_failed` 将 409/429 视为预期响应，因此不能用它代替下单成功率；以 `purchase_outcomes` 为准。P99 列只统计成功下单的 HTTP 请求，不包含支付；支付全链路另行统计。

表中成功单/秒 = 成功订单数 / 设定到达窗口。边界调度可能多发 1–2 次；k6 实际完成速率保存在 `summary.json`。`dropped_iterations` 非零表示计划请求没有全部发出，不能用目标速率冒充实际到达速率。

## 3. 主要结果

| 实验 | 目标请求/s | 秒 | 成功单/请求数 | 成功单/s | 下单成功率 | 成功下单 P99 ms | 漏发 |
|---|---:|---:|---:|---:|---:|---:|---:|
{table}

高负载实验的成功率下降主要来自业务码 221（内层名额已满），不是数据库提交成功。全部已完成 k6 实验的意外系统错误率为 0；原始 2,000/4,000 档曾分别漏发 26/253 次，阈值因此失败，记录没有删除。预分配 500 VU 后复测漏发为 0。不能仅凭低平均压测端 CPU 排除瞬时调度或 VU 初始化问题。

完整机器可读结果见 [summary.csv](load-results/summary.csv)，全部轮次见 [summary.md](load-results/summary.md)。每轮目录包括 k6 汇总、日志、配置、资源/数据库等待采样、SQL 统计和一致性审计。

## 4. 瓶颈与配置取舍

同一天 2,000 请求/秒对照：

- 并发上限 32：约 1,337 单/秒，P99 107.91 ms；库存 UPDATE 平均执行时间 **13.114 ms**。
- 并发上限 8：约 1,550 单/秒，P99 18.29 ms；同一 UPDATE 平均 **3.371 ms**。
- `pg_stat_activity` 采样出现 `transactionid` 和 `tuple` 锁等待；前者与事务持锁等待一致。请求集中竞争同一库存行，增加名额不会使该行的更新并行提交。
- 32 还超过连接池上限 20；本次没有采集 Go `sql.DB.Stats().WaitDuration`，不能精确拆分连接池等待、行锁等待及其他开销。

结论：这组环境下，更高并发上限使等待和拒绝增加。8 是当前可用起点，不是所有硬件、票种分布下的最优常数。若后续优化，可优先研究缩短库存行持锁时间、减少事务往返，并配合连接池等待指标；库存分桶或 Redis 预扣需要另行设计对账、补偿和崩溃恢复，不能为了漂亮 QPS 牺牲一致性。

默认外层预算的作用是控制进入资格查询/交易处理的流量。提高外层预算是容量实验配置，不建议直接把它作为线上默认值。当前外层是应用内 Redis 令牌桶，内层是每实例非阻塞并发槽位；没有 CDN、全局排队或跨实例全局交易名额。

## 5. 正确性与恢复实测

- **100 张票，约 2 万次请求**：只创建 100 单；7 次在事务内发现售罄返回 219，19,548 次预检无票返回 100009，346 次容量拒绝。无超卖。
- **单账号 5,001 次请求/10 秒**：1 单成功，4,991 次返回 900001，9 次返回 100019。不能用这种流量模型测交易容量。
- **冷却受控实验**：人为持有测试库存行锁 3 秒，使多个预检同时看到余票或占满 8 个名额，验证 219 → 错误码 3 → 2 秒后重新预检，以及 221 → 错误码 3 → 5 秒后成功。中途重试没有延长冷却。
- **跨票种**：同账号 20 个并发请求只有一单；新请求 ID 仍受已有订单限制。
- **幂等与过期**：相同请求 ID 返回同一订单；加速过期后原 ID 返回关闭订单，新 ID 可重新购票。
- **Redis 真实停机**：新下单返回 503，已有订单查询、支付、退款仍成功；恢复后退票账号仍被禁止重购。
- **应用真实重启**：待支付订单保留，重启后可完成支付。
- **支付故障状态注入**：停止应用后，用 SQL 构造“已记支付意图、模拟渠道已扣款、订单尚未确认”的持久化状态，再启动应用；worker 将有效订单确认为已支付，将已过期订单的迟到扣款退款。这是受控状态注入，不是实际第三方支付故障实验。
- **集中关单**：把 {expiry['initial_pending']:,} 个测试订单的截止时间调整到过去，worker 在约 **{expiry['drain_seconds']:.2f} 秒**内清理完毕，库存与资格释放正确。它每批最多 100 条，说明关单有排队延迟；10 分钟是支付截止时间，不代表所有订单恰好第 600 秒同步完成关单。此实验没有等待真实十分钟。

每轮 SQL 审计在同一个一致性快照中检查：库存非负、库存与流水相符、锁定/已售/暂存与订单状态相符、账号活动订单唯一、请求 ID 唯一、资格绑定一致、模拟支付账本金额/引用/退款状态一致。HTTP 成功单数还与数据库订单总数核对；完整链路再核对支付及退款完成数。

故障证据：[recovery](load-results/recovery/results.json)、[cooldown](load-results/cooldown/results.json)、[expiry-drain](load-results/expiry-drain/result.json)。

## 6. 简历怎么写

推荐选一条容量描述、一条保护或正确性描述，不必堆全部数字：

> 基于 Go/Kratos、PostgreSQL 与 Redis 实现门票抢购模块，利用请求幂等、账号资格锁与条件扣库存保证交易一致性；在本地 Docker 单应用实例 2 CPU、数据库另配 2 CPU 的测试环境下，800 请求/秒的三轮 30 秒实验达到约 794–799 成功单/秒，成功率不低于 99.27%，成功下单 P99 不超过 4.8 ms，测试内库存与订单对账无异常。

> 设计外层令牌桶、账号频控/冷却和内层有界并发保护；默认 200/s 预算下承受约 4,000 次入口请求/秒的 30 秒实验，约 95% 请求在外层被拒绝，成功下单维持约 200 单/秒；完成 Redis 中断、应用重启及支付恢复验证。

持续负载补充：180 新买家/秒持续 5 分钟，完成 {paid:,} 次模拟支付与 {refunded:,} 次退票，支付链路 P99 {end_to_end:.0f} ms。务必保留“模拟支付”和“本地测试”限定。

不能写“支持 4,000 下单 QPS”“生产级十万并发”“零故障”“验证多实例扩容”“真实支付高可用”。本实验没有覆盖这些结论。

## 7. 面试谈资：问题、证据与回答

**为什么不只看 QPS？** 4,000/s 默认保护测试里约 95% 是被拦截的请求。真正成功约 200 单/秒。我分别统计请求量、业务码、落库订单、尾延迟及库存审计。

**为什么用开放到达模型？** 如果每个虚拟用户等待响应再发请求，服务变慢时压测端也会自动降速。我固定到达速率，并检查 dropped iterations；曾发现漏发，保留失败轮次，预分配 VU 后复测。

**并发越大越好吗？** 不是。这次 32 个名额比 8 个名额吞吐更低、P99 更高。数据库采样显示热点库存行锁等待；应用并发还要结合连接池与持锁时间确定。

**预检有票为什么还会返回 219？** 预检只是快照，100 张票竞争实验实际出现 7 次这种竞争。事务内的条件扣减才决定能否成功，失败回滚不会额外扣库存。

**Redis 挂了会超卖吗？** 当前采用拒绝新下单的策略；库存事实源和一致性约束仍在 PostgreSQL。故障测试中已有订单支付、查询、退款继续工作，避免把保护组件故障扩大到整个交易生命周期。

**十分钟过期靠 Redis TTL 吗？** 不靠。数据库保存截止时间，支付路径检查过期，worker 批量回收。集中到期有处理延迟，本次 {expiry['initial_pending']} 单约 {expiry['drain_seconds']:.2f} 秒清空；后续需要监控逾期积压和最老逾期时间。

**支付成功但应用崩溃怎么办？** 先保存支付意图，再调用渠道；worker 查询未决结果并幂等落库。已过期订单收到扣款结果则退款补偿。这次用独立模拟账本和持久化状态注入验证恢复，尚未接真实渠道。

## 8. 后续优先级

1. 补充 `sql.DB.Stats`、事务耗时、关单积压/最老逾期时间、worker 批次耗时等指标，再接 Prometheus/Grafana 与告警。
2. 用独立负载机、固定后台负载和更长运行时间重复测试；这次五分钟不能排除慢性资源泄漏或日级稳定性问题。
3. 再验证多实例全局限流一致性、数据库故障与恢复、网络延迟和真实支付回调。
4. K8s 部署、扩缩容和滚动升级属于后续阶段，本报告不计入已完成能力。

## 9. 复现与文件

见 [压测操作说明](../scripts/load/README.md)。`run.py` 仅清理独立压测数据；每轮结果目录不可复用。`summarize.py` 生成表格，`build_report.py` 从原始 JSON 生成本报告。正常应用与压测应用使用不同 JWT issuer/key、端口、数据库和卷。
'''
    (ROOT / 'docs/bili-mall-benchmark.md').write_text(report)

if __name__ == '__main__':
    main()
