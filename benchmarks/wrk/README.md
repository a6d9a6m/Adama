# WRK Benchmark Suite

## Layout

```text
benchmarks/wrk/
  README.md
  legacy/
  data/
  results/
  scripts/
    read/
    write/
    seckill/
    chain/
  cmd/
    record/
    report/
```

## Prerequisites

1. Start the local stack with `docker compose`.
2. Make sure there is baseline data for goods, orders, addresses, and seckill goods.
3. Install `wrk` from the official `wg/wrk` source project or a trusted native package.
4. Use the correct entrypoint URL:
   - service: direct service port
   - gateway: `http://127.0.0.1:8080`
   - nginx: `http://127.0.0.1`

## Common Inputs

The scripts use environment variables instead of hard-coded paths.

- `TARGET`: `service`, `gateway`, or `nginx`
- `USER_ID`: user id for list and write scenarios, default `88`
- `GOODS_ID`: seckill goods id, default `1`
- `PAGE`: default `1`
- `PAGE_SIZE`: default `10`
- `KEYWORD`: optional goods keyword

The wrk runtime parameters remain standard CLI arguments:

- `-t` threads
- `-c` connections
- `-d` duration
- `-T` timeout

## Read Scenarios

Direct goods service:

```bash
TARGET=service wrk -t4 -c32 -d15s -T2s --script=benchmarks/wrk/scripts/read/goods_list.lua http://127.0.0.1:8003
```

Through gateway:

```bash
TARGET=gateway wrk -t4 -c32 -d15s -T2s --script=benchmarks/wrk/scripts/read/goods_list.lua http://127.0.0.1:8080
```

Through nginx:

```bash
TARGET=nginx wrk -t4 -c32 -d15s -T2s --script=benchmarks/wrk/scripts/read/goods_list.lua http://127.0.0.1
```

Other read scenarios:

- `benchmarks/wrk/scripts/read/order_list.lua`
- `benchmarks/wrk/scripts/read/address_list.lua`

## Write Scenarios

Address create:

```bash
TARGET=gateway USER_ID=88 wrk -t2 -c8 -d10s -T2s --script=benchmarks/wrk/scripts/write/address_create.lua http://127.0.0.1:8080
```

## Seckill Scenarios

Goods detail only:

```bash
TARGET=gateway USER_ID=88 GOODS_ID=1 wrk -t2 -c8 -d10s -T2s --script=benchmarks/wrk/scripts/seckill/goods_detail.lua http://127.0.0.1:8080
```

Two-step token + create order flow:

```bash
TARGET=gateway USER_ID=88 GOODS_ID=1 wrk -t2 -c8 -d10s -T2s --script=benchmarks/wrk/scripts/seckill/order_flow.lua http://127.0.0.1:8080
```

This flow alternates:

1. `GET /adama/goods/{id}` to fetch `seckill_token`
2. `POST /adama/order` with `X-Seckill-Token`

## Matrix Runner

The PowerShell helper runs the same read scenario across service, gateway, and nginx:

```powershell
powershell -ExecutionPolicy Bypass -File .\benchmarks\wrk\scripts\chain\run-read-matrix.ps1 -Scenario goods_list
```

Supported scenarios:

- `goods_list`
- `order_list`
- `address_list`

## Result Recording

Record a wrk output file to `jsonl`:

```bash
go run ./benchmarks/wrk/cmd/record -input .\tmp\goods-list.txt -scenario goods_list -env local -entrypoint gateway -db-mode primary
```

Generate a simple summary:

```bash
go run ./benchmarks/wrk/cmd/report -input benchmarks/wrk/results/history.jsonl -scenario goods_list
```

## Output Convention

The scripts append custom lines so the recorder can parse them:

- `Non-2xx responses: N`
- `Socket errors: connect X, read Y, write Z, timeout N`

The recorder stores:

- throughput
- average latency
- p50/p90/p99
- transfer rate
- timeout count
- socket errors
- non-2xx count
- scenario metadata

## Legacy Scripts

The previous ad hoc scripts are kept under `benchmarks/wrk/legacy/`:

- `legacy/post.lua`
- `legacy/post.json.lua`
