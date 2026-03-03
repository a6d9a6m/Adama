## 6. 为什么要把 `task` 拆成三个小服务

### 现象

原来的 `task` 同时承担了 Kafka 消费和定时补偿两类职责。入口限流放开以后，`gateway/nginx` 的秒杀入队能力已经接近直连 `order`，但数据库里已同步订单的增长仍明显落后于入队量，说明瓶颈已经从入口转移到了 `task` 进程内部。

### 根因分析

- Kafka 消费一条消息不是轻量落库，而是要执行完整的 DTM TCC、调用 `goods` / `order` 分支，再更新工作流和订单表
- 定时任务又会周期性扫库、回补 Redis、调用远程取消库存接口
- 这两类负载模型混在一个进程里，会争抢 CPU、连接池和日志资源，导致 Kafka 消费吞吐不稳定

### 拆分方式

当前按 `JOB_ROLE` 把原来的 `task` 拆成 3 个小服务：

- `task-worker`
  - 只消费 Kafka `order` topic
  - 只负责异步建单
- `task-timeout`
  - 只运行 `order_timeout_close`
  - 只负责超时未支付关闭和库存回补
- `task-scheduler`
  - 只运行 `order_sync_repair`
  - `stock_consistency_check`
  - `cleanup_stats`

### 结论

这次拆分先解决的是职责边界问题，而不是直接改业务语义。拆开后，主消费链路不会再被超时关闭任务打断，补偿和巡检任务也可以单独观察、单独扩容。

## 7. 为什么当前要优先优化 TCC 链路

### 现象

把 `task` 拆成 `worker / timeout / scheduler` 后，秒杀入口的放量能力已经明显提升，但 `12s` 观察窗口内真正完成同步并落库的订单仍然远低于成功入队数。这说明瓶颈已经集中到 `Kafka -> task-worker -> DTM TCC -> goods/order` 这条消费主链路。

### 根因分析

- 当前每条 Kafka 消息都不是轻量写库，而是要完整执行一次 DTM TCC
- 一次 TCC 至少包含全局事务协调、`goods` 分支 Try/Confirm/Cancel、`order` 分支 Try/Confirm/Cancel，以及工作流状态更新
- 这条链路包含多次远程调用和多次数据库写入，天然比“预扣库存后直接异步落单”更重
- 现在 `task-worker` 还是“取一条消息，同步处理完，再提交 offset”的消费模型，所以单条消息耗时会直接限制整体吞吐

### 当前判断

- 瓶颈的重要来源是 TCC 本身带来的链路长度和一致性成本
- 但这不等于 DTM/TCC 天然只能跑到现在这个水平
- 更准确地说，是当前项目把完整 TCC 放在了每条异步消息的主处理路径里，于是它成了消费端的吞吐上限

### 前后对比

- 接入 DTM barrier 前，`64` 并发、`10s` 两阶段秒杀压测下：
  - `service`：`412.1 success-rps`，`12s` 后已同步约 `103` 单
  - `gateway`：`354.3 success-rps`，`12s` 后已同步约 `97` 单
  - `nginx`：`381.3 success-rps`，`12s` 后已同步约 `97` 单
- 接入 DTM barrier 后，同口径压测下：
  - `service`：`351.1 success-rps`，`12s` 后已同步约 `531` 单
  - `gateway`：`421.8 success-rps`，`12s` 后已同步约 `529` 单
  - `nginx`：`403.1 success-rps`，`12s` 后已同步约 `530` 单
- 结论：
  - barrier 的主要收益不是把入口吞吐单纯拉高，而是把分支正确性补齐后，显著提升了 `12s` 观察窗口内真正完成同步和落库的订单数
  - 当前 `task-worker` 仍然是瓶颈，但消费完成率已经从约 `100` 单提升到约 `530` 单，接近 `5x`

### 优化方向

- 先降低单条消息里的 TCC 成本：减少额外查询、合并状态更新、收紧非必要远程调用
- 再提高 TCC 并行处理能力：增加 Kafka 分区、提高 `task-worker` 消费并发、校准连接池和超时参数
- 最后再考虑缩小 TCC 的职责边界：只让真正需要强一致的步骤进入 TCC，其余动作后置异步处理

### 结论

当前阶段不应该先放弃 TCC，而应该先优化 TCC 在异步消费主路径里的实现成本和并行能力。等这两部分做完，再判断是否有必要进一步收缩 TCC 覆盖范围。

# FAQ

## 1. HTTP 接口返回 `context deadline exceeded`，但 Kafka 消息已经写入成功

### 现象

调用下单接口时，HTTP 请求返回超时错误：
```shell
INFO service.name= service.version= ts=2021-08-07T13:44:44+08:00 caller=order.go:11 msg=CreateOrder request: gid:1  amount:2
ERROR service.name= service.version= ts=2021-08-07T13:44:45+08:00 caller=order.go:37 msg=context deadline exceeded
INFO service.name= service.version= ts=2021-08-07T13:44:45+08:00 caller=tracing.go:44 kind=server component=http operation=/api.order.service.v1.Order/CreateOrder args=gid:1  amount:2 code=0 reason= stack= latency=1.004600849
```

但是 Kafka 中实际已经可以看到对应消息，说明消息发送并没有完全失败。

### 原因分析

- HTTP 层超时先于 Kafka 发送流程完成，导致调用方感知为失败。
- 单独测试 Kafka SDK 时问题不明显，说明问题更可能出现在服务超时配置、网络环境或框架接入方式上。
- 将 `http.timeout` 调整为 `2s` 后，超时问题明显缓解，说明 Kafka 发送链路在当前环境下存在额外耗时。

### 处理思路

- 适当放宽 HTTP 超时配置，避免把异步发送耗时误判为接口失败。
- 排查服务节点与 Kafka Broker 之间的网络延迟。
- 检查 Kafka 配置和 Kratos 接入方式，确认发送逻辑是否阻塞在请求主链路。
- 对异步消息链路增加更明确的成功 / 失败埋点，避免“已投递但接口超时”的误判。

### 结论

这个问题本质上是“请求超时”和“消息是否成功写入”两个维度没有被清晰区分。对于异步链路，应优先保证投递结果可观测，并合理设置超时边界。

---

## 2. 压测数据对比

压测命令：
```shell
wrk -t4 -c10 -d5s -T1s --script=post.json.lua --latency http://127.0.0.1:8001/adama/order
```

### 本地 Kafka 环境

```shell
jw@jianwei wrk$ wrk -t4 -c10 -d5s -T2s --script=post.json.lua --latency http://127.0.0.1:8001/adama/order
Running 5s test @ http://127.0.0.1:8001/adama/order
  4 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     7.22ms   20.48ms 202.93ms   96.16%
    Req/Sec   560.23    135.44   717.00     87.82%
  Latency Distribution
     50%    3.10ms
     75%    3.75ms
     90%    5.61ms
     99%  133.76ms
  11077 requests in 5.02s, 1.15MB read
Requests/sec:   2208.23
Transfer/sec:    235.06KB
```

结论：
- 本地 Kafka 环境下吞吐较稳定。
- 峰值约 `2208.23 req/s`。
- 延迟分布整体可接受，但 `P99` 仍有较大抖动。

### Docker Kafka 环境

```shell
jw@jianwei wrk$ wrk -t4 -c10 -d5s -T1s --script=post.json.lua --latency http://127.0.0.1:8001/adama/order
Running 5s test @ http://127.0.0.1:8001/adama/order
  4 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency     3.77ms    2.59ms  11.28ms   86.11%
    Req/Sec     3.38      5.97    22.00     90.48%
  Latency Distribution
     50%    2.79ms
     75%    3.55ms
     90%    9.73ms
     99%   11.28ms
  76 requests in 5.06s, 8.09KB read
  Socket errors: connect 0, read 0, write 0, timeout 40
Requests/sec:     15.03
Transfer/sec:      1.60KB
```

结论：
- Docker Kafka 环境下出现大量超时。
- 吞吐从 `2208.23 req/s` 下降到 `15.03 req/s`。
- 当前环境更像是部署或网络问题，而不是业务逻辑瓶颈。

### 早期版本压测

```shell
jw@jianwei wrk$ wrk -t4 -c10 -d5s -T1s --script=post.json.lua --latency http://127.0.0.1:8001/adama/order
Running 5s test @ http://127.0.0.1:8001/adama/order
  4 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency   506.40ms  526.09ms 999.36ms  100.00%
    Req/Sec     1.40      0.60     3.00     65.00%
  Latency Distribution
     50%  997.67ms
     75%  998.54ms
     90%  999.36ms
     99%  999.36ms
  44 requests in 5.08s, 4.68KB read
  Socket errors: connect 0, read 0, write 0, timeout 36
Requests/sec:      8.67
Transfer/sec:      0.92KB
```

```shell
jianwei:wrk jw$ wrk -t4 -c10 -d5s -T1s --script=post.json.lua --latency http://127.0.0.1:8001/adama/order
Running 5s test @ http://127.0.0.1:8001/adama/order
  4 threads and 10 connections
  Thread Stats   Avg      Stdev     Max   +/- Stdev
    Latency    61.84ms   70.50ms 883.00ms   96.22%
    Req/Sec    37.34     11.03    70.00     69.23%
  Latency Distribution
     50%   48.13ms
     75%   62.52ms
     90%   82.04ms
     99%  409.75ms
  741 requests in 5.05s, 78.88KB read
Requests/sec:    146.79
Transfer/sec:     15.63KB
```

结论：
- 系统早期吞吐只有 `8.67 req/s`。
- 优化后提升到 `146.79 req/s`，再到本地 Kafka 环境下的 `2208.23 req/s`。
- 说明缓存、异步链路和环境治理对秒杀系统性能影响非常明显。

---

## 3. 当前经验总结

- 秒杀系统性能问题不能只看业务代码，消息队列部署方式和网络环境同样是关键瓶颈。
- 对于异步链路，接口超时不等于投递失败，必须把“请求结果”和“消息结果”分开观察。
- 压测需要分环境记录，单次数据不能直接作为系统能力结论。

---

## 4. DTM / 异步落库 / 超时补偿链路缺少统一状态机

### 现象

- `order` 服务已有 Redis 预扣和 Kafka 发送。
- `goods` 服务已有 `dtmcli` 和 TCC 路由。
- `job` 服务能消费 Kafka，但只做最简单的落库。

三段链路都存在，但没有统一工作流状态，导致：

- TCC 成功后，异步落库是否完成无法追踪。
- 超时关单时，不知道 Redis 和 DB 库存哪一步已经补偿。
- Kafka 发送失败时，没有“待修复”状态供扫描任务继续处理。

### 根因

- 缺少覆盖 `订单状态 / Redis 状态 / DB 库存状态 / 异步同步状态` 的工作流表。
- TCC 分支、Kafka 消费者、超时扫描任务分散在不同服务里，但没有共享状态机。

### 处理

- 新增 `adama_order_workflows`，统一记录 `status`、`stock_status`、`cache_status`、`sync_status`、`expire_at`、`stock_token`。
- 新增 `adama_stock_reservations`，让商品库存预留的 Try / Confirm / Cancel 具备幂等语义。
- `order` 服务改为“Redis 预扣 -> DTM TCC -> Kafka 投递”，即使消息投递失败，也保留 `pending` 工作流供后续修复。
- `job` 服务改为“消费成功即标记 synced”，并通过定时扫描修复未落库订单和超时未支付订单。

### 结论

这类问题不是单纯补一个 TCC 接口就能解决，先建立统一工作流状态，再让事务、异步消息、补偿任务围绕同一状态机协同，链路才真正闭环。


## 5. 并发量不高

### 现象

在当前 docker compose 链路下，`goods_list` 读接口压测结果存在明显分层差异：

- 直连 `goods` 服务约 `3647 req/s`，`P99 33.26ms`
- 经过 `gateway` 约 `1767 req/s`，`P99 47.63ms`
- 再经过 `nginx` 约 `965 req/s`，`P99 118.74ms`

`gateway` 自身并发爬坡时，`c64` 仍能维持约 `2028 req/s`，但到 `c128` 时 `P99` 已升到 `194.10ms`，`c256` 时吞吐反而下降到 `1213 req/s`，并出现非 `2xx` 响应，说明当前环境里的并发上限并不高。

### 根因分析

- 当前 `nginx` 配置只有单 worker，`worker_connections` 也偏小。
- `nginx` 到 `gateway` 的上游代理未显式启用连接复用，默认更容易退化成频繁建立短连接。
- 在 `docker` 网络内单次请求的 `curl` 耗时只有几毫秒，说明主要不是单次业务处理慢，而是高并发下反向代理层的连接管理成本过高。

### 处理

- 将 `worker_processes` 改为 `auto`，把 `worker_connections` 提升到 `4096`。
- 给 `gateway_upstream` 和 `ops_ai_upstream` 增加 `keepalive` 连接池。
- 在代理 location 中显式使用 `proxy_http_version 1.1`，并清空 `Connection` 头，复用到上游的长连接。
- 将 `gateway` 代理热路径日志从 `Info` 降到 `Debug`，避免高并发时日志写入放大延迟。
- 给 `gateway` 到下游服务的 `http.Transport` 增加连接池参数，减少反向代理到业务服务的连接争用。
- 压测时优先在 compose 网络内直连服务名，避免把 Windows 主机端口转发噪音混入服务基线。

### 调优前后对比

同样使用 `goods_list` 场景，在 compose 网络内压测：

- `nginx c32`
  调优前：`965 req/s`，`P99 118.74ms`
  调优后：`1482 req/s`，`P99 62.48ms`
- `gateway c32`
  调优前：`1767 req/s`，`P99 47.63ms`
  调优后：`2876 req/s`，`P99 27.72ms`
- `gateway c64`
  调优前：`2028 req/s`，`P99 93.03ms`
  调优后：`2325 req/s`，`P99 82.16ms`
- `nginx c64`
  调优后基线：`1745 req/s`，`P99 154.56ms`
- `nginx c128`
  调优后基线：`871 req/s`，`P99 451.37ms`

额外现象：

- `nginx` 高并发时日志里主要出现 `499`
- `gateway` 对应日志主要是 `context canceled`

这说明高并发下大量失败并不是上游主动返回业务错误，而是请求在排队和转发过程中耗时过长，最终被压测客户端先取消。

### 结论

这个问题当前更像是“反向代理层连接复用、日志开销和连接池配置过保守”，而不是 `goods` 业务逻辑本身已经达到瓶颈。经过这轮调优后，`nginx -> gateway` 的吞吐差距已经明显收敛，但 `nginx` 在 `c128` 以上仍然会快速恶化，说明当前更稳妥的实用并发区间仍应控制在 `c64` 左右。
