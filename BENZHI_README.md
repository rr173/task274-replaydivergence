# BENZHI 评测说明

基于 Go 实现的处理器指令重放状态分歧定位后端服务，一款后端服务，完成双执行轨迹导入与检查点锚定、状态指纹比较与首次分歧依赖回溯、读写依赖链固化与不可变定位快照发布。

## 启动

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/replaydivergence --addr :8080 --db task274-replaydivergence.db
```

## 自检（不启动长驻服务）

```bash
go run ./cmd/replaydivergence --smoke-test
```

`--smoke-test` 会真实创建重放批次、导入双轨迹、登记检查点、同步对齐、扫描指纹、比较分歧、回溯首次分歧、发布定位快照，关闭并重新打开数据库验证持久化与重启恢复，最后以 0 退出码结束。

## 构建门禁

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
go run ./cmd/replaydivergence --smoke-test
```

## HTTP API（前缀 /api）

批次：`POST /api/batches`、`GET /api/batches`、`GET /api/batches/{id}`、`POST /api/batches/{id}/sync`、`POST /api/batches/{id}/confirm`、`POST /api/batches/{id}/seal`、`GET /api/batches/{id}/stats`
轨迹：`POST /api/batches/{id}/events`、`GET /api/batches/{id}/events`、`PATCH /api/batches/{id}/events/{eid}/exclude`
检查点：`POST /api/batches/{id}/checkpoints`、`GET /api/batches/{id}/checkpoints`、`POST /api/batches/{id}/checkpoints/{cid}/mark-unreliable`
指纹与比较：`POST /api/batches/{id}/fingerprints/scan`、`GET /api/batches/{id}/fingerprints`、`POST /api/batches/{id}/compare`、`GET /api/batches/{id}/divergences`、`POST /api/batches/{id}/divergences/{did}/trace`、`GET /api/batches/{id}/divergences/{did}/chain`
快照：`POST /api/batches/{id}/snapshots`、`GET /api/batches/{id}/snapshots`、`POST /api/batches/{id}/snapshots/{sid}/publish`、`GET /api/snapshots/{sid}`
健康与自检：`GET /api/health`、`GET /api/selfcheck`

## 持久化

SQLite（modernc.org/sqlite，CGO 无关）。建表：replay_batches、exec_events、checkpoints、state_fingerprints、rw_dependencies、divergences、divergence_chain_edges、loc_snapshots。事件 (batch_id, side, seq) 幂等；指纹 (batch_id, side, seq, scope) upsert；已发布定位快照的配置与摘要不可改写。
