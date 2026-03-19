# 2026-03-29 秒杀主链路 Review

本次审查范围：

- 秒杀主链路
- `task-worker`
- `goods/order` TCC 分支
- `gateway/nginx` 限流配置
- `ops-ai` 检索与推理链路

关注点：

- 并发风险
- 状态机漏洞
- 幂等缺口
- 锁顺序问题
- 配置误伤
- 可恢复性缺口

## Findings

### 高风险

#### 1. `task-worker` 处理失败后仍然提交 Kafka offset

- 严重性：高
- 类型：可恢复性缺口 / 消息语义错误
- 现象：
  `handler` 返回错误后只记录日志，随后仍然执行 `CommitMessages`。消息一旦提交，Kafka 不会再重投，这条链路实际变成了 at-most-once。
- 影响：
  秒杀异步建单失败后，队列本身不再承担重试能力，只能依赖 `order_sync_repair` 扫库补偿；一旦补偿漏扫、限额不足或状态异常，订单会永久卡住。
- 代码位置：
  - [app/job/service/job/server.go](/d:/CCY/Code/Go/adama/app/job/service/job/server.go#L305)
  - [app/job/service/job/server.go](/d:/CCY/Code/Go/adama/app/job/service/job/server.go#L309)
  - [app/job/service/internal/data/order.go](/d:/CCY/Code/Go/adama/app/job/service/internal/data/order.go#L77)

#### 2. 超时补偿调用了需要 barrier 的 `goods /ordersCancel`

- 严重性：高
- 类型：状态机漏洞 / 可恢复性缺口
- 现象：
  超时关闭路径直接调用 `goods /ordersCancel`，但该接口现在先执行 `BarrierFromHTTPContext`。超时补偿请求没有附带 DTM barrier query 参数，按当前实现会直接报错。
- 影响：
  超时未支付订单可能已经释放了 Redis 侧库存和用户资格，但 DB 侧库存预留没有真正释放，形成缓存与库存表不一致。
- 代码位置：
  - [app/job/service/internal/data/order.go](/d:/CCY/Code/Go/adama/app/job/service/internal/data/order.go#L243)
  - [app/job/service/internal/data/order.go](/d:/CCY/Code/Go/adama/app/job/service/internal/data/order.go#L331)
  - [app/goods/service/internal/service/goods.go](/d:/CCY/Code/Go/adama/app/goods/service/internal/service/goods.go#L76)
  - [pkg/dtmutil/barrier.go](/d:/CCY/Code/Go/adama/pkg/dtmutil/barrier.go#L13)
- 说明：
  这是基于当前代码路径的推断。如果另有不经过 barrier 的专用 cancel 入口，需要再复核。

#### 3. 旧的 no-op TCC HTTP 接口仍然对外暴露

- 严重性：高
- 类型：状态机漏洞 / 误用风险
- 现象：
  `order` 服务仍注册了 `/adama/orderTry`、`/adama/orderConfirm`、`/adama/orderCancel`，但实现只是直接返回 `SUCCESS`，不会执行真实的 TCC 业务逻辑。
- 影响：
  任何绕过网关的直连调用、内网误调用或未来配置回退，都可能命中这组假接口，产生“事务看起来成功，实际未落状态”的隐性故障。
- 代码位置：
  - [api/order/service/v1/order_http.pb.go](/d:/CCY/Code/Go/adama/api/order/service/v1/order_http.pb.go#L28)
  - [app/order/service/internal/service/order.go](/d:/CCY/Code/Go/adama/app/order/service/internal/service/order.go#L116)
  - [app/order/service/internal/server/http.go](/d:/CCY/Code/Go/adama/app/order/service/internal/server/http.go#L45)

#### 4. 用户上下文缺失时回退到硬编码用户 ID

- 严重性：高
- 类型：状态污染 / 幂等主体错误
- 现象：
  普通下单和秒杀下单都允许在没有用户上下文时回退到固定用户 `333` 或 `88`。
- 影响：
  网关透传异常、服务直连测试、调用方漏传头时，请求不会失败，而是落到错误用户下，污染订单、工作流和用户级幂等键。
- 代码位置：
  - [app/order/service/internal/service/order.go](/d:/CCY/Code/Go/adama/app/order/service/internal/service/order.go#L27)
  - [app/order/service/internal/service/order.go](/d:/CCY/Code/Go/adama/app/order/service/internal/service/order.go#L49)
  - [app/order/service/internal/service/order.go](/d:/CCY/Code/Go/adama/app/order/service/internal/service/order.go#L235)

### 中风险

#### 5. 秒杀限流把取 token 和下单共用同一组桶

- 严重性：中
- 类型：配置误伤 / 公平性问题
- 现象：
  `gateway` 的全局 limiter 命中所有 `/api/v1/adama/` 路径，热点 limiter 同时作用于 `GET /adama/goods/{id}` 和 `POST /adama/order`；`nginx` 侧则只按 IP 做限流。
- 影响：
  取 token 和真正下单互相争抢预算；同 NAT 下用户会互相影响，而恶意流量换 IP 即可绕过用户维度限制。
- 代码位置：
  - [app/gateway/internal/proxy/rate_limit.go](/d:/CCY/Code/Go/adama/app/gateway/internal/proxy/rate_limit.go#L53)
  - [app/gateway/internal/proxy/rate_limit.go](/d:/CCY/Code/Go/adama/app/gateway/internal/proxy/rate_limit.go#L103)
  - [app/gateway/internal/biz/gateway.go](/d:/CCY/Code/Go/adama/app/gateway/internal/biz/gateway.go#L42)
  - [deploy/nginx/nginx.conf](/d:/CCY/Code/Go/adama/deploy/nginx/nginx.conf#L8)
  - [deploy/nginx/nginx.conf](/d:/CCY/Code/Go/adama/deploy/nginx/nginx.conf#L30)

#### 6. `ops-ai` 动态查询没有真正按参数缩小证据范围

- 严重性：中
- 类型：推理链路误导 / 证据污染
- 现象：
  `Ask` 支持 `params`，但底层 SQL 查询基本只取“最近几条”，没有按 `order_id`、`goods_id`、`user_id` 等参数过滤。
- 影响：
  问具体订单或具体商品问题时，`ops-ai` 可能混入无关工作流、无关库存预留和无关错误，导致 RCA 方向跑偏。
- 代码位置：
  - [app/ops-ai/internal/biz/ops_ai.go](/d:/CCY/Code/Go/adama/app/ops-ai/internal/biz/ops_ai.go#L94)
  - [app/ops-ai/internal/data/data.go](/d:/CCY/Code/Go/adama/app/ops-ai/internal/data/data.go#L274)
  - [app/ops-ai/internal/data/data.go](/d:/CCY/Code/Go/adama/app/ops-ai/internal/data/data.go#L303)
  - [app/ops-ai/internal/data/data.go](/d:/CCY/Code/Go/adama/app/ops-ai/internal/data/data.go#L332)

#### 7. Kafka reader 遇到一次拉取失败就退出

- 严重性：中
- 类型：并发风险 / 可恢复性缺口
- 现象：
  `consumeLoop` 里 `FetchMessage` 只要返回一次错误，就直接退出当前 reader goroutine，没有重连、退避重试或 supervisor。
- 影响：
  Broker 短抖动、网络抖动或 rebalance 异常都可能让消费并发数逐步下降，最终吞吐恶化但进程表面仍然存活。
- 代码位置：
  - [app/job/service/job/server.go](/d:/CCY/Code/Go/adama/app/job/service/job/server.go#L266)

## 建议优先级

### P0

- 修正 `task-worker` 的提交语义：失败消息不要直接提交，至少要区分可重试错误与不可重试错误。
- 把超时补偿的 `goods cancel` 切到专用补偿入口，或补齐 barrier 所需上下文，不要直接复用 TCC 分支 HTTP 接口。
- 下线或显式禁用旧的 `/adama/orderTry|Confirm|Cancel` no-op 接口。
- 缺失用户上下文时直接拒绝请求，不要回退到硬编码用户。

### P1

- 拆分“取 token”和“下单”的限流预算，至少区分接口级和用户级维度。
- 让 `ops-ai` 动态查询真正使用 `params` 做条件过滤。
- 给 Kafka reader 增加重连与退避逻辑。

## 备注

- 本次审查聚焦正确性和可恢复性，不包含代码风格建议。
- 未重新跑压测，结论以当前仓库代码路径为准。
