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

## Nginx 统一入口

启动 Nginx 反向代理：

```bash
docker compose up -d nginx
```
