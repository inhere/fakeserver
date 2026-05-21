# Fakeserver v0.4 · Phase 2 — SSE 实时推送 + recorder.Subscribe 多订阅

> **执行说明**：本计划面向"对 fakeserver 仓库零上下文"的工程师。每步 2–5 分钟，TDD，频繁提交。

**Goal**：让 `/__fakeserver/events` 端点把 ring 收到的每条 Entry 实时推送给所有订阅者；恢复 design §11.3 例子中的 `event: request` 类事件；心跳 15s 防代理超时；慢客户端非阻塞 send（满 → 丢弃最旧 + 累加 `eventsDropped` 计数）。同时为 Phase 3 watcher reload 集成预留 `EmitReload` 接口（Phase 2 仅实现接口 + 测试；watcher 端调用留 Phase 3）。

**Architecture**：

1. **`internal/recorder` 包扩展**：
   - `Subscribe() (<-chan Entry, func())` — 返回事件 channel + unsubscribe 函数；channel 容量 32（非阻塞 send，满则丢弃 + 累加 `eventsDropped`）
   - `EventsDropped() uint64` — 单元测试用计数器观察
   - `Append(e Entry)` 在写 ring 之后向所有订阅者广播
   - `EmitReload(diff ReloadDiff)` — Phase 3 watcher 集成预留；Phase 2 仅提供接口 + 单测
2. **`internal/webui/sse.go`** 新文件：
   - `sseEventsHandler(ring *recorder.Ring) rux.HandlerFunc`
   - 响应头：`Content-Type: text/event-stream` + `Cache-Control: no-cache` + `Connection: keep-alive`
   - `http.Flusher` 立即 flush；心跳 15s 间隔（参数化以便测试用短周期）
   - 客户端断开（`r.Context().Done()`）→ unsubscribe + return
3. **`webui/mount.go` 扩展**：注册 `GET /__fakeserver/events`

**Phase 2 完成定义（DoD）**：

1. `Subscribe` 返回的 channel 在 Unsubscribe 后**不再收到新事件**；多次 Unsubscribe 不 panic
2. 慢客户端满 channel → `EventsDropped` 自增，正常客户端不受影响
3. SSE 端点心跳每 N 间隔发一次 `: ping\n\n`（测试用 100ms 周期注入）
4. SSE 客户端断开 → server 端 unsubscribe 干净（不泄漏 goroutine；通过 `EventsDropped` 不再增长间接验证）
5. `EmitReload(ReloadDiff)` 接口存在，发送 `event: reload` 类事件并经 SSE handler 正确序列化
6. `go build ./...` + 全包绿 + `recorder` 覆盖率 ≥ 85%
7. Phase 1 既有测试不回归（Append/Snapshot 行为完全兼容）
8. bd v0.4 Phase 2 epic 创建并关闭

---

## 文件结构（Phase 2 产出）

| 操作 | 路径 | 职责 |
|---|---|---|
| 修改 | `internal/recorder/recorder.go` | 加 subscribers + EventsDropped + ReloadDiff struct + Subscribe + Unsubscribe + EmitReload |
| 修改 | `internal/recorder/recorder_test.go` | 复用既有 + 不改测试主体 |
| 新建 | `internal/recorder/subscribe_test.go` | Subscribe 多订阅 + Unsubscribe + 慢客户端 + EmitReload |
| 新建 | `internal/webui/sse.go` | SSE handler + 心跳 |
| 新建 | `internal/webui/sse_test.go` | SSE 响应头 + 事件帧格式 + 心跳 + 断开自动 unsub |
| 修改 | `internal/webui/mount.go` | 注册 `/__fakeserver/events` |
| 修改 | `internal/cli/serve_v04_e2e_test.go` | 加 SSE httptest 集成用例（短心跳 + 真 EventSource 风格 client） |

---

## Task 1 — Subscribe / Unsubscribe / EventsDropped + broadcast 改造 Append

**Files**: `internal/recorder/recorder.go` + `internal/recorder/subscribe_test.go`

### Step 1.1 TDD：先写 subscribe_test.go

测试用例：
- 单订阅者收到 N 个 Append
- 多订阅者各自独立收到
- Unsubscribe 后不再收到
- 慢客户端满 → 不阻塞 Append + EventsDropped 自增
- Unsubscribe 后多次调用不 panic
- EmitReload 发送 ReloadDiff 给所有订阅者

### Step 1.2 改造 recorder.go

- `Ring` struct 加：`subscribers map[uint64]*subscriber` + `nextSubID uint64` + `eventsDropped uint64`
- `type subscriber struct { ch chan Event }` （Event = 含 type=request|reload 的判别联合）
- 用 `type Event` 而非直接 `Entry`，让 EmitReload 也走同一 channel；Entry/ReloadDiff 分别作为 Event 子类型
- 简化：`Event { Kind string; Entry *Entry; Reload *ReloadDiff }`
- `Append` 末尾 broadcast `Event{Kind:"request", Entry: &e}`
- `EmitReload(d ReloadDiff)` 广播 `Event{Kind:"reload", Reload: &d}`

### Step 1.3 commit

```
git commit -m "feat(recorder): Subscribe/Unsubscribe + broadcast 改造 + EmitReload + dropped 计数"
```

---

## Task 2 — `internal/webui/sse.go` SSE handler

**Files**: `internal/webui/sse.go` + `internal/webui/sse_test.go`

### Step 2.1 TDD：sse_test.go

测试用例：
- 响应头含 `text/event-stream`
- Append 一条 Entry → SSE 客户端收到 `event: request\ndata: {...}\n\n`
- EmitReload → 客户端收到 `event: reload\ndata: {...}\n\n`
- 心跳：用 100ms 周期，等 250ms → 至少看到 2 个 `: ping\n\n`
- 客户端断开 context.Done → handler 返回 + 后续 Append 不影响其他订阅者

### Step 2.2 实现

```go
func sseEventsHandler(ring *recorder.Ring, heartbeatEvery time.Duration) rux.HandlerFunc { ... }
```

handler 关键路径：
- 写响应头
- `flusher, ok := c.Resp.(http.Flusher); !ok → 500`（防御）
- `events, cancel := ring.Subscribe(); defer cancel()`
- `ticker := time.NewTicker(heartbeatEvery); defer ticker.Stop()`
- `for { select { case ev := <-events: 序列化 + flush; case <-ticker.C: 写 ": ping\n\n" + flush; case <-c.Req.Context().Done(): return } }`

事件序列化函数：
- `request` → `event: request\ndata: <json of Entry>\n\n`
- `reload` → `event: reload\ndata: <json of ReloadDiff>\n\n`

### Step 2.3 mount.go 扩展

```go
r.GET("/__fakeserver/events", sseEventsHandler(ring, 15*time.Second))
```

测试用短心跳：测试函数注入参数化 handler 而不走 mount.go 那条 15s 硬编码路径。

### Step 2.4 commit

```
git commit -m "feat(webui): SSE /events 端点——request/reload 事件 + 心跳 + 客户端断开自动 unsub"
```

---

## Task 3 — `internal/cli/serve_v04_e2e_test.go` 扩展 SSE 集成测试

可选：起 httptest server → 用 `http.Get` 读流前 N 字节，验证看到 `event: request`。

### commit

```
git commit -m "test(cli): v0.4 SSE 集成 E2E"
```

---

## Task 4 — DoD 核对 + 文档回写 + bd close

```
go test ./... -count=1
go test -cover ./internal/{recorder,webui}
go vet ./...
```

回写：
- v0.4 overview §2 Phase 2 行 `✅ 已完成 (commit <T1>..<T4>)`
- Phase 2 详述末尾"实际落地偏差" + commit 流水
- design.md 修订记录追加 `v0.4-phase0.4.2-applied`
- design.md §13 追加 "v0.4 Phase 2 阶段确认"（**严格 3 条事实**）
- bd Phase 2 epic 关闭

---

## 自检

| 检查项 | 结果 |
|---|---|
| 每 Task 自带测试 | ✓ |
| 不污染 Phase 1 既有测试 | ✓ |
| SSE 心跳参数化（便于测试用短周期）| ✓ |
| 慢客户端非阻塞 + 计数 | ✓ |
| EmitReload 接口预留，watcher 接入留 Phase 3 | ✓ |
| design.md 回写**严格 3 条事实** | ✓ |
