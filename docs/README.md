# Docs Index

`docs/` 按“规划、架构、AI 使用、性能沉淀”拆分。

当前约定：

- 根目录 [FQA.md](/d:/CCY/Code/Go/adama/FQA.md) 保留为常见问题与排查沉淀
- 根目录 [scheduled.md](/d:/CCY/Code/Go/adama/scheduled.md) 保留为当前项目进度清单
- `docs/plans/` 放实施规划与测试规划
- `docs/architecture/` 放系统结构、存储、消息和事务设计
- `docs/ai/` 放 AI 使用方式、运维 AI 路线和后续 AI 产出目录

目录说明：

- [plans/project-plan.md](/d:/CCY/Code/Go/adama/docs/plans/project-plan.md)
  项目实施规划与阶段目标

- [plans/benchmark-plan.md](/d:/CCY/Code/Go/adama/docs/plans/benchmark-plan.md)
  benchmark / wrk 测试套件改造规划

- [plans/documentation-plan.md](/d:/CCY/Code/Go/adama/docs/plans/documentation-plan.md)
  文档专项完善计划与执行顺序

- [architecture/system-overview.md](/d:/CCY/Code/Go/adama/docs/architecture/system-overview.md)
  系统总览、服务边界、调用关系与阅读顺序

- [architecture/seckill-mainline.md](/d:/CCY/Code/Go/adama/docs/architecture/seckill-mainline.md)
  当前秒杀主链路的同步、异步、TCC 与补偿流程

- [architecture/order-workflow.md](/d:/CCY/Code/Go/adama/docs/architecture/order-workflow.md)
  订单工作流状态机、字段语义与收敛路径

- [architecture/gateway.md](/d:/CCY/Code/Go/adama/docs/architecture/gateway.md)
  gateway 的暴露面控制、限流、上下文透传与 Nginx 分工

- [architecture/storage/mysql-structure.md](/d:/CCY/Code/Go/adama/docs/architecture/storage/mysql-structure.md)
  MySQL 结构设计

- [architecture/storage/redis-key.md](/d:/CCY/Code/Go/adama/docs/architecture/storage/redis-key.md)
  Redis key 设计

- [architecture/messaging/kafka.md](/d:/CCY/Code/Go/adama/docs/architecture/messaging/kafka.md)
  Kafka 相关设计

- [architecture/transactions/distributed-transaction.md](/d:/CCY/Code/Go/adama/docs/architecture/transactions/distributed-transaction.md)
  分布式事务资料与设计说明

- [ai/roadmap.md](/d:/CCY/Code/Go/adama/docs/ai/roadmap.md)
  运维 AI 设计与开发规划

- [ai/usage.md](/d:/CCY/Code/Go/adama/docs/ai/usage.md)
  当前项目里 AI 更适合承担的工作类型

- [ai/reviews/README.md](/d:/CCY/Code/Go/adama/docs/ai/reviews/README.md)
  AI reviewer 风险审查与变更评审记录

- [ai/testing/README.md](/d:/CCY/Code/Go/adama/docs/ai/testing/README.md)
  AI 辅助测试矩阵、补测计划与验证记录

- [ai/knowledge/README.md](/d:/CCY/Code/Go/adama/docs/ai/knowledge/README.md)
  AI 参与整理的架构知识、RCA 和私有知识资产

- [ai/performance/README.md](/d:/CCY/Code/Go/adama/docs/ai/performance/README.md)
  AI 辅助形成的性能基线、瓶颈分析与调优结论
