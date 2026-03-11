接下来规划的计划：（服务命名不带 service 这个词）

新建 gateway 服务
落 JWT 鉴权、刷新、黑名单
落认证中间件与统一用户上下文
补商品列表、地址 CRUD、用户订单列表
补秒杀下单的一次性 token 与用户幂等
增加运维 ai 服务 ops-ai-service
给 task-service 增加 cron + 分布式锁
搭 docker-compose 本地全链路环境
接 Prometheus + Grafana 的最小监控集
接 Nginx 做统一入口
引入 ProxySQL 验证读写分离

---

更详细的规划补充：

一、gateway 服务落地规划

1. 服务命名与目录
- 新建 `app/gateway`
- 内部结构保持和现有服务一致：`cmd`、`configs`、`internal`
- `internal` 下至少拆分为 `server`、`service`、`biz`、`data`、`middleware`、`proxy`

2. 第一阶段能力
- 提供统一 HTTP 入口，统一前缀为 `/api/v1`
- 只代理公网接口：用户、商品、订单
- 不暴露 `task`、TCC、补偿、内部事务接口
- 接入统一日志、trace_id 透传、超时控制

3. 第二阶段能力
- 接入 JWT 鉴权
- 支持刷新 token
- 支持黑名单和强制登出
- 加入 IP 级和用户级限流
- 统一错误码和统一响应结构

4. 第三阶段能力
- 支持网关层聚合接口
- 支持秒杀接口单独限流与风控
- 预留灰度发布和熔断降级扩展点

二、认证与用户上下文规划

1. JWT 方案
- `user` 负责登录、签发 access token / refresh token
- `gateway` 负责校验 access token
- refresh token 可以先落 Redis，再评估是否持久化

2. 用户上下文透传
- 网关把 `user_id`、`trace_id`、`request_id` 注入下游 Header
- 下游服务不重复解析 token，只消费可信上下文

3. 黑名单与踢出机制
- Redis 维护 token 黑名单
- 用户修改密码、主动登出、风控封禁时，将 token 标记失效

三、业务能力补齐规划

1. 商品列表
- `goods` 增加列表查询接口
- 支持基础分页、状态过滤、秒杀活动过滤
- 网关对外暴露统一查询参数

2. 地址 CRUD
- 优先放在 `user`
- 先支持新增、修改、删除、列表、默认地址
- 后续订单下单直接复用地址快照

3. 用户订单列表
- 先由 `order` 提供分页查询
- 必要时由 `gateway` 聚合用户、商品展示字段

4. 秒杀一次性 token 与用户幂等
- 秒杀活动详情接口返回一次性 token
- 下单前校验 token 是否有效、是否过期、是否已使用
- Redis 记录用户维度幂等键，限制重复请求

四、task 服务完善规划

1. cron 任务
- 把超时未支付关闭、库存修复、未同步订单补偿统一放入 `task`
- 每类任务独立 handler，避免逻辑散落

2. 分布式锁
- 先用 Redis 分布式锁
- 保证同一类扫描任务在多实例场景只由一个执行者跑

3. 任务分类
- 订单补偿任务
- 超时关闭任务
- 库存一致性校验任务
- 数据清理与统计任务

五、ops-ai-service 规划

1. 服务定位
- 面向运维排障、知识检索和异常解释，不进入核心交易主链路
- 第一阶段只做只读能力，不直接执行生产写操作
- 输入主要来自 FAQ、规划文档、压测记录、运行日志、数据库只读查询结果

2. 模型选择
- 默认模型使用 `gpt-5 mini`
- 适合运维问答、FAQ 总结、日志归纳、故障现象解释
- 复杂根因分析和跨多份上下文综合判断时升级到 `gpt-5.2`
- 文档和 FAQ 检索向量化使用 embedding 模型，优先 `text-embedding-3-small`

3. 使用方式
- 统一通过 `Responses API` 接入
- 不是单纯聊天，而是“检索增强 + 工具调用”
- 先从 FAQ、docs、计划文档、压测历史中检索上下文，再交给模型总结回答
- 对数据库、日志、任务状态等动态信息，使用 tool calling 做只读查询

4. 第一阶段工具
- `search_faq`
- `search_docs`
- `query_order_workflow`
- `query_stock_reservation`
- `query_recent_errors`
- `query_benchmark_history`

5. 数据准备
- 把 `FQA.md`
- `scheduled.md`
- `docs/`
- `benchmarks/`
- 常见错误日志模板
- 这些内容统一切片后入向量库

6. 接口设计建议
- 提供 `/ops-ai/ask`
- 输入：问题、场景标签、是否允许动态查询
- 输出：结论、依据、引用片段、建议动作
- 明确区分“文档结论”和“模型推断”

7. 第二阶段扩展
- 接入日志平台查询
- 接入 Prometheus 指标查询
- 接入任务执行历史查询
- 增加异常巡检日报和 FAQ 自动补全能力

8. 风险边界
- 不允许模型直接写库
- 不允许模型直接执行 shell 或运维变更
- 所有动态查询工具默认只读
- AI 输出必须附带依据来源，避免无依据结论

六、docker-compose 全链路环境规划

1. 组成
- mysql
- redis
- kafka
- etcd
- nacos
- dtm
- gateway
- user
- goods
- order
- task
- nginx
- prometheus
- grafana

2. 目标
- 一条命令拉起本地联调环境
- 固定网络与服务名，消除硬编码 IP
- 支持最小压测和接口联调

3. 文件拆分建议
- `docker-compose.yml`：基础运行
- `docker-compose.obs.yml`：监控增强
- `docker-compose.dev.yml`：本地开发覆盖配置

七、监控规划

1. Prometheus
- 采集 gateway、goods、order、user、task 的 HTTP / gRPC 指标
- 采集 Redis、MySQL、Kafka、Nginx 基础运行指标

2. Grafana
- 先做 3 个核心看板：
- 请求量与错误率
- 秒杀下单延迟与成功率
- 补偿任务执行次数与失败次数

3. 告警最小集
- 秒杀下单错误率过高
- Kafka 消费堆积
- 超时关闭任务失败
- Redis / MySQL 不可用

八、Nginx 统一入口规划

1. 角色定位
- Nginx 仅做统一公网入口、TLS、反向代理、静态入口
- 业务网关能力由 `gateway` 承担

2. 路由关系
- 外部流量先到 Nginx
- Nginx 转发到 `gateway`
- `gateway` 再路由到内部服务

3. 不建议
- 不建议让 Nginx 直接承载鉴权、聚合、业务限流主逻辑

九、ProxySQL 读写分离规划

1. 目标
- 验证订单、商品、用户查询类流量走读库
- 写请求保持走主库

2. 实施步骤
- 先完成 MySQL 主从复制验证
- 再引入 ProxySQL
- 最后调整应用侧数据源接入方式

3. 风险点
- 读写延迟导致刚写后读不一致
- 秒杀主链路不应依赖最终一致的从库读取


