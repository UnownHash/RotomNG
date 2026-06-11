package tracking

import (
	"sync"
	"testing"
	"time"
)

func TestNewRequestTracker(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	if tr == nil {
		t.Fatal("expected non-nil tracker")
	}
}

func TestAdd_And_Get(t *testing.T) {
	tr := NewRequestTracker[uint32, string]()
	now := time.Now()

	tr.Add(1, Request[string]{StartTime: now, MethodName: "GET", Data: "payload"})

	req, ok := tr.Get(1)
	if !ok {
		t.Fatal("expected request to be found")
	}
	if req.StartTime != now {
		t.Errorf("StartTime = %v, want %v", req.StartTime, now)
	}
	if req.MethodName != "GET" {
		t.Errorf("MethodName = %q, want %q", req.MethodName, "GET")
	}
	if req.Data != "payload" {
		t.Errorf("Data = %q, want %q", req.Data, "payload")
	}
}

func TestGet_Removes_Entry(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})

	_, ok := tr.Get(1)
	if !ok {
		t.Fatal("expected request to be found on first Get")
	}

	_, ok = tr.Get(1)
	if ok {
		t.Error("expected request to be gone after Get")
	}
}

func TestGet_Missing_Key(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()

	_, ok := tr.Get(42)
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestAdd_Overwrites(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "OLD"})
	tr.Add(1, Request[struct{}]{MethodName: "NEW"})

	req, ok := tr.Get(1)
	if !ok {
		t.Fatal("expected request to be found")
	}
	if req.MethodName != "NEW" {
		t.Errorf("MethodName = %q, want %q", req.MethodName, "NEW")
	}
}

func TestPeak_Does_Not_Remove(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})

	req, ok := tr.Peak(1)
	if !ok {
		t.Fatal("expected request to be found via Peak")
	}
	if req.MethodName != "M1" {
		t.Errorf("MethodName = %q, want %q", req.MethodName, "M1")
	}

	// Should still be present.
	_, ok = tr.Peak(1)
	if !ok {
		t.Error("expected request to remain after Peak")
	}

	// Get should also find it.
	_, ok = tr.Get(1)
	if !ok {
		t.Error("expected Get to find request after Peak")
	}
}

func TestPeak_Missing_Key(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()

	_, ok := tr.Peak(99)
	if ok {
		t.Error("expected false for missing key")
	}
}

func TestDone_Calls_Fn_For_Each(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})
	tr.Add(2, Request[struct{}]{MethodName: "M2"})
	tr.Add(3, Request[struct{}]{MethodName: "M3"})

	var collected []string
	tr.Done(func(req Request[struct{}]) {
		collected = append(collected, req.MethodName)
	})

	if len(collected) != 3 {
		t.Fatalf("collected %d requests, want 3", len(collected))
	}
}

func TestDone_Nil_Fn_Discards(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})

	tr.Done(nil)

	// nil fn silently discards and terminates the tracker.
	tr.Add(2, Request[struct{}]{MethodName: "M2"})
	_, ok := tr.Get(2)
	if ok {
		t.Error("Add after Done(nil) should be a no-op")
	}
	_, ok = tr.Get(1)
	if ok {
		t.Error("Get after Done(nil) should return false")
	}
}

func TestDone_Empty_Tracker(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()

	var called bool
	tr.Done(func(_ Request[struct{}]) {
		called = true
	})

	if called {
		t.Error("fn should not be called on empty tracker")
	}

	// Even on an empty tracker, Done terminates it.
	tr.Add(1, Request[struct{}]{MethodName: "M1"})
	_, ok := tr.Get(1)
	if ok {
		t.Error("Add after Done should be a no-op")
	}
}

func TestDone_Terminates_Add(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})

	tr.Done(func(_ Request[struct{}]) {})

	// Add after Done should be silently ignored.
	tr.Add(2, Request[struct{}]{MethodName: "M2"})
	_, ok := tr.Get(2)
	if ok {
		t.Error("Add after Done should be a no-op")
	}
}

func TestDone_Terminates_Get(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})

	tr.Done(func(_ Request[struct{}]) {})

	_, ok := tr.Get(1)
	if ok {
		t.Error("Get after Done should return false")
	}
}

func TestDone_Terminates_Peak(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})

	tr.Done(func(_ Request[struct{}]) {})

	_, ok := tr.Peak(1)
	if ok {
		t.Error("Peak after Done should return false")
	}
}

func TestDone_Called_Twice(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()
	tr.Add(1, Request[struct{}]{MethodName: "M1"})

	var firstCount, secondCount int
	tr.Done(func(_ Request[struct{}]) {
		firstCount++
	})
	tr.Done(func(_ Request[struct{}]) {
		secondCount++
	})

	if firstCount != 1 {
		t.Errorf("first Done: collected %d, want 1", firstCount)
	}
	if secondCount != 0 {
		t.Errorf("second Done: collected %d, want 0", secondCount)
	}
}

func TestString_Index_Type(t *testing.T) {
	tr := NewRequestTracker[string, int]()
	tr.Add("req-a", Request[int]{MethodName: "POST", Data: 42})

	req, ok := tr.Get("req-a")
	if !ok {
		t.Fatal("expected request to be found")
	}
	if req.Data != 42 {
		t.Errorf("Data = %d, want 42", req.Data)
	}
}

func TestConcurrent_Access(t *testing.T) {
	tr := NewRequestTracker[uint32, struct{}]()

	var wg sync.WaitGroup
	const n = 100

	// Concurrent adds.
	for i := range uint32(n) {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			tr.Add(id, Request[struct{}]{MethodName: "M"})
		}(i)
	}
	wg.Wait()

	// Concurrent gets — each key should be retrievable exactly once.
	found := make([]bool, n)
	var mu sync.Mutex

	for i := range uint32(n) {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			if _, ok := tr.Get(id); ok {
				mu.Lock()
				found[id] = true
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()

	for i, f := range found {
		if !f {
			t.Errorf("request %d was not retrieved", i)
		}
	}

	// Tracker should now be empty.
	for i := range uint32(n) {
		if _, ok := tr.Get(i); ok {
			t.Errorf("request %d should have been removed", i)
		}
	}
}
