# Gateway 设计

> 本文档说明当前 `app/gateway` 的职责边界、路由暴露面、限流规则、上下文透传方式，以及它和 `nginx` 的分工。

## 角色定位

当前系统里，`gateway` 不是通用 API 网关平台，而是一个面向本仓库业务链路的轻量反向代理层。

它主要承担 4 件事：

1. 统一对外 HTTP 入口
2. 控制哪些路径允许暴露，哪些必须拦截
3. 为秒杀路径增加应用层限流
4. 透传请求上下文和追踪头，方便下游服务感知调用来源

它不承担的职责包括：

- JWT 鉴权
- 聚合查询
- 熔断降级
- 复杂风控
- 直接提供业务数据存储

这些能力后续可以继续扩展，但当前主线代码里还没有正式落地。

## 请求路径

当前公网请求优先经过：

```text
Client -> Nginx -> Gateway -> User/Goods/Order
```

其中：

- `nginx` 承担统一公网入口和前置限流
- `gateway` 承担业务暴露面和应用层限流
- 下游服务只接收已经过网关整理的请求头

## 服务注册方式

`gateway` 启动时会为 3 个下游服务创建独立 upstream：

- `user`
- `goods`
- `order`

每个 upstream 都支持：

- 独立 `base_url`
- 独立超时配置
- 统一的 HTTP transport

因此 `gateway` 当前更接近“配置驱动的业务路由器”，而不是动态发现式代理。

## 对外前缀

`gateway` 只处理 `/api/v1/` 前缀下的请求。

如果请求不在这个前缀内：

- 直接返回 `route_not_found`

这条规则的价值是：

- 避免网关被误用成任意代理
- 强制对外 API 有统一版本前缀

## 路由暴露范围

当前显式放行的业务前缀如下：

| 前缀 | 下游 |
|:---|:---|
| `/user` | `user` |
| `/goods` | `goods` |
| `/order` | `order` |
| `/adama/goods` | `order` |
| `/adama/order` | `order` |

这里要特别注意：

- 秒杀详情和秒杀下单都挂在 `order` 服务上
- `goods` 服务并不直接暴露秒杀详情公网入口

## 显式拦截的路径

### 精确拦截

以下路径即使存在，也不会被网关向外暴露：

- `/adama/orderTry`
- `/adama/orderConfirm`
- `/adama/orderCancel`
- `/ordersTry`
- `/ordersConfirm`
- `/ordersCancel`

### 前缀拦截

以下前缀被统一视为内部路径：

- `/adama/tcc`
- `/internal`
- `/task`
- `/compensate`

这部分设计的核心目的，是避免外部请求直接打到：

- TCC 分支接口
- 内部补偿接口
- 任务接口
- 内部事务入口

## 上下文透传

`gateway` 在转发请求前，会确保并透传以下请求头：

- `X-Request-Id`
- `X-Trace-Id`
- `X-Forwarded-For`
- `X-Forwarded-Proto`
- `X-Forwarded-Host`
- `X-Forwarded-Uri`
- `X-Forwarded-Prefix`

如果上游没传：

- `X-Request-Id` 会自动生成
- `X-Trace-Id` 默认复用 `X-Request-Id`

这层透传让下游服务能稳定读取：

- 请求标识
- 链路标识
- 原始协议与来源
- 网关前缀

对秒杀路径而言，还会继续依赖：

- `X-User-Id`
- `X-Seckill-Token`

其中 `X-User-Id` 当前仍由调用方或更前置的一层传入，`gateway` 只负责继续透传，而不负责校验。

## 错误语义

`gateway` 当前统一返回 JSON 错误结构：

```json
{
  "code": "route_not_found",
  "message": "route not found"
}
```

当前主要错误类型包括：

- `route_not_found`
- `route_forbidden`
- `upstream_unavailable`
- `gateway_token_rate_limited`
- `gateway_order_rate_limited`
- `user_goods_order_rate_limited`

## 限流设计

当前秒杀限流分为两层。

### 第一层：Nginx 前置限流

`nginx` 对两个热点入口单独做了限流：

- `GET /api/v1/adama/goods/{id}`
- `POST /api/v1/adama/order`

并优先使用：

- `X-User-Id`

作为限流 key；没有时退化到客户端地址。

### 第二层：Gateway 应用层限流

`gateway` 内部还做了 3 组限流。

#### 1. token 获取限流

命中范围：

- `GET /api/v1/adama/goods/{id}`

配置项：

- `GATEWAY_TOKEN_RPS`
- `GATEWAY_TOKEN_BURST`

#### 2. 秒杀下单总量限流

命中范围：

- `POST /api/v1/adama/order`

配置项：

- `GATEWAY_ORDER_RPS`
- `GATEWAY_ORDER_BURST`

#### 3. 用户维度商品限流

命中范围：

- `POST /api/v1/adama/order`

限流 key 组成：

- 优先 `X-User-Id`
- 其次 `X-Forwarded-For`
- 再次 `RemoteAddr`
- 再拼接请求体中的 `gid`

最终效果是：

- 同一用户针对同一商品的秒杀下单有独立限流预算

配置项：

- `GATEWAY_USER_GOODS_ORDER_RPS`
- `GATEWAY_USER_GOODS_ORDER_BURST`

## 当前实现的边界

需要明确几个现实边界：

1. `gateway` 负责透传用户头，不负责鉴权和签发用户身份。
2. 路由是静态配置，不是运行时动态注册。
3. 秒杀限流只覆盖热点秒杀路径，不覆盖全部业务接口。
4. `gateway` 是应用层代理，不替代 `nginx` 的公网入口职责。

## 和 Nginx 的分工

当前推荐理解如下：

| 组件 | 主要职责 |
|:---|:---|
| `nginx` | 统一入口、前置限流、反向代理、状态页 |
| `gateway` | 业务暴露面控制、应用层路由、上下文透传、应用层秒杀限流 |

这样分工的好处是：

- `nginx` 保持简单稳定
- 业务相关暴露面规则收敛在 Go 代码里
- 秒杀限流既能在边缘层拦截，也能在应用层做更细粒度控制

## 测试关注点

后续补测试时，`gateway` 最值得覆盖的点有：

1. 路由只暴露允许的前缀
2. 内部路径确实被拦截
3. 限流只命中秒杀相关路径
4. `X-Request-Id`、`X-Trace-Id` 等头能正确透传
5. `POST /adama/order` 的用户维度限流不会误伤其他路径

## 相关文档

- [system-overview.md](system-overview.md)
- [seckill-mainline.md](seckill-mainline.md)
- [deploy/README-compose.md](../../deploy/README-compose.md)
