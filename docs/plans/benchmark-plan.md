# Benchmark 改造计划

## 目标

让 `benchmarks/` 从“单个旧接口的临时 wrk 脚本”升级为“可重复执行、可记录结果、可对比环境差异”的压测资产，覆盖当前项目已经落地的这些能力：

- `gateway` 统一入口
- `nginx -> gateway -> service` 转发链路
- 秒杀详情取 token + 下单两阶段流程
- `task` 异步修复与超时关闭链路
- `Prometheus/Grafana` 观测数据
- `ProxySQL` 接入后的读写路径差异

## 当前问题

基于现状，`benchmarks/wrk` 还存在这些缺口：

1. 只覆盖旧的 `POST /adama/order`，没有适配当前推荐入口。
2. 没有区分直连 `order`、经 `gateway`、经 `nginx` 三种链路。
3. 没有适配“先获取一次性 token，再发起秒杀下单”的业务流程。
4. 没有覆盖商品列表、订单列表、地址列表这类读接口。
5. 结果记录器只解析 `Requests/sec`、`Transfer/sec`、`timeout`，缺少延迟分位、错误率、链路标签和版本标签。
6. 没有固定测试前置条件，导致不同机器和不同环境下结果不可比。
7. 没有把压测结果和 Prometheus 指标、Grafana 看板关联起来。

## 改造范围

本轮 benchmark 改造建议分 4 个阶段执行。

### 第一阶段：整理目录与基线场景

目标：先把现有 wrk 资产整理成可直接跑的基线集。

计划项：

1. 重构 `benchmarks/wrk` 目录结构。
2. 将脚本拆成 `read`、`write`、`seckill`、`chain` 四类。
3. 补一个总入口说明文档，明确每个脚本的用途、目标接口、执行方式。
4. 统一参数命名：
   - `BASE_URL`
   - `TARGET`
   - `SCENARIO`
   - `THREADS`
   - `CONNECTIONS`
   - `DURATION`
5. 将旧脚本保留到 `legacy/` 或在 README 中明确标记为旧路径兼容样例。

建议目录：

```text
benchmarks/
  wrk/
    README.md
    scripts/
      read/
      write/
      seckill/
      chain/
    data/
    results/
    cmd/
      record/
```

### 第二阶段：适配当前接口与链路

目标：压测对象从旧接口切到当前项目实际入口。

计划项：

1. 新增直连服务场景：
   - `goods` 商品列表
   - `order` 用户订单列表
   - `user` 地址列表
2. 新增网关场景：
   - `GET /api/v1/goods/list`
   - `GET /api/v1/order/list`
   - `GET /api/v1/user/address/list`
3. 新增 Nginx 场景：
   - `GET /api/v1/...`
   - 验证 `nginx -> gateway -> service` 额外开销
4. 新增秒杀场景：
   - `GET /api/v1/adama/goods/{id}` 获取一次性 token
   - `POST /api/v1/adama/order` 携带 `X-Seckill-Token`
5. 新增对比维度：
   - 直连服务
   - 经过 gateway
   - 经过 nginx
   - 接入 proxysql 前后

说明：

- 秒杀接口不能再用固定 body 直接无限打，因为当前已经有 token 校验和用户维度幂等控制。
- 压测脚本必须支持“先拿 token，再消费 token”的前置动作，否则结果会被业务拦截污染。

### 第三阶段：增强脚本能力与结果记录

目标：让压测结果可以长期沉淀和横向比较。

计划项：

1. 扩展 `benchmarks/wrk/cmd/record` 的解析字段：
   - `avg_latency`
   - `p50`
   - `p90`
   - `p99`
   - `requests_per_sec`
   - `transfer_per_sec`
   - `timeouts`
   - `non_2xx`
   - `socket_errors`
2. 给结果记录补充上下文标签：
   - `scenario`
   - `environment`
   - `entrypoint`
   - `database_mode`
   - `build_ref`
   - `created_at`
3. 输出格式统一为 `jsonl`，保留追加写入能力。
4. 增加按场景聚合的摘要脚本，输出：
   - 同场景最近一次结果
   - 同场景历史最好值
   - 与上一次相比的涨跌幅
5. 给 `main_test.go` 补更多解析样例，覆盖：
   - 普通成功输出
   - 含 socket errors
   - 含 timeout
   - 缺少关键字段时报错

### 第四阶段：接入观测与验收标准

目标：让 benchmark 不只看 wrk 输出，也能结合监控判断瓶颈位置。

计划项：

1. 每轮压测前固定采样这些指标：
   - `server_requests_code_total`
   - `server_requests_seconds_bucket`
   - `adama_task_job_runs_total`
   - MySQL exporter 基础指标
   - Redis exporter 基础指标
   - Kafka consumer lag
2. 每个 benchmark 场景在执行后记录：
   - wrk 原始输出
   - 关键 Prometheus 查询结果
   - 是否触发告警
3. 把 Grafana 3 个基础看板纳入压测观察面：
   - 请求量与错误率
   - 秒杀下单延迟与成功率
   - 补偿任务执行次数与失败次数
4. 定义最小验收标准：
   - 功能正确：请求成功率满足阈值
   - 性能可比：同一环境重复执行波动可接受
   - 可追溯：每次压测都有场景、入口、配置、结果记录

## 建议优先落地的场景

优先顺序如下：

1. 商品列表读接口基线压测
原因：最简单，先打通 `service / gateway / nginx` 三层链路对比。

2. 用户订单列表读接口压测
原因：能顺带观察 `order` 查询链路和后续 ProxySQL 读流量验证。

3. 秒杀详情接口压测
原因：只读接口，但已经带业务窗口校验，能较早暴露热点问题。

4. 秒杀下单双阶段压测
原因：最贴近核心链路，但脚本复杂度最高，应放在前面基础链路稳定后执行。

5. 地址 CRUD 中的列表与新增压测
原因：用于补全普通业务接口的写入开销样本。

## 执行前置条件

为了保证 benchmark 结果可比，执行前建议固定这些前置条件：

1. 使用 `docker compose` 启动统一环境。
2. 固定数据库初始数据规模，至少包括：
   - 商品列表数据
   - 秒杀商品数据
   - 用户地址数据
   - 订单样本数据
3. 固定压测入口：
   - 直连服务时明确端口
   - 走网关时明确 `8080`
   - 走 nginx 时明确 `80`
4. 固定 wrk 参数模板，避免每次人工拼命令。
5. 压测前清理上一次生成的 token、幂等键、历史结果缓存。

## 输出物

这个改造计划完成后，`benchmarks/` 至少应产出这些内容：

1. 新版 benchmark README。
2. 多场景 wrk 脚本。
3. 可扩展的结果记录器。
4. 标准化结果目录。
5. 基于当前项目链路的 benchmark 执行说明。
6. 至少一份基线历史结果样本。

## 建议实施顺序

按下面顺序推进最稳妥：

1. 重构 `benchmarks/wrk` 目录和 README。
2. 先补读接口场景：商品列表、订单列表、地址列表。
3. 再补 `gateway` 和 `nginx` 链路版本。
4. 再补秒杀详情取 token 场景。
5. 最后补秒杀下单双阶段脚本和结果记录增强。
6. 收尾时再接 Prometheus 查询与 Grafana 对照说明。

## 非目标

这一轮 benchmark 改造先不做这些事情：

- 不引入 k6、Locust、JMeter 等新工具替换 wrk。
- 不直接做分布式压测平台。
- 不做自动扩缩容测试。
- 不做线上压测方案。
- 不把 benchmark 直接接进 CI 的必跑流水线。

## 下一步建议

如果按这个计划继续实施，下一步应先做：

1. 重写 `benchmarks/wrk/README.md`
2. 新建分场景脚本目录
3. 先落商品列表、订单列表、地址列表三个读接口 benchmark
