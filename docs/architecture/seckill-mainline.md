# 秒杀主链路

> 本文档描述当前仓库主线代码里的秒杀下单流程，重点说明请求入口、同步阶段、异步阶段、TCC 协同和失败收敛方式。

## 适用范围

本文档描述的是当前主线实现，不是未来规划中的理想方案。

对应入口主要是：

- `GET /api/v1/adama/goods/{id}`
- `POST /api/v1/adama/order`
- `task-worker` 消费 Kafka `order` topic

## 链路目标

当前秒杀链路的目标是：

1. 在同步入口尽快完成资格校验和热点库存裁决
2. 把重操作后移到 Kafka 异步消费阶段
3. 通过工作流状态、TCC 和补偿任务让系统逐步收敛

## 入口接口

### 1. 获取秒杀 token

入口：

- `GET /api/v1/adama/goods/{id}`

主要动作：

1. 校验用户上下文
2. 读取商品秒杀信息
3. 校验活动开始时间和结束时间
4. 生成一次性 token
5. 将 token 写入 Redis，并设置到活动结束时间为止的 TTL

返回内容包括：

- 秒杀商品信息
- `seckill_token`
- 使用说明：请求头 `X-Seckill-Token`

### 2. 发起秒杀下单

入口：

- `POST /api/v1/adama/order`

主要前置条件：

- 必须有有效 `X-User-Id`
- 必须携带 `X-Seckill-Token`
- 商品必须处于秒杀时间窗口内
- Redis 售罄标记不能命中

## 同步阶段

订单服务收到秒杀请求后，同步阶段主要做 3 件事：

1. 检查商品是否售罄
2. 在 Redis 中原子完成资格校验和库存预留
3. 将订单消息写入 Kafka

### Redis/Lua 预留逻辑

同步热点裁决由 `reserveSeckillOrderScript` 承担，主要使用以下 key：

- `ADAMA:GOODS:{goods_id}:STOCK`
- `ADAMA:GOODS:{goods_id}:OVER`
- `ADAMA:ORDER:TOKEN:{user_id}:{goods_id}:{token}`
- `ADAMA:ORDER:USERS:{goods_id}`

Lua 脚本完成以下动作：

1. 校验 token 是否存在
2. 校验该用户是否已经在该商品上占位
3. 校验剩余库存是否足够
4. 扣减 Redis 库存
5. 删除一次性 token
6. 将用户写入该商品的下单用户集合
7. 库存耗尽时写售罄标记

返回语义：

- `0`：预留成功
- `1`：token 无效或已使用
- `2`：重复请求
- `3`：库存不足

### 同步阶段的产物

Redis 预留成功后，订单服务会：

1. 生成 `order_id`
2. 生成 `stock_token`
3. 将 `uid / goods_id / order_id / amount / expire_at / stock_token` 写入 Kafka 消息头

当前实现里，同步阶段不会立刻把工作流写入 MySQL，而是把这件事延后到异步消费侧的 `ensureWorkflow`。

这是理解当前主线实现的一个关键点：

- 同步入口负责“裁决和投递”
- 异步消费者负责“落工作流和推动事务完成”

## 异步阶段

`task-worker` 消费到 Kafka 消息后，会执行 `CreateOrder` 主流程。

### worker 第一阶段：确保工作流存在

如果当前 `order_id` 在 `adama_order_workflows` 中不存在，`task-worker` 会先补写一条最小工作流记录：

- `status = preparing`
- `stock_status = reserving`
- `cache_status = reserved`
- `sync_status = pending`

这一步把 Redis 侧已经预留成功的结果，锚定到 MySQL 工作流里。

### worker 第二阶段：决定是否跑 TCC

如果工作流尚未进入：

- `status = pending_payment`
- `stock_status = reserved`

则 `task-worker` 会发起一次 DTM TCC：

1. 调 `goods /ordersTry|Confirm|Cancel`
2. 调 `order /adama/tcc/order/try|confirm|cancel`

这里的职责分工是：

- `goods` 分支负责数据库侧库存预留与回滚
- `order` 分支负责订单工作流状态推进和 Redis 回补

### worker 第三阶段：建单并标记同步完成

TCC 成功后，`task-worker` 会：

1. 向 `adama_orders` 插入最终订单事实
2. 更新工作流为：
   - `status = pending_payment`
   - `stock_status = reserved`
   - `cache_status = reserved`
   - `sync_status = synced`
3. 清空 `last_error`

至此，这笔秒杀请求从“已预留、待异步完成”推进为“已建单、待支付”。

## TCC 分支职责

### goods 分支

`goods` 分支操作的是数据库库存预留表 `adama_stock_reservations`。

`Try`：

- 校验是否已有预留记录
- 对 `adama_goods.stock_count` 做条件扣减
- 写入或更新 `adama_stock_reservations`

`Confirm`：

- 将预留状态写成 `reserved`

`Cancel`：

- 如果没有预留记录，按空回滚处理
- 如果已释放，按幂等处理
- 否则回补数据库库存，并把预留状态改成 `released`

### order 分支

`order` 分支操作的是订单工作流和 Redis 缓存状态。

`Try`：

- 写入或更新 `adama_order_workflows`
- 初始化为 `preparing / reserving / reserved / pending`

`Confirm`：

- 将工作流推进到 `pending_payment / reserved / reserved / pending`

`Cancel`：

- 如果缓存侧尚未释放，则回补 Redis 库存、清售罄标记、移除用户占位、删除幂等 key
- 将工作流改成 `cancelled / released / released / cancelled`

## 失败处理

### 1. Kafka 投递失败

如果 Redis 预留成功但 Kafka 投递失败，订单服务会立即调用取消逻辑释放已预留的 Redis 资源。

这一步的目标是避免出现：

- Redis 库存已经被吃掉
- 但异步链路没有消息可消费

### 2. worker 处理失败

如果 `task-worker` 在 TCC 或建单过程中失败：

- 会把错误写回 `last_error`
- 工作流保持未同步状态
- 等待后续 repair 任务重试

### 3. 超时未支付

`task-timeout` 会扫描超时工作流：

- 将状态改为 `timeout_closed`
- 回补 Redis 库存
- 调用 `goods` 的内部取消接口释放数据库预留

### 4. 修复任务

`task-scheduler` 会周期性扫描：

- `sync_status != synced`
- 且 `status in (preparing, pending_payment)`

对这些记录重新执行 `CreateOrder`，推动其继续收敛。

## 链路全景

```text
Client
  -> Gateway
  -> Order: GET /adama/goods/{id}
  -> Redis: issue token

Client
  -> Gateway
  -> Order: POST /adama/order
  -> Redis/Lua: validate token + decr stock + mark user
  -> Kafka: publish order message

task-worker
  -> ensure workflow
  -> DTM TCC(goods + order)
  -> MySQL: insert adama_orders
  -> MySQL: update workflow to pending_payment + synced

task-timeout / task-scheduler
  -> timeout close / repair / consistency check
```

## 当前实现的几个关键特点

这条主链路有几个容易忽略的特点：

1. 同步入口已经做了 Redis 热点裁决，但 MySQL 工作流锚点是在异步侧补写的。
2. `task-worker` 不是单纯“落订单”，而是整个异步事务收敛器。
3. Redis、数据库库存预留、订单工作流三者不是一次同步写完，而是分阶段收敛。
4. repair 和 timeout 任务不是附属品，而是主链路失败后的正式恢复路径。

## 相关文档

- [system-overview.md](system-overview.md)
- [order-workflow.md](order-workflow.md)
- [transactions/redis-first-tcc.md](transactions/redis-first-tcc.md)
- [transactions/distributed-transaction.md](transactions/distributed-transaction.md)
