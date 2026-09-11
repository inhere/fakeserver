package recorder

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubscribe_ReceivesAppendedEntries(t *testing.T) {
	r := New(10)
	events, cancel := r.Subscribe()
	defer cancel()

	r.Append(Entry{Path: "/p", Status: 200})

	select {
	case ev := <-events:
		if ev.Kind != EventRequest {
			t.Errorf("Kind=%q, want %q", ev.Kind, EventRequest)
		}
		if ev.Entry == nil || ev.Entry.Path != "/p" {
			t.Errorf("Entry mismatch: %+v", ev.Entry)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("subscriber should receive Append within 200ms")
	}
}

func TestSubscribe_MultipleSubscribersIndependent(t *testing.T) {
	r := New(10)
	ev1, c1 := r.Subscribe()
	defer c1()
	ev2, c2 := r.Subscribe()
	defer c2()

	r.Append(Entry{Path: "/x"})

	for i, ch := range []<-chan Event{ev1, ev2} {
		select {
		case ev := <-ch:
			if ev.Entry == nil || ev.Entry.Path != "/x" {
				t.Errorf("subscriber %d entry mismatch: %+v", i, ev.Entry)
			}
		case <-time.After(200 * time.Millisecond):
			t.Errorf("subscriber %d did not receive event", i)
		}
	}
}

func TestUnsubscribe_StopsDelivery(t *testing.T) {
	r := New(10)
	events, cancel := r.Subscribe()
	cancel()
	// 再次取消应是 no-op，不 panic
	cancel()

	r.Append(Entry{Path: "/p"})

	select {
	case ev, ok := <-events:
		if ok {
			t.Errorf("unsubscribed channel should not receive event; got %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		// 也可以接受不收到 + 不 close（实现细节）
	}
}

func TestCloseSubscribers_ClosesActiveSubscriptions(t *testing.T) {
	r := New(10)
	events, cancel := r.Subscribe()
	defer cancel()

	r.CloseSubscribers()
	r.Append(Entry{Path: "/after-close", Status: 200})

	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("subscriber channel should be closed")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("subscriber channel should close within 200ms")
	}
	if got := r.Snapshot(); len(got) != 1 || got[0].Path != "/after-close" {
		t.Fatalf("ring should keep accepting entries after CloseSubscribers: %+v", got)
	}
}

func TestSubscribe_SlowSubscriberDoesNotBlockOthers(t *testing.T) {
	r := New(10)

	// 慢订阅者：不读 channel
	_, cancelSlow := r.Subscribe()
	defer cancelSlow()

	// 快订阅者
	fast, cancelFast := r.Subscribe()
	defer cancelFast()

	// 发送多于慢订阅者 channel 容量（32）+ 1 → 触发丢弃
	const N = 50
	for i := 0; i < N; i++ {
		r.Append(Entry{Path: "/p", Status: i})
	}

	// 快订阅者应能从 channel 读取至少前 32 条（channel 容量）
	received := 0
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case <-fast:
			received++
		default:
			// channel 空了，跳出
			if received >= 1 {
				goto done
			}
		}
	}
done:
	if received == 0 {
		t.Error("fast subscriber should receive at least 1 event despite slow neighbor")
	}
	// dropped 计数应 > 0（慢订阅者满了）
	if r.EventsDropped() == 0 {
		t.Errorf("expected EventsDropped > 0 due to slow subscriber; got 0")
	}
}

func TestEmitReload_BroadcastsToSubscribers(t *testing.T) {
	r := New(10)
	events, cancel := r.Subscribe()
	defer cancel()

	r.EmitReload(ReloadDiff{Added: []string{"GET /a"}, Removed: nil, Changed: []string{"GET /b"}})

	select {
	case ev := <-events:
		if ev.Kind != EventReload {
			t.Errorf("Kind=%q, want %q", ev.Kind, EventReload)
		}
		if ev.Reload == nil || len(ev.Reload.Added) != 1 || ev.Reload.Added[0] != "GET /a" {
			t.Errorf("Reload mismatch: %+v", ev.Reload)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("subscriber should receive reload within 200ms")
	}
}

func TestAppend_NoSubscribers_StillWritesRing(t *testing.T) {
	r := New(5)
	for i := 0; i < 3; i++ {
		r.Append(Entry{Path: "/p", Status: i})
	}
	if len(r.Snapshot()) != 3 {
		t.Errorf("Phase 1 行为应保留：无订阅者时仍正常写 ring；got %d", len(r.Snapshot()))
	}
}

func TestEventsDropped_ConcurrentSafe(t *testing.T) {
	r := New(10)
	// 加一个慢订阅者持续触发 drop
	_, cancel := r.Subscribe()
	defer cancel()

	var wg = make(chan struct{}, 4)
	for g := 0; g < 4; g++ {
		go func() {
			for i := 0; i < 50; i++ {
				r.Append(Entry{Path: "/p", Status: i})
			}
			wg <- struct{}{}
		}()
	}
	for i := 0; i < 4; i++ {
		<-wg
	}
	// EventsDropped 必然 > 0（200 个 Append vs 32 容量）；用 atomic 读
	if atomic.LoadUint64(&r.eventsDropped) == 0 {
		t.Error("expected EventsDropped > 0 under concurrent Append + slow subscriber")
	}
}
func TestSubscribe_ConcurrentCancelAppendClose(t *testing.T) {
	r := New(8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, cancel := r.Subscribe()
				r.Append(Entry{Method: "GET", Path: "/x"})
				cancel()
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				r.Append(Entry{Method: "POST", Path: "/y"})
				r.CloseSubscribers()
			}
		}()
	}
	wg.Wait()
}
