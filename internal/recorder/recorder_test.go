package recorder

import (
	"sync"
	"testing"
	"time"
)

func TestNew_DefaultSizeWhenZeroOrNegative(t *testing.T) {
	r := New(0)
	if r.Cap() != 200 {
		t.Errorf("Cap=%d, want 200 (zero → default)", r.Cap())
	}
	r = New(-5)
	if r.Cap() != 200 {
		t.Errorf("Cap=%d, want 200 (negative → default)", r.Cap())
	}
	r = New(10)
	if r.Cap() != 10 {
		t.Errorf("Cap=%d, want 10", r.Cap())
	}
}

func TestAppend_Snapshot_ChronologicalOrder(t *testing.T) {
	r := New(5)
	for i := 1; i <= 3; i++ {
		r.Append(Entry{Path: "/p", Status: i})
	}
	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	for i, e := range got {
		if e.Status != i+1 {
			t.Errorf("snapshot[%d].Status=%d, want %d", i, e.Status, i+1)
		}
	}
}

func TestAppend_OverwritesOldestWhenFull(t *testing.T) {
	r := New(3)
	for i := 1; i <= 5; i++ {
		r.Append(Entry{Path: "/p", Status: i})
	}
	got := r.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	// 5 entries appended into capacity 3 → oldest 2 dropped, keep 3,4,5
	wantStatus := []int{3, 4, 5}
	for i, e := range got {
		if e.Status != wantStatus[i] {
			t.Errorf("snapshot[%d].Status=%d, want %d", i, e.Status, wantStatus[i])
		}
	}
}

func TestSnapshot_ReturnsCopyNotInternalSlice(t *testing.T) {
	r := New(5)
	r.Append(Entry{Status: 200})
	got := r.Snapshot()
	got[0].Status = 999 // mutating snapshot should not affect ring
	again := r.Snapshot()
	if again[0].Status == 999 {
		t.Error("Snapshot returned shared slice; ring data corrupted by caller")
	}
}

func TestAppend_ConcurrentSafe(t *testing.T) {
	r := New(100)
	const G = 16
	const N = 1000
	var wg sync.WaitGroup
	wg.Add(G)
	for g := 0; g < G; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < N; i++ {
				r.Append(Entry{Path: "/p", Status: g, TS: time.Now()})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = r.Snapshot()
		}
	}()
	wg.Wait()
	if got := r.Snapshot(); len(got) != 100 {
		t.Errorf("after concurrent appends, Snapshot len=%d, want 100 (= cap)", len(got))
	}
}
