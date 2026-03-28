# 订单工作流

> 本文档聚焦 `adama_order_workflows` 这张表及其状态机，说明一笔秒杀订单从预留到建单、再到超时关闭或取消，是如何推进和收敛的。

## 为什么需要工作流表

秒杀链路不是一步完成的：

- Redis 先做资格和库存裁决
- Kafka 承担削峰
- `task-worker` 异步推进 TCC 和建单
- `task-timeout` 处理超时关闭
- `task-scheduler` 处理修复和巡检

因此系统需要一张中间状态表，承接“这笔秒杀请求现在走到哪一步了”。当前这个角色由 `adama_order_workflows` 承担。

## 核心字段

工作流表里的关键字段可以按职责分成 4 组。

### 1. 主键与业务标识

- `order_id`
- `user_id`
- `goods_id`
- `amount`
- `stock_token`

### 2. 状态字段

- `status`
- `stock_status`
- `cache_status`
- `sync_status`

### 3. 运行信息

- `kafka_attempts`
- `last_error`

### 4. 时间字段

- `expire_at`
- `paid_at`
- `created_at`
- `updated_at`

## 状态字段语义

### 主状态 `status`

| 值 | 含义 |
|:---|:---|
| `preparing` | 已进入异步建单流程，但事务还没完成 |
| `pending_payment` | 订单已建成，库存已保留，等待支付 |
| `cancelled` | 事务取消，缓存和库存都应释放 |
| `timeout_closed` | 超时未支付，被关闭并进入回收流程 |
| `paid` | 预留给支付成功后的最终状态 |

### 数据库库存状态 `stock_status`

| 值 | 含义 |
|:---|:---|
| `reserving` | 数据库侧库存事务还未收敛 |
| `reserved` | 数据库侧库存已成功预留 |
| `released` | 数据库侧库存已释放 |

### Redis 缓存状态 `cache_status`

| 值 | 含义 |
|:---|:---|
| `reserved` | Redis 侧库存和用户占位仍有效 |
| `released` | Redis 侧库存和用户占位已释放 |

### 同步状态 `sync_status`

| 值 | 含义 |
|:---|:---|
| `pending` | 异步建单流程未完成 |
| `synced` | 订单事实已落库，异步流程完成 |
| `cancelled` | 流程被取消或回滚完成 |

## 初始状态

当前主线代码里，工作流初始状态可能由两处写入：

1. `order` 分支的 TCC `Try`
2. `task-worker` 的 `ensureWorkflow`

初始化值一致：

| 字段 | 初始值 |
|:---|:---|
| `status` | `preparing` |
| `stock_status` | `reserving` |
| `cache_status` | `reserved` |
| `sync_status` | `pending` |

这表示：

- Redis 侧已经预留成功
- 数据库侧事务还没有完全确认
- 最终订单事实也还没有完全落下去

## 正常流转

### 阶段 1：入口预留成功

同步入口成功后，Redis 已经完成：

- token 消费
- 用户占位
- 库存扣减

但此时工作流表可能还没落库，真正的工作流锚点由异步消费者补齐。

### 阶段 2：异步确保工作流

`task-worker` 收到 Kafka 消息后，会先确保存在一条工作流：

```text
status       = preparing
stock_status = reserving
cache_status = reserved
sync_status  = pending
```

### 阶段 3：TCC Confirm 后进入待支付

当 `goods` 和 `order` 的 TCC 分支都成功后，工作流推进为：

```text
status       = pending_payment
stock_status = reserved
cache_status = reserved
sync_status  = pending
```

这意味着：

- 数据库库存已经预留成功
- Redis 库存仍保持预留
- 订单还没有完全标记为异步收敛完成

### 阶段 4：订单事实落库并完成同步

`task-worker` 创建 `adama_orders` 后，会把工作流更新为：

```text
status       = pending_payment
stock_status = reserved
cache_status = reserved
sync_status  = synced
```

这表示一笔“待支付订单”已经正式建立完成。

## 取消与回滚

### TCC Cancel

如果 TCC 失败或显式取消，订单服务的取消逻辑会：

1. 如果 Redis 侧尚未释放：
   - 回补 Redis 库存
   - 删除售罄标记
   - 移除用户占位
   - 删除幂等 key
2. 将工作流更新为：

```text
status       = cancelled
stock_status = released
cache_status = released
sync_status  = cancelled
```

### 空回滚

如果工作流不存在，取消逻辑直接返回，不报错。

这对应的是：

- 消息乱序
- Cancel 先于 Try 落地
- barrier 语义下的空回滚场景

## 超时关闭

`task-timeout` 会扫描两类记录：

1. `status = pending_payment` 且 `expire_at <= now`
2. `status = timeout_closed` 但 `stock_status != released`

处理动作分两步：

### 1. 先标记主状态

先将：

```text
status = timeout_closed
```

### 2. 再逐步释放资源

如果 `cache_status != released`：

- 回补 Redis 库存
- 删除售罄标记
- 移除用户占位
- 删除幂等 key
- 将 `cache_status` 更新为 `released`

如果 `stock_status != released`：

- 调用 `goods` 内部取消接口释放数据库预留
- 将 `stock_status` 更新为 `released`

因此 `timeout_closed` 不是终点快照，而是一个“正在关闭并等待释放完成”的状态。

## 修复任务

`task-scheduler` 会扫描：

- `sync_status != synced`
- 且 `status in (preparing, pending_payment)`

这些记录说明：

- 订单异步链路没完全完成
- 但还没有被明确关闭或取消

修复策略是重新调用 `CreateOrder`，让系统再次尝试：

1. 跑 TCC
2. 建订单事实
3. 更新工作流为 `synced`

## 一致性巡检

`task-scheduler` 还会扫描：

- `status = pending_payment`
- 但 `stock_status != reserved` 或 `cache_status != reserved`

这类记录通常意味着：

- 工作流主状态显示“待支付”
- 但库存或缓存状态没有正确收敛

这是一种明显的异常信号，应该视为补偿或排查入口。

## 状态转移图

```text
preparing
  -> pending_payment + pending
  -> pending_payment + synced

preparing
  -> cancelled

pending_payment + pending
  -> pending_payment + synced
  -> timeout_closed
  -> cancelled

timeout_closed
  -> timeout_closed + cache released
  -> timeout_closed + stock released
```

更准确地说，这张表不是单一状态机，而是：

- `status`
- `stock_status`
- `cache_status`
- `sync_status`

四个并行状态组合出来的收敛模型。

## 典型组合

| 组合 | 说明 |
|:---|:---|
| `preparing / reserving / reserved / pending` | 已进入异步阶段，等待 TCC 和建单完成 |
| `pending_payment / reserved / reserved / pending` | TCC 成功，但最终同步还没完成 |
| `pending_payment / reserved / reserved / synced` | 待支付订单已完全建成 |
| `cancelled / released / released / cancelled` | 事务取消并已完成资源释放 |
| `timeout_closed / released / released / synced or pending` | 超时关闭后资源已回收，是否 synced 取决于关闭时机 |

## 排障时怎么读这张表

排查一笔秒杀订单时，建议按下面顺序读：

1. 先看 `status`
   判断订单处于准备中、待支付、取消还是超时关闭。
2. 再看 `sync_status`
   判断异步建单流程是否真的完成。
3. 再看 `stock_status`
   判断数据库库存是否已成功预留或释放。
4. 再看 `cache_status`
   判断 Redis 侧是否还有占位。
5. 最后看 `last_error`
   判断最近一次失败点在哪。

## 相关文档

- [system-overview.md](/d:/CCY/Code/Go/adama/docs/architecture/system-overview.md)
- [seckill-mainline.md](/d:/CCY/Code/Go/adama/docs/architecture/seckill-mainline.md)
- [mysql-structure.md](/d:/CCY/Code/Go/adama/docs/architecture/storage/mysql-structure.md)

