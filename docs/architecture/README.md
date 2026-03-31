# Architecture

这个目录放系统设计和核心链路文档。

## 内容

- [system-overview.md](system-overview.md)
  系统总览、服务边界和阅读顺序

- [seckill-mainline.md](seckill-mainline.md)
  秒杀主链路、异步消费和补偿流程

- [order-workflow.md](order-workflow.md)
  订单工作流状态机和状态推进规则

- [gateway.md](gateway.md)
  网关、Nginx、限流和上下文透传分工

- [transactions/distributed-transaction.md](transactions/distributed-transaction.md)
  DTM / TCC 背景与分支语义

- [transactions/redis-first-tcc.md](transactions/redis-first-tcc.md)
  Redis 主冻结、SQL 账本化的事务演进方案
