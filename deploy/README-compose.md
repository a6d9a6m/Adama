# Compose 使用说明

## 基础环境

启动本地全链路联调环境：

```bash
docker compose up -d
```

包含：

- mysql
- redis
- kafka
- etcd
- nacos
- dtm
- user
- goods
- order
- task
- gateway
- ops-ai

## 开发覆盖

开发模式叠加：

```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d
```

## 监控增强

叠加 Prometheus 和 Grafana：

```bash
docker compose -f docker-compose.yml -f docker-compose.obs.yml up -d
```

包含：

- Prometheus 抓取服务 `/metrics`
- MySQL / Redis / Kafka / Nginx exporter
- Grafana 预置数据源与基础看板
- Prometheus 最小告警规则

## Nginx 统一入口

启动 Nginx 反向代理：

```bash
docker compose up -d nginx
```

## ProxySQL 验证

叠加 ProxySQL 和只读副本：

```bash
docker compose -f docker-compose.yml -f docker-compose.proxysql.yml up -d
```

默认端口：

- ProxySQL 管理端口：`6032`
- ProxySQL 业务端口：`6033`

当前配置只提供“本地验证读写路由”的最小骨架。MySQL 主从复制仍需在容器启动后补充 `CHANGE REPLICATION SOURCE TO`、`START REPLICA` 等初始化步骤。
