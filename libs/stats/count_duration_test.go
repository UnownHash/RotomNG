package stats

import (
	"testing"
	"time"
)

// --- CountDurationCollector tests ---

func TestNewCountDurationCollector_Basic(t *testing.T) {
	c := NewCountDurationCollector[uint64](15*time.Minute, 2*time.Second)
	if c == nil {
		t.Fatal("expected non-nil collector")
	}
	expected := 1 + int((15*time.Minute)/(2*time.Second))
	if len(c.countBuckets) != expected {
		t.Errorf("countBuckets = %d, want %d", len(c.countBuckets), expected)
	}
	if len(c.durationBuckets) != expected {
		t.Errorf("durationBuckets = %d, want %d", len(c.durationBuckets), expected)
	}
}

func TestNewCountDurationCollector_InvalidArgs(t *testing.T) {
	if c := NewCountDurationCollector[int](10*time.Second, 0); c != nil {
		t.Error("expected nil for zero bucket duration")
	}
	if c := NewCountDurationCollector[int](10*time.Second, 3*time.Second); c != nil {
		t.Error("expected nil for non-divisible durations")
	}
}

func TestCountDurationCollector_AddAndGetWindows(t *testing.T) {
	c := NewCountDurationCollector[int](10*time.Second, 1*time.Second)

	c.Add(1, 100)
	c.Add(1, 200)
	c.Add(1, 50)

	s := c.GetWindows(5*time.Second, 10*time.Second)

	for i := range s.Counts {
		if s.Counts[i].Count() != 3 {
			t.Errorf("Counts window[%d] count = %d, want 3", i, s.Counts[i].Count())
		}
		if s.Counts[i].Sum() != 3 {
			t.Errorf("Counts window[%d] sum = %d, want 3", i, s.Counts[i].Sum())
		}
		if s.Durations[i].Count() != 3 {
			t.Errorf("Durations window[%d] count = %d, want 3", i, s.Durations[i].Count())
		}
		if s.Durations[i].Sum() != 350 {
			t.Errorf("Durations window[%d] sum = %d, want 350", i, s.Durations[i].Sum())
		}
	}
}

func TestCountDurationCollector_Empty(t *testing.T) {
	c := NewCountDurationCollector[int](10*time.Second, 1*time.Second)

	s := c.GetWindows(5 * time.Second)
	if s.Counts[0].Count() != 0 {
		t.Errorf("Counts count = %d, want 0", s.Counts[0].Count())
	}
	if s.Durations[0].Count() != 0 {
		t.Errorf("Durations count = %d, want 0", s.Durations[0].Count())
	}
}

func TestCountDurationCollector_AdvanceClears(t *testing.T) {
	c := NewCountDurationCollector[int](4*time.Second, 1*time.Second)

	c.Add(1, 100)

	// Simulate time advancing past the entire window
	c.mu.Lock()
	c.headTime = c.headTime.Add(-5 * time.Second)
	c.mu.Unlock()

	s := c.GetWindows(4 * time.Second)
	if s.Counts[0].Count() != 0 {
		t.Errorf("Counts count after full advance = %d, want 0", s.Counts[0].Count())
	}
	if s.Durations[0].Count() != 0 {
		t.Errorf("Durations count after full advance = %d, want 0", s.Durations[0].Count())
	}
}

func TestCountDurationCollector_PartialAdvance(t *testing.T) {
	c := NewCountDurationCollector[int](4*time.Second, 1*time.Second)

	c.Add(1, 100)

	// Simulate 2 buckets of time passing
	c.mu.Lock()
	c.headTime = c.headTime.Add(-2 * time.Second)
	c.mu.Unlock()

	c.Add(1, 200)

	s := c.GetWindows(4 * time.Second)
	if s.Counts[0].Count() != 2 {
		t.Errorf("Counts count = %d, want 2", s.Counts[0].Count())
	}
	if s.Counts[0].Sum() != 2 {
		t.Errorf("Counts sum = %d, want 2", s.Counts[0].Sum())
	}
	if s.Durations[0].Sum() != 300 {
		t.Errorf("Durations sum = %d, want 300", s.Durations[0].Sum())
	}
}

func TestTimeWindowedStatsPeriod_Avg(t *testing.T) {
	c := NewCountDurationCollector[int](10*time.Second, 1*time.Second)

	c.Add(1, 100)
	c.Add(1, 200)

	s := c.GetWindows(10 * time.Second)

	avg := s.Durations[0].Avg()
	if avg != 150.0 {
		t.Errorf("Avg = %f, want 150.0", avg)
	}

	// Empty period should return 0
	empty := NewCountDurationCollector[int](10*time.Second, 1*time.Second)
	es := empty.GetWindows(10 * time.Second)
	if es.Durations[0].Avg() != 0 {
		t.Errorf("empty Avg = %f, want 0", es.Durations[0].Avg())
	}
}

func TestTimeWindowedStatsPeriod_RatePerSecond(t *testing.T) {
	c := NewCountDurationCollector[int](10*time.Second, 1*time.Second)

	c.Add(5, 0)
	c.Add(5, 0)

	s := c.GetWindows(10 * time.Second)

	rate := s.Counts[0].RatePerSecond()
	if rate <= 0 {
		t.Errorf("RatePerSecond = %f, want > 0", rate)
	}

	// Empty period should return 0
	empty := NewCountDurationCollector[int](10*time.Second, 1*time.Second)
	es := empty.GetWindows(10 * time.Second)
	if es.Counts[0].RatePerSecond() != 0 {
		t.Errorf("empty RatePerSecond = %f, want 0", es.Counts[0].RatePerSecond())
	}
}

func TestCountDurationCollector_ConsistentTimestamps(t *testing.T) {
	c := NewCountDurationCollector[int](10*time.Second, 1*time.Second)

	c.Add(1, 50)

	s := c.GetWindows(5*time.Second, 10*time.Second)

	for i := range s.Counts {
		if !s.Counts[i].StartTime().Equal(s.Durations[i].StartTime()) {
			t.Errorf("window[%d] start times differ: Counts=%v Durations=%v", i, s.Counts[i].StartTime(), s.Durations[i].StartTime())
		}
		if !s.Counts[i].EndTime().Equal(s.Durations[i].EndTime()) {
			t.Errorf("window[%d] end times differ: Counts=%v Durations=%v", i, s.Counts[i].EndTime(), s.Durations[i].EndTime())
		}
	}
}
