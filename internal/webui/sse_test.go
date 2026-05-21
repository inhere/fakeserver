package webui

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gookit/rux/v2"

	"github.com/inhere/fakeserver/internal/recorder"
)

// newSSETestServer 建立一个 httptest server，仅挂载 SSE 端点（短心跳便于测试）。
func newSSETestServer(t *testing.T, ring *recorder.Ring, heartbeat time.Duration) *httptest.Server {
	t.Helper()
	router := rux.New()
	router.GET("/__fakeserver/events", sseEventsHandler(ring, heartbeat))
	return httptest.NewServer(router)
}

func TestSSE_SetsEventStreamContentType(t *testing.T) {
	ring := recorder.New(10)
	srv := newSSETestServer(t, ring, time.Hour) // 心跳设大，避免干扰头部断言
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/__fakeserver/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type=%q, want prefix text/event-stream", ct)
	}
	if resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("Cache-Control=%q, want no-cache", resp.Header.Get("Cache-Control"))
	}
}

func TestSSE_DeliversRequestEvent(t *testing.T) {
	ring := recorder.New(10)
	srv := newSSETestServer(t, ring, time.Hour)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/__fakeserver/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Append 一条 Entry → SSE handler 应推一个 request 事件
	go func() {
		time.Sleep(50 * time.Millisecond) // 等 client 连上
		ring.Append(recorder.Entry{Method: "GET", Path: "/p", Status: 200})
	}()

	scanner := bufio.NewScanner(resp.Body)
	var seenEvent, seenData bool
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) && scanner.Scan() {
		line := scanner.Text()
		if line == "event: request" {
			seenEvent = true
		}
		if strings.HasPrefix(line, "data:") && strings.Contains(line, `"path":"/p"`) {
			seenData = true
		}
		if seenEvent && seenData {
			break
		}
	}
	if !seenEvent || !seenData {
		t.Errorf("expected SSE frame `event: request` + data with /p; seenEvent=%v seenData=%v", seenEvent, seenData)
	}
}

func TestSSE_DeliversReloadEvent(t *testing.T) {
	ring := recorder.New(10)
	srv := newSSETestServer(t, ring, time.Hour)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/__fakeserver/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	go func() {
		time.Sleep(50 * time.Millisecond)
		ring.EmitReload(recorder.ReloadDiff{Added: []string{"GET /new"}})
	}()

	scanner := bufio.NewScanner(resp.Body)
	var seen bool
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) && scanner.Scan() {
		line := scanner.Text()
		if line == "event: reload" {
			seen = true
			break
		}
	}
	if !seen {
		t.Error("expected SSE frame `event: reload`")
	}
}

func TestSSE_HeartbeatPingEmittedPeriodically(t *testing.T) {
	ring := recorder.New(10)
	srv := newSSETestServer(t, ring, 80*time.Millisecond)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/__fakeserver/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	pings := 0
	deadline := time.Now().Add(500 * time.Millisecond) // 500ms / 80ms ≈ 6 ticks
	for time.Now().Before(deadline) && scanner.Scan() {
		if scanner.Text() == ": ping" {
			pings++
			if pings >= 2 {
				break
			}
		}
	}
	if pings < 2 {
		t.Errorf("expected at least 2 heartbeat pings within 500ms; got %d", pings)
	}
}

func TestSSE_ClientDisconnect_UnsubscribesCleanly(t *testing.T) {
	ring := recorder.New(10)
	srv := newSSETestServer(t, ring, time.Hour)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/__fakeserver/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	// 等 handler 完成 Subscribe（让 client 取到响应头）
	time.Sleep(80 * time.Millisecond)

	// 客户端断开
	cancel()
	resp.Body.Close()

	// 等 server 端 unsubscribe（handler ctx.Done 回收订阅者）
	time.Sleep(150 * time.Millisecond)

	// 此时 ring 应没有订阅者：Append N 次后 EventsDropped 不应增长（因为没人在订阅 → 没人占 channel → 没人需要丢弃）
	startDropped := ring.EventsDropped()
	for i := 0; i < 100; i++ {
		ring.Append(recorder.Entry{Path: "/x", Status: 200})
	}
	if delta := ring.EventsDropped() - startDropped; delta != 0 {
		t.Errorf("after client disconnect, expected no drops on Append; got delta=%d", delta)
	}

	// 不影响 ring 写：Snapshot 应包含 Append
	if got := ring.Snapshot(); len(got) == 0 {
		t.Error("ring should still accept Append after subscriber disconnect")
	}
}
