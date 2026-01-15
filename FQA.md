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
