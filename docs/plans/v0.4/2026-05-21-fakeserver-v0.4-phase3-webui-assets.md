# Fakeserver v0.4 Phase 3 Web UI Assets Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `GET /__fakeserver/ui/` 提供内嵌静态 Web UI，覆盖项目、路由、历史、配置 4 个只读视图，并通过 SSE 实时追加请求历史与 reload 提示。

**Architecture:** `internal/webui` 继续作为唯一 UI 边界：新增 `assets.go` 用 `embed.FS` 暴露 `assets/*`，`mount.go` 注册 `/__fakeserver/ui/*`，静态 HTML/CSS/JS 不引入构建链或外部 CDN。`internal/cli/serve.go` 在 watcher reload 成功后计算轻量路由 diff 并调用 `ring.EmitReload`，让 Phase 2 已实现的 SSE 通道端到端可见。

**Tech Stack:** Go 标准库 `embed` / `io/fs` / `net/http` / `encoding/json`，现有 `rux/v2`，手写 HTML/CSS/JS。

---

## File Structure

| 操作 | 路径 | 职责 |
|---|---|---|
| 新建 | `internal/webui/assets.go` | `//go:embed assets/*`，提供 `/__fakeserver/ui/` 静态资源 handler |
| 新建 | `internal/webui/assets/index.html` | UI 框架页：侧栏、顶部状态、4 个只读 view 容器 |
| 新建 | `internal/webui/assets/style.css` | 工具型工作台视觉：紧凑表格、侧栏、状态 chip、响应式布局 |
| 新建 | `internal/webui/assets/main.js` | fetch JSON API、hash 路由、EventSource 监听、表格渲染 |
| 修改 | `internal/webui/mount.go` | 注册 `/__fakeserver/ui/` 与 `/__fakeserver/ui/*path` |
| 新建/修改 | `internal/webui/assets_test.go` | 静态资源 handler 单测：HTML/CSS/JS、admin disabled 404 |
| 修改 | `internal/cli/serve.go` | watcher reload 成功后计算 route diff 并 `ring.EmitReload` |
| 修改 | `internal/cli/serve_v04_e2e_test.go` | Web UI 综合 E2E + reload 事件集成测试 |
| 修改 | `docs/plans/2026-05-20-fakeserver-v0.4-overview.md` | Phase 3 完成后回写状态、偏差、commit 流水 |
| 修改 | `docs/fakeserver-design.md` | Phase 3 完成后追加修订记录与 §13 严格 3 条事实 |

---

## Task 1: Static Asset Handler

**Files:**
- Create: `internal/webui/assets.go`
- Create: `internal/webui/assets/index.html`
- Modify: `internal/webui/mount.go`
- Test: `internal/webui/assets_test.go`

- [x] **Step 1: Write failing tests**

Add tests:
- `GET /__fakeserver/ui/` returns `200`, `text/html`, and contains `<title>fakeserver</title>`
- `GET /__fakeserver/ui/index.html` returns the same app shell
- `GET /__fakeserver/ui/missing.css` returns `404`
- `adminEnabled=false` keeps `/__fakeserver/ui/` unreachable

Run:

```bash
go test ./internal/webui -run 'TestUIAssets|TestMount_AdminDisabled' -count=1
```

Expected: fail because UI route does not exist.

- [x] **Step 2: Implement minimal embed handler**

Create `assets.go` with:
- `//go:embed assets/*`
- `fs.Sub(embeddedAssets, "assets")`
- `http.FileServer(http.FS(sub))`
- wrapper that maps `/__fakeserver/ui/` to `index.html`

Update `mount.go` to register:
- `GET /__fakeserver/ui/`
- `GET /__fakeserver/ui/*path`

- [x] **Step 3: Run tests and commit**

```bash
go test ./internal/webui -run 'TestUIAssets|TestMount_AdminDisabled' -count=1
git add internal/webui/assets.go internal/webui/assets/index.html internal/webui/assets_test.go internal/webui/mount.go docs/plans/v0.4/2026-05-21-fakeserver-v0.4-phase3-webui-assets.md
git commit -m "feat(webui): embed UI asset handler"
```

---

## Task 2: Four Read-Only UI Views

**Files:**
- Modify: `internal/webui/assets/index.html`
- Create: `internal/webui/assets/style.css`
- Create: `internal/webui/assets/main.js`
- Modify: `internal/webui/assets_test.go`

- [x] **Step 1: Write asset contract tests**

Add assertions:
- HTML includes links for `#projects`, `#routes`, `#history`, `#config`
- HTML references `style.css` and `main.js`
- CSS response is `text/css` and contains `.sidebar`
- JS response is JavaScript and contains `EventSource`

Run:

```bash
go test ./internal/webui -run TestUIAssets -count=1
```

Expected: fail until CSS/JS exist and HTML links are present.

- [x] **Step 2: Implement UI assets**

UI behavior:
- Projects view: fetch `/__fakeserver/api/projects`
- Routes view: fetch `/__fakeserver/routes`
- History view: fetch `/__fakeserver/api/history`, then append `event: request` rows from `/__fakeserver/events`
- Config view: fetch `/__fakeserver/api/config`, render redacted JSON as read-only `<pre>`
- Reload event: show a compact banner with added/removed/changed counts

Design constraints:
- no external assets, no CDN, no build step
- dense operational UI, no marketing hero
- stable table dimensions and responsive single-column layout on small screens

- [x] **Step 3: Run tests and commit**

```bash
go test ./internal/webui -run TestUIAssets -count=1
git add internal/webui/assets/index.html internal/webui/assets/style.css internal/webui/assets/main.js internal/webui/assets_test.go docs/plans/v0.4/2026-05-21-fakeserver-v0.4-phase3-webui-assets.md
git commit -m "feat(webui): add read-only dashboard assets"
```

---

## Task 3: Watcher Reload SSE Integration

**Files:**
- Modify: `internal/cli/serve.go`
- Modify: `internal/cli/serve_v04_e2e_test.go`

- [x] **Step 1: Write failing reload event test**

Add an integration test around a helper function, not a real filesystem watcher:
- build old cfg with `GET /old`
- build new cfg with `GET /new`
- call route diff helper
- expect `Added=["GET /new"]`, `Removed=["GET /old"]`, `Changed=[]`

Add SSE integration:
- subscribe to `/__fakeserver/events`
- trigger the same reload path used by watcher helper, or call the helper that emits after swap
- expect `event: reload`

Run:

```bash
go test ./internal/cli -run 'TestServe_v04_Reload|TestRouteReloadDiff' -count=1
```

Expected: fail because diff helper / emit wiring does not exist.

- [x] **Step 2: Implement route diff helper and watcher emit**

Add unexported helpers in `serve.go`:
- `routeSignature(config.Route) string`
- `routeDetail(config.Route) string`
- `diffRoutes(oldCfg, newCfg *config.Config) recorder.ReloadDiff`

In watcher callback:
- keep `currentCfg := cfg` near holder setup
- after successful `holder.Swap(...)`, call `ring.EmitReload(diffRoutes(currentCfg, newCfg))`
- set `currentCfg = newCfg`

Changed detection can compare mode + method/path + proxy target + cases count + status/body presence. Keep it deterministic and cheap; exact semantic deep diff is not required for v0.4 UI.

- [x] **Step 3: Run tests and commit**

```bash
go test ./internal/cli -run 'TestServe_v04_Reload|TestRouteReloadDiff' -count=1
git add internal/cli/serve.go internal/cli/serve_v04_e2e_test.go docs/plans/v0.4/2026-05-21-fakeserver-v0.4-phase3-webui-assets.md
git commit -m "feat(cli): emit reload SSE events after watcher swap"
```

---

## Task 4: Comprehensive v0.4 E2E

**Files:**
- Modify: `internal/cli/serve_v04_e2e_test.go`

- [x] **Step 1: Write E2E assertions**

Extend existing v0.4 E2E:
- `GET /__fakeserver/ui/` contains `<title>fakeserver</title>` and 4 view links
- `GET /__fakeserver/ui/style.css` contains `.sidebar`
- hit one mock route, then `GET /__fakeserver/api/history` includes the path
- `adminEnabled:false` makes `/__fakeserver/ui/` return `404`

Run:

```bash
go test ./internal/cli -run TestServe_v04 -count=1
```

Expected: fail before Task 1/2, pass after current implementation.

- [x] **Step 2: Fix only integration gaps**

Do not add new UI features here. This task only wires E2E expectations against already implemented behavior.

- [x] **Step 3: Run tests and commit**

```bash
go test ./internal/cli -run TestServe_v04 -count=1
git add internal/cli/serve_v04_e2e_test.go docs/plans/v0.4/2026-05-21-fakeserver-v0.4-phase3-webui-assets.md
git commit -m "test(cli): cover v0.4 web UI integration"
```

---

## Task 5: Quality Gates and Documentation Closure

**Files:**
- Modify: `docs/plans/2026-05-20-fakeserver-v0.4-overview.md`
- Modify: `docs/fakeserver-design.md`
- Modify: `docs/plans/v0.4/2026-05-21-fakeserver-v0.4-phase3-webui-assets.md`

- [ ] **Step 1: Run quality gates**

```bash
go test ./... -count=1
go test -cover ./internal/recorder ./internal/webui
go vet ./...
go build ./...
```

Expected:
- all commands exit 0
- `internal/webui` coverage does not drop below the Phase 2 baseline materially

- [ ] **Step 2: Update docs**

Update:
- v0.4 overview §2 Phase 3 status to `✅ 已完成`
- Phase 3 详述末尾写实际落地偏差
- Phase 3 commit 流水
- `docs/fakeserver-design.md` 修订记录追加 `v0.4-phase0.4.3-applied`
- `docs/fakeserver-design.md` §13 追加 `v0.4 Phase 3 阶段确认`，严格 3 条事实

- [ ] **Step 3: Close beads issue and commit**

```bash
bd close lite-tools-xzn --reason="fakeserver v0.4 Phase 3 Web UI completed"
git add docs/plans/2026-05-20-fakeserver-v0.4-overview.md docs/fakeserver-design.md docs/plans/v0.4/2026-05-21-fakeserver-v0.4-phase3-webui-assets.md
git commit -m "docs(v0.4): close Phase 3 web UI milestone"
```

---

## DoD Checklist

- [ ] `/__fakeserver/ui/` returns embedded HTML with 4 view links
- [ ] UI assets are fully embedded under `internal/webui/assets/`
- [ ] No CDN, no external asset URL, no JS build chain
- [ ] History view loads `/api/history` and appends live `event: request`
- [ ] Reload SSE event is emitted after successful watcher swap
- [ ] `adminEnabled:false` disables UI routes
- [ ] `go test ./... -count=1`, `go vet ./...`, and `go build ./...` pass
- [ ] v0.4 overview and design §13 are updated
- [ ] `lite-tools-xzn` is closed
