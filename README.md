# task274-replaydivergence

处理器指令重放状态分歧定位服务：从参考/待测双执行轨迹中定位首次架构状态分歧及其读写依赖链。

## 业务背景

微架构验证中，待测实现（如 RTL 模拟器）最终寄存器状态与参考实现（黄金模型）不一致时，验证工程师需要确定**最早由哪条重放指令触发**。本服务持久化双轨迹与检查点，同步对齐后按检查点回放状态并计算指纹，比较差异后沿读写依赖回溯首次分歧，最终发布不可变定位快照。

## 核心概念

- **重放批次**：`receiving → syncing → pending_loc → confirmed → sealed`，封存后只读。
- **执行事件**：指令序号（幂等键）、PC、操作码、读/写资源（`reg:rax` / `mem:0x1000`）、写入值；状态 `raw → aligned → divergent → missing → excluded`。
- **检查点**：工程师锚定的已知状态（seq + 期望值），可标记不可信以锁定比较范围。
- **状态指纹**：排序后的 (资源, 值) 对做 FNV-1a 迭代哈希（`fnv1a-sorted`）。
- **读写依赖**：待测指令读资源 → 参考轨迹最近写者，构成依赖边；回溯沿边上游定位首次分歧。
- **定位快照**：`draft → published → superseded`，冻结比较配置与定位证据。

## 标准命令

```bash
# 构建 / 静态检查 / 单元测试
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...

# 端到端自检（含重启恢复验证，退出码 0 即通过）
go run ./cmd/replaydivergence --smoke-test

# 启动服务（SQLite 落盘，WAL 模式）
go run ./cmd/replaydivergence --addr :8080 --db ./task274-replaydivergence.db
```

## 典型调用链

```
POST /api/batches                      → 创建批次（receiving）
POST /api/batches/{id}/events          → 导入 reference / under_test 轨迹（幂等）
POST /api/batches/{id}/checkpoints     → 登记检查点
POST /api/batches/{id}/sync            → 对齐（receiving → syncing）
POST /api/batches/{id}/fingerprints/scan → 检查点状态指纹
POST /api/batches/{id}/compare         → 比较（syncing → pending_loc）
POST /api/batches/{id}/divergences/{did}/trace → 回溯首次分歧（→ confirmed）
POST /api/batches/{id}/snapshots       → 快照草稿
POST /api/batches/{id}/snapshots/{sid}/publish → 发布并封存批次
GET  /api/batches/{id}/stats           → 统计
```

## 技术栈

- Go 1.26.3（GOTOOLCHAIN=local，CGO_ENABLED=0）
- SQLite via `modernc.org/sqlite` v1.52.0（纯 Go，离线可构建；组件版本见 `component-versions.json`）
- HTTP 路由 `net/http`（Go 1.22+ 方法路由），统一 `/api` 前缀

## 持久化与恢复

所有数据落盘 SQLite（WAL 模式）。`--smoke-test` 会真实关闭并重新打开数据库，验证批次状态、分歧定位与快照在重启后完整恢复。
