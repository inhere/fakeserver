// Package recorder 提供请求历史的内存环形缓冲。design §11.4。
// Phase 1：Append + Snapshot；Subscribe/Unsubscribe 留 Phase 2 SSE 使用。
package recorder

import (
	"sync"
	"time"
)

// Entry 是 ring 中单条记录（design §11.4）。
// 不包含请求/响应 body（隐私 + 体积）。
type Entry struct {
	TS          time.Time `json:"ts"`
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	Status      int       `json:"status"`
	DurationMs  float64   `json:"durationMs"`
	ClientIP    string    `json:"clientIp,omitempty"`
	RouteIndex  int       `json:"routeIndex,omitempty"`  // Phase 1 占位 0；待 mock/proxy 包协作填充
	CaseIndex   int       `json:"caseIndex,omitempty"`   // 同上
	ProxyTarget string    `json:"proxyTarget,omitempty"` // 同上
}

// Ring 是固定容量的环形缓冲。
type Ring struct {
	mu   sync.RWMutex
	buf  []Entry
	head int // 下一个写入位置；缓冲已满时 buf[head] 是最旧的
	size int // 当前有效条数（≤ cap）
}

// New 创建一个容量为 size 的 Ring；size <= 0 → 默认 200（design §11.4 默认值）。
func New(size int) *Ring {
	if size <= 0 {
		size = 200
	}
	return &Ring{buf: make([]Entry, size)}
}

// Cap 返回环形缓冲的容量。
func (r *Ring) Cap() int {
	return len(r.buf)
}

// Append 写入一条新 Entry。满时覆盖最旧条目。
func (r *Ring) Append(e Entry) {
	r.mu.Lock()
	r.buf[r.head] = e
	r.head = (r.head + 1) % len(r.buf)
	if r.size < len(r.buf) {
		r.size++
	}
	r.mu.Unlock()
}

// Snapshot 返回按时间顺序的 Entry 副本（最旧在前，最新在后）。
// 调用方可自由修改返回的 slice 而不影响 ring 内部状态。
func (r *Ring) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, r.size)
	if r.size < len(r.buf) {
		copy(out, r.buf[:r.size])
		return out
	}
	// 已满：buf[head] 是最旧；后续按 buf[head+1] ... 绕回
	start := r.head
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}
