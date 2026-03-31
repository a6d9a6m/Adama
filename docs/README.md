# Docs Index

`docs/` 现在按常见开源项目的文档分层收敛为 `architecture / reference / operations` 主结构，辅以少量索引文件；临时计划、AI review、测试矩阵等私有工作文档不再纳入 git。

当前约定：

- 根目录 [FQA.md](../FQA.md) 保留为常见问题与排查沉淀
- 根目录 [scheduled.md](../scheduled.md) 保留为当前项目进度清单
- `docs/architecture/` 放系统结构、核心链路和事务设计
- `docs/reference/` 放存储、缓存、消息等静态参考资料
- `docs/operations/` 放运行、部署、观测和联调入口

目录说明：

- [architecture/system-overview.md](architecture/system-overview.md)
  系统总览、服务边界、调用关系与阅读顺序

- [architecture/README.md](architecture/README.md)
  architecture 目录索引

- [architecture/seckill-mainline.md](architecture/seckill-mainline.md)
  当前秒杀主链路的同步、异步、TCC 与补偿流程

- [architecture/order-workflow.md](architecture/order-workflow.md)
  订单工作流状态机、字段语义与收敛路径

- [architecture/gateway.md](architecture/gateway.md)
  gateway 的暴露面控制、限流、上下文透传与 Nginx 分工

- [architecture/transactions/distributed-transaction.md](architecture/transactions/distributed-transaction.md)
  分布式事务资料与设计说明

- [architecture/transactions/redis-first-tcc.md](architecture/transactions/redis-first-tcc.md)
  Redis 主冻结、SQL 账本化的 TCC 演进方案

- [reference/mysql-structure.md](reference/mysql-structure.md)
  MySQL 结构设计

- [reference/redis-key.md](reference/redis-key.md)
  Redis key 设计

- [reference/kafka.md](reference/kafka.md)
  Kafka 相关设计

- [reference/README.md](reference/README.md)
  reference 目录索引

- [operations/README.md](operations/README.md)
  本地运行、部署编排、观测和压测入口
