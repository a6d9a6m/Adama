# Redis 主冻结 TCC 方案分析

## 背景

当前秒杀主链路已经演进为：

- `GET /adama/goods/{id}` 获取一次性 token
- `POST /adama/order` 先执行 Redis/Lua 预留
- 之后写工作流并入 Kafka
- `task-worker` 异步消费，进入 `DTM TCC`

当前实现虽然已经把部分热点前移到了 Redis，但 `goods/order` 的 `Try/Confirm/Cancel` 仍然较重：

- `task-worker` 每条消息都要跑完整 `DTM TCC`
- `goods Try` 仍有 SQL 落账和状态写入
- `order` 工作流、barrier、订单表与业务表都落在单机 MySQL
- 前台入口和异步消费者共享同一套 MySQL / goods / order 资源

前面的压测和问题排查已经说明：

- Redis 预留本身不是当前主要瓶颈
- 瓶颈在 `task-worker -> DTM TCC -> MySQL`
- 当 `Try` 把热点判定放在 SQL 时，容易出现锁冲突、连接压力和吞吐波动

因此需要讨论一个更贴近秒杀场景的设计：`Try` 的高频库存冻结完全由 Redis 承担，SQL 退到账本、恢复和对账角色。

## 目标

目标不是放弃 TCC，而是重新划分 TCC 内部职责：

- `Redis` 承担热点库存冻结、用户资格占位、过期控制
- `SQL` 承担工作流锚点、订单事实、恢复、补偿、对账
- `DTM TCC` 继续承担跨服务一致性，但不再让 SQL 参与每次最热点库存判定

预期收益：

- 降低 `goods Try` 的 SQL 热点冲突
- 降低 `task-worker` 对 MySQL 的竞争强度
- 提高秒杀高峰下的稳定吞吐
- 保留可恢复性和补偿能力

## 当前方案与拟议方案对比

### 当前方案

`Try`

- Redis/Lua 校验 token、校验重复、预扣库存
- 写 `adama_order_workflows`
- 入 Kafka
- `task-worker` 再跑 `goods/order` 的 `TCC Try`
- `goods/order` 在 SQL 中继续写预留和状态

`Confirm`

- 确认库存和订单
- 更新工作流和订单表

`Cancel`

- 回补 Redis
- 回写 SQL 状态

问题在于：

- 热点库存已经在 Redis 判定过一次，SQL `Try` 还在承担一部分热点职责
- 数据库仍处在主竞争路径
- `task-worker` 处理一条消息的成本高

### 拟议方案

`Try`

- Redis/Lua 一次完成：
  - token 校验
  - 用户防重
  - 库存冻结
  - 冻结记录写入 Redis
- SQL 只写最小工作流锚点，或由异步路径低频写入

`Confirm`

- 订单成功后，把最终成功状态回写 SQL
- 清理或转化 Redis 临时冻结状态

`Cancel`

- 超时或失败时，按 Redis 冻结记录回补库存、释放用户占位
- 再把取消状态回写 SQL

SQL 不再承担：

- 每次热点库存是否足够的主判断
- 每次热点用户是否已经下单的主判断

SQL 主要承担：

- 工作流事实
- 订单事实
- barrier 与补偿痕迹
- 对账和恢复

## 建议状态模型

### Redis 侧

Redis 应成为 `Try` 的真实冻结事实源。建议保留或新增以下状态：

- `seckill:stock:{goods_id}`
  - 剩余可冻结库存
- `seckill:user:{goods_id}`
  - 已冻结或已抢购用户集合
- `seckill:reserve:{order_id}`
  - 单笔冻结记录
  - 最少包含：`order_id / user_id / goods_id / amount / status / expire_at`

其中 `seckill:reserve:{order_id}` 可以用：

- `Hash`
- 或编码后的 `String`

状态建议：

- `reserved`
- `confirmed`
- `cancelled`

所有最热点动作应尽量合并为一段 Lua：

- 校验 token
- 校验用户是否已冻结
- 校验库存是否足够
- 扣减库存
- 写用户占位
- 写冻结记录

### SQL 侧

SQL 退到账本角色后，建议最小化以下信息：

工作流表：

- `order_id`
- `user_id`
- `goods_id`
- `status`
- `cache_status`
- `sync_status`
- `expire_at`
- `last_error`

订单表：

- 只在确认成功后写入最终订单事实

库存预留表：

- 不再作为热点判定主表
- 只作为落账记录、恢复依据或审计依据

## Try / Confirm / Cancel 语义建议

### Try

Try 的主目标是：快速冻结，而不是在 SQL 中做完整确认。

建议动作：

1. Redis/Lua 完成冻结
2. 返回冻结成功结果
3. 异步或低频写 SQL 工作流锚点

如果工作流锚点写 SQL 失败：

- 需要立刻触发补偿，释放 Redis 冻结
- 或记录到待修复队列，由 scheduler 做极短周期恢复

关键点：

- Redis 冻结成功后，必须保证系统里最终能找到这笔冻结痕迹
- 否则会出现“库存被吃掉，但系统无记录”

### Confirm

Confirm 的目标是把“冻结成功”收敛成“订单成立”。

建议动作：

1. 校验 Redis 冻结记录仍存在且状态为 `reserved`
2. 写最终订单事实到 SQL
3. 更新工作流到 `pending_payment/synced`
4. 将 Redis 冻结标记为 `confirmed`，或做保留 TTL 缩短

关键点：

- `Confirm` 必须幂等
- 重复 `Confirm` 不能重复建单

### Cancel

Cancel 的目标是撤销冻结。

建议动作：

1. 校验 Redis 冻结记录
2. 如果仍是 `reserved`，则回补库存、释放用户占位
3. 回写 SQL 为 `cancelled` / `timeout_closed`
4. 若冻结记录不存在，则按空回滚处理

关键点：

- `Cancel` 必须幂等
- 空回滚和悬挂仍应由 `DTM barrier` 兜底

## 与 DTM barrier 的关系

这个方案不会替代 barrier。

barrier 仍然处理：

- 空回滚
- 重复 Confirm / Cancel
- 悬挂

区别只是：

- barrier 保护的业务动作不再是“热点 SQL 预留”
- 而是“Redis 冻结状态流转 + SQL 账本回写”

换句话说：

- TCC 语义还在
- barrier 还在
- 但 Try 的热点资源从 SQL 切到了 Redis

## 需要重点回答的设计问题

### 1. Redis 冻结成功但 SQL 工作流没写进去怎么办

这是该方案最关键的风险点。

可选策略：

- `方案 A`
  - Redis 冻结成功后，同步写最小工作流锚点
  - 若写失败，立即补偿 Redis
  - 优点：简单直接
  - 缺点：入口仍保留一笔 SQL

- `方案 B`
  - Redis 冻结成功后直接入 Kafka
  - 由异步消费者补写工作流
  - 若 Kafka 发送失败，则同步补偿 Redis
  - 优点：前台更轻
  - 缺点：恢复链更复杂

结合当前项目，建议先选 `方案 A`：

- 保留一笔最小工作流锚点写入
- 但不再让 SQL 参与热点库存判定

这样前台仍然有一笔 SQL，但已经比当前 `TCC Try` 轻很多。

### 2. Redis 数据丢失如何恢复

因为 Redis 承担主冻结事实，所以必须有恢复手段：

- SQL 工作流能反查“理论应存在的冻结”
- scheduler 扫描：
  - SQL 仍是 `reserved/preparing`
  - 但 Redis 冻结记录丢失
- 按业务策略：
  - 标记异常待人工处理
  - 或判定为失败并回收流程

因此 Redis 不能成为“唯一不可恢复状态源”，SQL 仍要保留最小账本。

### 3. 是否还需要库存预留 SQL 表

需要，但角色要变。

它不再承担：

- 最热点瞬时库存竞争

它继续承担：

- 审计
- 恢复
- 对账
- 异常排查

也可以考虑降频写入，例如：

- 仅在 Confirm / Cancel 后补落账
- 或由异步消费者批量补落

### 4. task-worker 是否还能跑完整 TCC

可以，但建议收缩 `Try` 的重量。

更合适的演进方向是：

- `task-worker` 仍驱动整体事务完成
- 但 `goods Try` 不再重做 SQL 热点库存冻结
- 它只做：
  - Redis 冻结状态确认
  - 最小账本回写

这样 `task-worker` 的单消息成本会显著下降。

## 推荐落地路径

建议分阶段改，不要一次性切换所有语义。

### 第一阶段

- 保留现有 Redis/Lua
- 把 `goods Try` 的 SQL 逻辑进一步降到最小
- 明确 Redis 冻结记录结构
- 明确 SQL 工作流最小锚点字段

### 第二阶段

- `Confirm/Cancel` 以 Redis 冻结记录为主
- SQL 只负责最终状态和账本
- scheduler 新增 Redis/SQL 对账检查

### 第三阶段

- 评估是否把工作流锚点也进一步后移
- 若后移，则同时补 Kafka 发送失败补偿和更明确的恢复策略

## 当前项目的建议结论

对当前项目，建议采用：

- Redis 作为主冻结事实源
- SQL 作为最小工作流锚点和最终账本
- barrier 保留
- `task-worker` 保留
- 但 `goods Try` 和相关 SQL 预留逻辑继续收缩

不建议：

- 完全移除 SQL 工作流锚点
- 完全依赖 Redis 而不保留恢复账本
- 在单机现状下继续增加热点 SQL Try 复杂度

## 讨论结论候选

后续设计讨论建议围绕这几个问题收敛：

1. 入口是否接受“保留一笔最小工作流 SQL”作为 Redis 方案的安全锚点
2. `goods Try` 是否正式退化为“Redis 冻结状态确认 + 低频账本写入”
3. `scheduler` 需要新增哪些 Redis/SQL 对账规则
4. 最终是否保留当前库存预留表，还是改成更轻的审计记录表

如果这几个问题达成一致，这个方向就可以进入具体字段、Lua 脚本和迁移实现设计。
