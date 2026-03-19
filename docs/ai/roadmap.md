# 运维 AI 设计与开发规划

## 1. 目标定位

当前项目里的 `ops-ai` 已经具备第一阶段的本地 RAG 能力：

- 检索 `FQA.md`、`scheduled.md`、`docs/`、`benchmarks/`
- 查询订单工作流、库存预留、最近错误
- 可选调用 OpenAI `Responses API`

但它还不是面向故障分析的运维 AI。要实现目标能力，系统应从“本地 FAQ / 文档问答”升级为“日志与观测驱动的故障分析助手”，重点补齐三层能力：

1. 异常聚类 / 症状总结
2. 日志到结构化症状对象
3. 基于通用知识和项目知识做 RCA / next-step recommendation

## 2. 设计结论

结合 Drain / Drain3、Elastic log categorization、OpenObserve log patterns、日志解析综述、Datadog Bits AI SRE、Elastic AI Assistant 和 Salesforce RCA 知识挖掘路线，当前项目最适合的实现方式不是“直接把日志喂给大模型”，而是先建立一条分层流水线：

1. 采集原始日志
2. 解析为模板和参数
3. 在时间窗口内聚成异常簇
4. 生成结构化症状对象
5. 把症状对象与 Prometheus、MySQL、Redis、Kafka 等运行态证据合并
6. 最后才让模型做假设、验证和结论生成

也就是说，模型只负责“推断”，不负责“替代日志工程”。

## 3. 为什么当前项目要这样做

### 3.1 当前项目现状

- 已有 `ops-ai` 服务，但主要依赖本地文档检索和少量数据库只读查询。
- 已有 Prometheus / Grafana / exporter，但没有统一日志采集、日志存储和日志模式分析链路。
- 服务日志目前主要还在容器 stdout/stderr 中，缺少可重放、可聚类、可窗口统计的日志输入面。
- 当前故障热点集中在秒杀链路、`task-worker` 异步消费、TCC 分支和数据库锁竞争，这些问题都非常适合“模式聚类 + 症状对象 + RCA”路径。

### 3.2 为什么不直接做“大模型总结日志”

如果直接让模型读原始日志，会有几个问题：

- 日志量太大，窗口成本不可控
- 重复日志太多，模型会被噪音淹没
- 缺少结构化字段，难以做统计异常和频次对比
- 结论缺少可验证的中间证据
- 没法形成稳定的历史知识资产

因此，必须先把“日志行”压缩成“模板 / 异常簇 / 症状对象”。

## 4. 目标架构

### 4.1 总体分层

建议把运维 AI 拆成 4 层：

1. 日志输入层
2. 症状抽取层
3. 证据汇聚层
4. 推理与建议层

### 4.2 日志输入层

首版不建议直接引入 Elastic/OpenObserve 作为主依赖，而是优先复用当前已有栈：

- Docker Compose
- Kafka
- MySQL
- Prometheus
- 现有 `ops-ai`

推荐做法：

- 新增日志采集器，把 `gateway`、`goods`、`order`、`task-worker`、`task-timeout`、`task-scheduler`、`ops-ai` 的容器 stdout/stderr 采集出来
- 采集器可选 `Vector` 或 `Fluent Bit`
- 首版统一写入 Kafka 新 topic，例如 `ops_ai_logs_raw`

这样做的好处是：

- 和当前项目现有 Kafka 体系兼容
- 可以回放日志
- 可以异步扩展解析器
- 不要求首版就接外部观测平台

### 4.3 症状抽取层

建议新增独立后台组件，例如：

- `ops-ai-log-worker`

职责：

- 消费 `ops_ai_logs_raw`
- 清洗日志行
- 做模板抽取
- 窗口聚合
- 产出症状对象

#### 4.3.1 模板抽取

首版建议直接采用成熟路线，而不是自写解析算法：

- 方案优先级 1：接入 Drain3 作为模板抽取器
- 方案优先级 2：离线评估 `logpai/logparser`
- 方案优先级 3：后续再考虑 LLM-assisted parsing

当前项目更适合的工程实现是：

- 先以独立 Python sidecar / service 方式接入 Drain3
- `ops-ai-log-worker` 通过本地 RPC / HTTP 调用解析服务
- 不把 Drain3 逻辑硬写进现有 Go 服务里

原因：

- Drain3 已经成熟，支持在线模板抽取、流式更新、持久化和参数提取
- 当前项目重点在落地，不在重复发明解析器
- 首版要的是“可用”和“可解释”

#### 4.3.2 首版输出的结构化日志事件

每条日志至少转成以下字段：

- `timestamp`
- `service`
- `instance`
- `level`
- `trace_id`
- `request_id`
- `raw_message`
- `template_id`
- `template_text`
- `params`
- `event_type`
- `error_code`
- `error_class`

其中：

- `template_id / template_text / params` 来自模板抽取
- `event_type / error_code / error_class` 来自规则归一化

#### 4.3.3 症状对象

这一步不是直接给人看日志，而是给 AI 推理使用。建议定义标准症状对象：

- `symptom_id`
- `window_start`
- `window_end`
- `service`
- `severity`
- `template_id`
- `template_text`
- `error_class`
- `count`
- `baseline_count`
- `delta_ratio`
- `affected_trace_ids`
- `affected_request_ids`
- `sample_messages`
- `related_metrics`
- `related_entities`
- `first_seen_at`
- `last_seen_at`

示例：

```json
{
  "service": "goods",
  "severity": "high",
  "error_class": "mysql_deadlock",
  "template_text": "Error 1213: Deadlock found when trying to get lock; try restarting transaction",
  "count": 203,
  "baseline_count": 0,
  "delta_ratio": 203,
  "related_entities": {
    "goods_id": ["1"],
    "operation": ["CreateOrdersTccTry"]
  }
}
```

### 4.4 异常聚类与模式检测

结合当前项目特点，首版不做复杂无监督模型，先做三类检测：

1. 新模板异常
   - 最近窗口首次出现、历史未见或极少见

2. 模板频次异常
   - 当前 1m / 5m 计数相对过去窗口显著上升

3. 参数热点异常
   - 同一模板下，某个 `goods_id`、`order_id`、`status`、`error_code` 突然集中爆发

这个阶段只要能稳定回答这些问题，就已经很有价值：

- 哪类异常在增长
- 哪个服务最异常
- 哪个商品 / 哪个订单 / 哪个工作流最集中
- 新异常模板是什么

### 4.5 证据汇聚层

`ops-ai` 不应该只看日志，还要拉运行态证据。当前项目建议接入以下工具：

- Prometheus 查询（住：promethus通过docker可以找到，本地有相关image）
  - 请求量、错误率、延迟、`task` 任务指标、Kafka exporter 指标

- MySQL 查询
  - 工作流状态、库存预留、最近错误、聚合计数

- Redis 查询
  - 秒杀库存 key、售罄标记、用户抢购集合、热点 key 状态

- Kafka 查询
  - topic lag、partition 积压、consumer group 消费速率

- Docker / 容器查询
  - 容器状态、最近 N 行日志、重启次数

其中关键原则是：

- 模型只调用只读工具
- 先读证据，再做推断
- 不允许直接执行写操作和修复动作

### 4.6 推理与建议层

推理层要采用“假设驱动”而不是“摘要驱动”。

目标流程：

1. 根据问题和告警类型生成初始假设
2. 针对每个假设发起定向查询
3. 用证据支持或否定假设
4. 输出最可能根因、影响面和下一步动作

建议输出结构：

- `summary`
- `top_hypotheses`
- `supporting_evidence`
- `rejected_hypotheses`
- `likely_root_cause`
- `next_steps`
- `confidence`

### 4.7 模型策略

当前项目里已经接了 `Responses API`，首版继续复用即可。

建议策略：

- 默认在线模型：`gpt-5-mini`
  - 适合问答、症状归纳、轻量 RCA

- 深度 RCA / 事故复盘：可选升级为更强模型
  - 由调用参数显式指定

- 无 API Key 时：
  - 保留本地规则化 fallback
  - 但只输出“文档结论 / 规则结论”，不伪装成模型推断

## 5. 与当前项目结合后的实现边界

### 5.1 第一阶段不做的事

- 不做自动修复
- 不做写库或执行 shell
- 不直接让模型处理全量原始日志
- 不首版就引入多套外部观测平台
- 不把整个系统改成多 agent

### 5.2 第一阶段重点做的事

- 日志统一采集
- 模板抽取和模式聚类
- 症状对象沉淀
- 读工具接入
- 假设驱动推理

## 6. 存储设计建议

建议新增两类表：

### 6.1 结构化日志事件表

- `ops_ai_log_events`

字段建议：

- `id`
- `ts`
- `service`
- `level`
- `trace_id`
- `request_id`
- `template_id`
- `template_text`
- `params_json`
- `event_type`
- `error_class`
- `raw_message`

### 6.2 症状对象表

- `ops_ai_symptoms`

字段建议：

- `id`
- `window_start`
- `window_end`
- `service`
- `severity`
- `template_id`
- `error_class`
- `count`
- `baseline_count`
- `delta_ratio`
- `sample_messages_json`
- `entities_json`
- `metrics_json`
- `status`

这两层分开后：

- 事件表解决“可追溯”
- 症状表解决“可推理”

## 7. API 设计建议

建议保留当前 `/ops-ai/ask`，但扩展成三类调用模式：

1. `question_answer`
   - 文档问答 / FAQ / 运行态问答

2. `incident_analysis`
   - 输入时间范围、服务、异常标签
   - 输出根因候选与证据

3. `symptom_summary`
   - 输入时间窗口
   - 输出近 5m / 15m 症状聚类摘要

同时补两个内部接口：

- `/ops-ai/internal/symptoms/recent`
- `/ops-ai/internal/incidents/analyze`

## 8. 开发规划

### 步骤 1：补齐日志输入面

1. 在 Compose 中新增日志采集器
2. 把核心服务 stdout/stderr 统一采集
3. 新增 Kafka topic `ops_ai_logs_raw`
4. 统一日志基础字段：`service`、`level`、`trace_id`、`request_id`

### 步骤 2：实现日志模板抽取服务

1. 以独立服务方式接入 Drain3
2. 支持在线模板学习
3. 支持返回 `template_id / template_text / params`
4. 支持模板状态持久化

### 步骤 3：实现日志事件归一化

1. 新增 `ops-ai-log-worker`
2. 消费 `ops_ai_logs_raw`
3. 调用 Drain3 解析日志
4. 生成结构化日志事件
5. 落库到 `ops_ai_log_events`

### 步骤 4：实现异常聚类与窗口分析

1. 按 `service + template_id + level + error_class` 聚合
2. 计算 1m / 5m / 15m 计数
3. 计算历史基线和增幅
4. 识别新模板、频次异常、参数热点异常
5. 生成 `ops_ai_symptoms`

### 步骤 5：补齐运行态工具

1. 新增 Prometheus 查询工具
2. 新增 Redis 查询工具
3. 新增 Kafka lag 查询工具
4. 新增容器状态与最近日志查询工具
5. 保持所有工具只读

### 步骤 6：扩展 `ops-ai` 检索层

1. 把 `ops_ai_symptoms` 接入检索
2. 把结构化日志事件接入检索
3. 调整证据优先级：症状对象 > 运行态查询 > 文档 > FAQ
4. 按问题类型路由不同工具集

### 步骤 7：实现假设驱动 RCA

1. 为 `incident_analysis` 设计固定输出 schema
2. 先生成根因假设
3. 再发起定向工具查询
4. 输出支持证据与排除证据
5. 生成 likely root cause 和 next steps

### 步骤 8：沉淀项目私有知识

1. 把历史 benchmark、FAQ、FQA、scheduled、故障记录统一纳入知识库
2. 为 FAQ / FQA 建立编号和标签
3. 为历史事故沉淀标准 RCA 记录
4. 建立“症状 -> 根因 -> 动作”的知识样本

### 步骤 9：补齐验证集

1. 基于现有已知问题构造测试集
2. 覆盖 MySQL 1040、1213、Kafka 积压、秒杀限流、超时补偿失败等场景
3. 评估症状抽取准确率
4. 评估 RCA 命中率与 next-step 可用性

### 步骤 10：再决定是否引入外部观测平台增强

1. 评估是否接入 OpenObserve / Elastic 做日志模式与可视化增强
2. 评估是否需要向量检索与嵌入模型
3. 评估是否需要更强模型处理复杂 RCA

## 9. 参考依据

- Drain：在线日志模板抽取，适合作为首版模板解析基础
- Drain3：支持流式模板学习、参数提取和持久化，适合工程接入
- Elastic log categorization：说明“日志类别 + 计数异常”是成熟产品路径
- OpenObserve log patterns：说明“模式提取 + 频次观察”非常适合运维首屏
- System Log Parsing: A Survey：说明日志解析仍然是自动化日志分析的基础环节
- LoFI：说明故障诊断应关注 fault-indicating descriptions 和 parameters
- Datadog Bits AI SRE：说明 RCA 应采用假设驱动而不是摘要驱动
- Elastic AI Assistant：说明知识库、函数调用和运行态数据结合是可行路径
- Salesforce RCA knowledge mining：说明历史事故知识库对 RCA 非常重要
