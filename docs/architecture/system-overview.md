# 系统总览

> 这份文档用于回答 4 个问题：系统由哪些服务组成、请求怎么流动、关键依赖是什么、后续应该从哪里继续阅读。

## 目标

`adama` 是一个围绕秒杀场景搭建的 Go 微服务工程，用来验证以下几类后端问题：

- 高并发下如何避免超卖
- 如何把同步下单改造成异步削峰
- 如何在 Redis、Kafka、MySQL、多服务协作下保持状态尽量一致
- 如何把网关、观测、任务补偿、压测和部署组织成统一工程骨架

它不是单纯的 CRUD 示例，更偏向“秒杀主链路 + 一致性 + 工程化”的后端实践仓库。

## 服务划分

当前核心服务分为 6 类：

| 模块 | 目录 | 主要职责 |
|:---|:---|:---|
| 用户服务 | `app/user/service` | 用户注册、登录、用户信息与地址相关能力 |
| 商品服务 | `app/goods/service` | 商品查询、库存相关事务分支、商品侧数据访问 |
| 订单服务 | `app/order/service` | 普通下单、秒杀下单、工作流写入、Kafka 投递 |
| 任务服务 | `app/job/service` | 异步消费、超时关闭、修复任务、一致性巡检 |
| 网关服务 | `app/gateway` | 对外统一入口、路由转发、暴露面控制、秒杀限流 |
| 运维 AI 服务 | `app/ops-ai` | 面向运维排障和知识检索的只读辅助能力 |

其中 `app/job/service` 进一步拆成 3 个运行角色：

- `task-worker`：消费 Kafka `order` topic，异步执行建单链路
- `task-timeout`：关闭超时未支付订单，并触发库存回补
- `task-scheduler`：修复未同步订单、巡检库存一致性、收集统计

## 基础依赖

系统围绕以下基础组件运行：

| 组件 | 角色 |
|:---|:---|
| MySQL | 订单工作流、订单事实、商品与用户数据持久化 |
| Redis | 秒杀 token、库存预扣、售罄标记、用户幂等与任务锁 |
| Kafka | 秒杀异步削峰与任务分发 |
| DTM | TCC 协调与 barrier 语义兜底 |
| etcd | 服务注册发现 |
| Nacos | 配置管理 |
| Prometheus | 指标采集 |
| Grafana | 看板展示 |
| Nginx | 统一公网入口 |
| ProxySQL | 读写分离验证 |

## 目录结构

仓库主要目录职责如下：

| 目录 | 职责 |
|:---|:---|
| `app/` | 各业务服务实现 |
| `api/` | Proto、gRPC、HTTP Gateway、Swagger 生成产物 |
| `pkg/` | 公共组件与通用工具 |
| `deploy/` | Compose、Nginx、Prometheus、Grafana、MySQL、ProxySQL 配置 |
| `benchmarks/` | wrk 脚本、结果记录与汇总工具 |
| `docs/` | 规划、架构、AI 沉淀、性能与测试文档 |
| `FQA.md` | 高频问题与排查沉淀 |
| `scheduled.md` | 当前执行任务清单 |

## 对外入口与暴露面

系统对外请求优先经由以下路径进入：

```text
Client -> Nginx -> Gateway -> User/Goods/Order/Ops-AI
```

其中 `gateway` 统一使用 `/api/v1` 前缀，并显式做了两类控制：

1. 暴露面控制
   只放行用户、商品、订单和秒杀相关公网接口。
2. 内部路径拦截
   阻断 `/internal`、`/task`、`/compensate`、`/adama/tcc` 以及旧的 TCC 直连入口。

这意味着：

- 内部事务分支接口不应该由外部直接调用
- 秒杀主链路应优先走网关暴露的稳定入口

## 核心调用关系

### 普通查询链路

```text
Client
  -> Gateway
  -> User/Goods/Order
  -> MySQL / Redis
```

这类链路以同步读写为主，关注接口聚合、路由暴露和上下文透传。

### 秒杀下单主链路

```text
Client
  -> Gateway
  -> Order
  -> Redis/Lua 预留库存与资格
  -> Order 写工作流锚点
  -> Kafka 投递消息
  -> task-worker 消费
  -> goods/order TCC 分支协同
  -> MySQL 落账与状态推进
```

这条链路是当前项目最核心的后端价值点，涉及：

- Redis 热点路径裁决
- Kafka 异步削峰
- 订单工作流状态机
- DTM / TCC / barrier
- 失败补偿与最终一致性

### 超时与补偿链路

```text
task-timeout
  -> 扫描超时未支付订单
  -> 回写工作流状态
  -> 触发库存回补

task-scheduler
  -> 修复未同步订单
  -> 巡检库存一致性
  -> 汇总工作流统计
```

这部分链路承担的是“恢复能力”，不是吞吐主路径，但决定系统能否从异常中收敛。

## 状态职责划分

当前系统里，几类状态源各自承担的职责大致如下：

| 状态源 | 主要职责 |
|:---|:---|
| Redis | 秒杀 token、用户幂等、库存预留、售罄标记、部分短期事实 |
| Kafka | 异步排队和削峰，不保存业务最终状态 |
| MySQL 工作流表 | 订单状态推进、同步状态、错误信息、过期时间 |
| MySQL 业务表 | 订单事实、商品事实、用户事实 |
| task 任务 | 补偿、回收、修复、巡检 |

理解这层职责划分，有助于区分：

- 哪些动作是“快速裁决”
- 哪些动作是“异步完成”
- 哪些动作是“失败后收敛”

## 当前工程特点

相对普通后端项目，这个仓库最值得关注的点主要有：

- 秒杀链路不是单点实现，而是 Redis、Kafka、MySQL、任务服务协同完成
- 同时保留了 Proto、gRPC、HTTP 三层契约与生成链路
- 网关、监控、压测、部署和读写分离验证都放在同一仓库内维护
- 任务服务被显式拆成 worker、timeout、scheduler 三个角色，职责更清晰

## 推荐阅读顺序

第一次接触仓库，建议按下面顺序阅读：

1. 当前文档：建立系统边界和模块感知
2. [redis-first-tcc.md](/d:/CCY/Code/Go/adama/docs/architecture/transactions/redis-first-tcc.md)：理解秒杀主链路的设计取舍
3. [distributed-transaction.md](/d:/CCY/Code/Go/adama/docs/architecture/transactions/distributed-transaction.md)：理解 DTM / TCC 背景
4. [mysql-structure.md](/d:/CCY/Code/Go/adama/docs/architecture/storage/mysql-structure.md)：理解持久化对象
5. [redis-key.md](/d:/CCY/Code/Go/adama/docs/architecture/storage/redis-key.md)：理解缓存与幂等 key
6. [kafka.md](/d:/CCY/Code/Go/adama/docs/architecture/messaging/kafka.md)：理解异步消息设计
7. [README-compose.md](/d:/CCY/Code/Go/adama/deploy/README-compose.md)：理解本地运行和依赖编排

## 后续文档落点

这份总览之后，后续文档按主题继续展开：

- 秒杀主链路：重点写 token、预留、入队、消费、补偿
- 订单工作流：重点写状态机和状态推进规则
- gateway：重点写暴露面、限流和上下文透传
- task：重点写三类角色和调度职责
- 观测与压测：重点写指标、看板和基线

如果某个问题已经需要落到 SQL、Redis key、TCC 分支或补偿细节，说明应该进入专题文档，而不是继续堆在总览里。

