// Package recorder 提供请求历史的内存环形缓冲 + SSE 订阅者广播。
// design §11.4 + §11.5。
package recorder

import (
	"sync"
	"sync/atomic"
	"time"
)

// Capture is a bounded request/response snapshot for the Web UI debug console.
type Capture struct {
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Body        string            `json:"body,omitempty"`
	BodySize    int64             `json:"bodySize,omitempty"`
	Truncated   bool              `json:"truncated,omitempty"`
	Binary      bool              `json:"binary,omitempty"`
	Omitted     []string          `json:"omitted,omitempty"`
}

// Entry 是 ring 中单条记录（design §11.4）。不包含请求/响应 body。
type Entry struct {
	ID             uint64    `json:"id"`
	TS             time.Time `json:"ts"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	Status         int       `json:"status"`
	DurationMs     float64   `json:"durationMs"`
	ClientIP       string    `json:"clientIp,omitempty"`
	RouteIndex     *int      `json:"routeIndex,omitempty"`
	CaseIndex      *int      `json:"caseIndex,omitempty"`
	RouteMode      string    `json:"routeMode,omitempty"`
	RouteSource    string    `json:"routeSource,omitempty"`
	ProxyTarget    string    `json:"proxyTarget,omitempty"`
	Scenario       string    `json:"scenario,omitempty"`
	CaseName       string    `json:"caseName,omitempty"`
	OverrideSource string    `json:"overrideSource,omitempty"`
	Request        Capture   `json:"request,omitempty"`
	Response       Capture   `json:"response,omitempty"`
}

// ReloadDiff 描述一次 holder.Swap 后路由表的增删改差异（design §11.3 event: reload）。
// Phase 2 提供接口；Phase 3 watcher 端调用接入。
type ReloadDiff struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// EventKind 是 SSE 事件类型；当前两类（design §11.3 例子）。
type EventKind string

const (
	EventRequest EventKind = "request"
	EventReload  EventKind = "reload"
)

// Event 是经 Subscribe channel 投递的判别联合。
// Kind=EventRequest → Entry 字段非 nil；Kind=EventReload → Reload 字段非 nil。
type Event struct {
	Kind   EventKind   `json:"kind"`
	Entry  *Entry      `json:"entry,omitempty"`
	Reload *ReloadDiff `json:"reload,omitempty"`
}

// subscriberChanCap 是单个订阅者 channel 的容量。
// 满则丢弃该事件 + EventsDropped 自增（design §11.5 慢客户端不阻塞其他客户端）。
const subscriberChanCap = 32

// Ring 是固定容量的环形缓冲 + 多订阅者广播管理器。
type Ring struct {
	mu          sync.RWMutex
	buf         []Entry
	head        int
	size        int
	nextEntryID uint64

	subsMu        sync.Mutex
	subscribers   map[uint64]chan Event
	nextSubID     uint64
	eventsDropped uint64 // atomic
}

// New 创建一个容量为 size 的 Ring；size <= 0 → 默认 200。
func New(size int) *Ring {
	if size <= 0 {
		size = 200
	}
	return &Ring{
		buf:         make([]Entry, size),
		subscribers: make(map[uint64]chan Event),
	}
}

// Cap 返回环形缓冲的容量。
func (r *Ring) Cap() int {
	return len(r.buf)
}

// Append 写入一条新 Entry。满时覆盖最旧条目。同时向所有订阅者广播 Event{Kind:Request}。
func (r *Ring) Append(e Entry) {
	r.mu.Lock()
	if e.ID == 0 {
		r.nextEntryID++
		e.ID = r.nextEntryID
	}
	r.buf[r.head] = e
	r.head = (r.head + 1) % len(r.buf)
	if r.size < len(r.buf) {
		r.size++
	}
	r.mu.Unlock()

	r.broadcast(Event{Kind: EventRequest, Entry: &e})
}

// Get returns the entry with id if it is still retained in the ring.
func (r *Ring) Get(id uint64) (Entry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for i := 0; i < r.size; i++ {
		idx := i
		if r.size == len(r.buf) {
			idx = (r.head + i) % len(r.buf)
		}
		if r.buf[idx].ID == id {
			return r.buf[idx], true
		}
	}
	return Entry{}, false
}

// EmitReload 向所有订阅者广播 reload 事件。Phase 3 watcher onReload 调用。
func (r *Ring) EmitReload(diff ReloadDiff) {
	r.broadcast(Event{Kind: EventReload, Reload: &diff})
}

// Snapshot 返回按时间顺序的 Entry 副本（最旧在前，最新在后）。
func (r *Ring) Snapshot() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, r.size)
	if r.size < len(r.buf) {
		copy(out, r.buf[:r.size])
		return out
	}
	start := r.head
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}

// Subscribe 注册一个新订阅者，返回事件 channel + cancel 函数。
// channel 容量 subscriberChanCap=32；满时事件被丢弃 + EventsDropped 自增。
// cancel 是幂等的；多次调用安全。
func (r *Ring) Subscribe() (<-chan Event, func()) {
	r.subsMu.Lock()
	id := r.nextSubID
	r.nextSubID++
	ch := make(chan Event, subscriberChanCap)
	r.subscribers[id] = ch
	r.subsMu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			r.subsMu.Lock()
			if existing, ok := r.subscribers[id]; ok {
				delete(r.subscribers, id)
				close(existing)
			}
			r.subsMu.Unlock()
		})
	}
	return ch, cancel
}

// CloseSubscribers closes every active subscription channel.
// This is used during HTTP graceful shutdown so long-lived SSE handlers can
// return promptly instead of waiting for their client connection to close.
func (r *Ring) CloseSubscribers() {
	r.subsMu.Lock()
	defer r.subsMu.Unlock()
	for id, ch := range r.subscribers {
		delete(r.subscribers, id)
		close(ch)
	}
}

// EventsDropped 返回因订阅者 channel 满而被丢弃的事件总数（design §11.5 "events.dropped"）。
func (r *Ring) EventsDropped() uint64 {
	return atomic.LoadUint64(&r.eventsDropped)
}

// broadcast 向所有订阅者非阻塞发送事件。
// 慢订阅者（channel 满）→ 丢弃此事件 + EventsDropped 自增；不影响其他订阅者。
func (r *Ring) broadcast(ev Event) {
	r.subsMu.Lock()
	// 复制订阅者引用到本地切片，避免持锁期间 send 阻塞影响其他 Subscribe/Unsubscribe
	chans := make([]chan Event, 0, len(r.subscribers))
	for _, ch := range r.subscribers {
		chans = append(chans, ch)
	}
	r.subsMu.Unlock()

	for _, ch := range chans {
		select {
		case ch <- ev:
		default:
			atomic.AddUint64(&r.eventsDropped, 1)
		}
	}
}
