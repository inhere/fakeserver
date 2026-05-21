package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/recorder"
)

// defaultHeartbeat 是 mount.go 注册 /events 时使用的心跳间隔。
// design §11.5："心跳：每 15s 发一个 `: ping` 注释行（保持连接活）"。
// 单测注入更短的值（如 100ms）以避免等待。
const defaultHeartbeat = 15 * time.Second

// sseEventsHandler 返回 /__fakeserver/events 的 handler。
// 行为（design §11.3 + §11.5）：
//   - 响应头：text/event-stream + no-cache + keep-alive
//   - 每条 ring 事件 → SSE 帧（event: <kind>\ndata: <json>\n\n）
//   - 每 heartbeatEvery 间隔发一个 `: ping\n\n`
//   - 客户端断开（r.Context().Done()）→ unsubscribe + return
func sseEventsHandler(ring *recorder.Ring, heartbeatEvery time.Duration) rux.HandlerFunc {
	return func(c *rux.Context) {
		flusher, ok := c.Resp.(http.Flusher)
		if !ok {
			http.Error(c.Resp, "SSE unsupported by underlying writer", http.StatusInternalServerError)
			return
		}

		c.SetHeader("Content-Type", "text/event-stream")
		c.SetHeader("Cache-Control", "no-cache")
		c.SetHeader("Connection", "keep-alive")
		// 不显式调用 WriteHeader(200)：rux 的 responseWriter 在 handler 返回时
		// 会自动 ensure WriteHeader，重复调用触发 "superfluous WriteHeader call"
		// 警告。第一次 fmt.Fprint 会触发 implicit 200，然后 Flush 让客户端立刻收到。
		fmt.Fprint(c.Resp, ": ready\n\n")
		flusher.Flush()

		events, cancel := ring.Subscribe()
		defer cancel()

		ticker := time.NewTicker(heartbeatEvery)
		defer ticker.Stop()

		ctx := c.Req.Context()
		for {
			select {
			case ev, open := <-events:
				if !open {
					return
				}
				writeSSEFrame(c.Resp, ev)
				flusher.Flush()
			case <-ticker.C:
				if _, err := fmt.Fprint(c.Resp, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case <-ctx.Done():
				return
			}
		}
	}
}

// writeSSEFrame 把一个 Event 序列化为 SSE 帧（不调用 Flush；调用方决定何时 flush）。
// 不返回错误：若 JSON 编码失败，回退为空 data 段——SSE 客户端接收到事件本身已足够提示。
func writeSSEFrame(w http.ResponseWriter, ev recorder.Event) {
	var payload []byte
	switch ev.Kind {
	case recorder.EventRequest:
		payload, _ = json.Marshal(ev.Entry)
	case recorder.EventReload:
		payload, _ = json.Marshal(ev.Reload)
	default:
		payload = []byte("{}")
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, string(payload))
}
