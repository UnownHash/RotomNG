package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/settings"
)

// mockDeviceConn implements DeviceConn for testing.
type mockDeviceConn struct {
	id     string
	origin string
	runFn  func(ctx context.Context, command string) (*RunJobResponse, error)
}

func (m *mockDeviceConn) ID() string     { return m.id }
func (m *mockDeviceConn) Origin() string { return m.origin }
func (m *mockDeviceConn) RunJob(ctx context.Context, command string) (*RunJobResponse, error) {
	if m.runFn != nil {
		return m.runFn(ctx, command)
	}
	return &RunJobResponse{CommandResult: "ok"}, nil
}

func newTestManager(t *testing.T, jobsPath string) *Manager {
	t.Helper()

	container, err := settings.NewContainer(ManagerSettings{JobsPath: jobsPath})
	if err != nil {
		t.Fatalf("failed to create settings container: %v", err)
	}

	cfg := ManagerConfig{
		managerSettingsContainer: container,
		Logger:                   logging.NewDiscardLogger(),
	}
	return NewManager(cfg)
}

func writeJobFile(t *testing.T, dir string, filename string, data any) {
	t.Helper()

	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("failed to marshal job data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), b, 0644); err != nil {
		t.Fatalf("failed to write job file: %v", err)
	}
}

func TestNewManager(t *testing.T) {
	mgr := newTestManager(t, "/tmp/nonexistent")
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	// Fresh manager should have no jobs or instances.
	if jobs := mgr.GetJobs(); len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
	if instances := mgr.GetJobInstances(); len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}
}

func TestManagerSettingsValidate(t *testing.T) {
	s := ManagerSettings{JobsPath: ""}
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty jobs path")
	}

	s = ManagerSettings{JobsPath: "/some/path"}
	if err := s.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReloadSingleJobFile(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job1.json", Job{ID: "test-job", Description: "a test", Exec: "echo hello"})

	mgr := newTestManager(t, dir)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	jobs := mgr.GetJobs()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].ID != "test-job" {
		t.Errorf("expected job id 'test-job', got '%s'", jobs[0].ID)
	}
	if jobs[0].Exec != "echo hello" {
		t.Errorf("expected exec 'echo hello', got '%s'", jobs[0].Exec)
	}
}

func TestReloadJobSliceFile(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "jobs.json", []Job{
		{ID: "a", Description: "first", Exec: "cmd1"},
		{ID: "b", Description: "second", Exec: "cmd2"},
	})

	mgr := newTestManager(t, dir)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	jobs := mgr.GetJobs()
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	// Jobs should be sorted by id.
	if jobs[0].ID != "a" || jobs[1].ID != "b" {
		t.Errorf("jobs not sorted: got %s, %s", jobs[0].ID, jobs[1].ID)
	}
}

func TestReloadMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "first.json", Job{ID: "x", Description: "x", Exec: "x"})
	writeJobFile(t, dir, "second.json", Job{ID: "y", Description: "y", Exec: "y"})

	mgr := newTestManager(t, dir)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	if jobs := mgr.GetJobs(); len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestReloadDuplicateJobId(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "a.json", Job{ID: "dup", Description: "first", Exec: "cmd1"})
	writeJobFile(t, dir, "b.json", Job{ID: "dup", Description: "second", Exec: "cmd2"})

	mgr := newTestManager(t, dir)
	err := mgr.Reload()
	if err == nil {
		t.Fatal("expected error for duplicate job id")
	}
}

func TestReloadIgnoresNonJsonFiles(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "valid", Description: "valid", Exec: "cmd"})
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a job"), 0644)

	mgr := newTestManager(t, dir)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	if jobs := mgr.GetJobs(); len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestReloadInvalidJson(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid json"), 0644)

	mgr := newTestManager(t, dir)
	err := mgr.Reload()
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestReloadNonexistentDirectory(t *testing.T) {
	mgr := newTestManager(t, "/tmp/does-not-exist-for-test-12345")
	err := mgr.Reload()
	if err == nil {
		t.Fatal("expected error for nonexistent directory")
	}
}

func TestReloadSubdirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	os.MkdirAll(subdir, 0755)
	writeJobFile(t, subdir, "nested.json", Job{ID: "nested", Description: "nested", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	if jobs := mgr.GetJobs(); len(jobs) != 1 {
		t.Fatalf("expected 1 job from subdirectory, got %d", len(jobs))
	}
}

func TestReloadReplacesJobs(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "v1", Description: "v1", Exec: "cmd1"})

	mgr := newTestManager(t, dir)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("first reload failed: %v", err)
	}

	if jobs := mgr.GetJobs(); len(jobs) != 1 || jobs[0].ID != "v1" {
		t.Fatalf("unexpected jobs after first reload: %+v", jobs)
	}

	// Replace file and reload.
	writeJobFile(t, dir, "job.json", Job{ID: "v2", Description: "v2", Exec: "cmd2"})
	if err := mgr.Reload(); err != nil {
		t.Fatalf("second reload failed: %v", err)
	}

	jobs := mgr.GetJobs()
	if len(jobs) != 1 || jobs[0].ID != "v2" {
		t.Fatalf("expected v2 job after reload, got: %+v", jobs)
	}
}

func TestGetJobByID(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "find-me", Description: "desc", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	job, ok := mgr.GetJobByID("find-me")
	if !ok {
		t.Fatal("expected to find job")
	}
	if job.ID != "find-me" {
		t.Errorf("unexpected job id: %s", job.ID)
	}

	_, ok = mgr.GetJobByID("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent job")
	}
}

func TestRunJobSuccess(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "run-me", Description: "desc", Exec: "echo test"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	device := &mockDeviceConn{
		id:     "dev1",
		origin: "test-origin",
		runFn: func(_ context.Context, command string) (*RunJobResponse, error) {
			if command != "echo test" {
				t.Errorf("unexpected command: %s", command)
			}
			return &RunJobResponse{CommandResult: "test output"}, nil
		},
	}

	instance := mgr.RunJob(context.Background(), "run-me", device, 5*time.Second)

	if instance.JobID != "run-me" {
		t.Errorf("expected job id 'run-me', got '%s'", instance.JobID)
	}
	if instance.DeviceID != "dev1" {
		t.Errorf("expected device id 'dev1', got '%s'", instance.DeviceID)
	}
	if instance.DeviceOrigin != "test-origin" {
		t.Errorf("expected device origin 'test-origin', got '%s'", instance.DeviceOrigin)
	}
	if instance.Status != JobInstanceStatusStarted {
		t.Errorf("expected started status, got %s", instance.Status)
	}

	// Wait for the goroutine to complete.
	mgr.Wait()

	// Fetch the updated instance.
	updated, ok := mgr.GetJobInstanceByID(instance.ID)
	if !ok {
		t.Fatal("expected to find job instance after completion")
	}
	if updated.Status != JobInstanceStatusSucceeded {
		t.Errorf("expected succeeded status, got %s", updated.Status)
	}
	if updated.Result != "test output" {
		t.Errorf("expected result 'test output', got '%s'", updated.Result)
	}
	if updated.FinishedAt.IsZero() {
		t.Error("expected non-zero finished time")
	}
}

func TestRunJobFailure(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "fail-job", Description: "desc", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	device := &mockDeviceConn{
		id:     "dev1",
		origin: "origin",
		runFn: func(_ context.Context, _ string) (*RunJobResponse, error) {
			return nil, errors.New("device error")
		},
	}

	instance := mgr.RunJob(context.Background(), "fail-job", device, 5*time.Second)
	mgr.Wait()

	updated, ok := mgr.GetJobInstanceByID(instance.ID)
	if !ok {
		t.Fatal("expected to find job instance")
	}
	if updated.Status != JobInstanceStatusFailed {
		t.Errorf("expected failed status, got %s", updated.Status)
	}
	if updated.Result != "device error" {
		t.Errorf("expected 'device error' result, got '%s'", updated.Result)
	}
}

func TestRunJobNotFound(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, dir)

	device := &mockDeviceConn{id: "dev1", origin: "origin"}

	instance := mgr.RunJob(context.Background(), "nonexistent", device, 5*time.Second)

	if instance.Status != JobInstanceStatusFailed {
		t.Errorf("expected failed status for missing job, got %s", instance.Status)
	}
	if instance.Result == "" {
		t.Error("expected non-empty result for missing job")
	}
	// Job not found should not be stored in instances.
	_, ok := mgr.GetJobInstanceByID(instance.ID)
	if ok {
		t.Error("job-not-found instance should not be stored")
	}
}

func TestRunJobTimeout(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "slow-job", Description: "desc", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	device := &mockDeviceConn{
		id:     "dev1",
		origin: "origin",
		runFn: func(ctx context.Context, _ string) (*RunJobResponse, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Second):
				return &RunJobResponse{CommandResult: "should not get here"}, nil
			}
		},
	}

	instance := mgr.RunJob(context.Background(), "slow-job", device, 50*time.Millisecond)
	mgr.Wait()

	updated, _ := mgr.GetJobInstanceByID(instance.ID)
	if updated.Status != JobInstanceStatusFailed {
		t.Errorf("expected failed status for timed-out job, got %s", updated.Status)
	}
}

func TestRunJobIncrementingIds(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "job", Description: "desc", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	device := &mockDeviceConn{id: "dev", origin: "origin"}

	i1 := mgr.RunJob(context.Background(), "job", device, 5*time.Second)
	i2 := mgr.RunJob(context.Background(), "job", device, 5*time.Second)
	i3 := mgr.RunJob(context.Background(), "job", device, 5*time.Second)

	if i1.ID >= i2.ID || i2.ID >= i3.ID {
		t.Errorf("expected incrementing ids, got %d, %d, %d", i1.ID, i2.ID, i3.ID)
	}

	mgr.Wait()
}

func TestAddFailedJobInstance(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, dir)

	instance := mgr.AddFailedJobInstance("job-1", "device-1", "something broke")

	if instance.ID == 0 {
		t.Error("expected non-zero id")
	}
	if instance.JobID != "job-1" {
		t.Errorf("expected job id 'job-1', got '%s'", instance.JobID)
	}
	if instance.DeviceID != "device-1" {
		t.Errorf("expected device id 'device-1', got '%s'", instance.DeviceID)
	}
	if instance.Status != JobInstanceStatusFailed {
		t.Errorf("expected failed status, got %s", instance.Status)
	}
	if instance.Result != "something broke" {
		t.Errorf("expected result 'something broke', got '%s'", instance.Result)
	}
	if instance.StartedAt.IsZero() || instance.FinishedAt.IsZero() {
		t.Error("expected non-zero timestamps")
	}

	// Should be retrievable.
	fetched, ok := mgr.GetJobInstanceByID(instance.ID)
	if !ok {
		t.Fatal("expected to find instance")
	}
	if fetched.Result != "something broke" {
		t.Errorf("fetched result mismatch: %s", fetched.Result)
	}
}

func TestGetJobInstances(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, dir)

	mgr.AddFailedJobInstance("j1", "d1", "err1")
	mgr.AddFailedJobInstance("j2", "d2", "err2")
	mgr.AddFailedJobInstance("j3", "d3", "err3")

	instances := mgr.GetJobInstances()
	if len(instances) != 3 {
		t.Fatalf("expected 3 instances, got %d", len(instances))
	}

	// Should be sorted newest first (descending id).
	for i := 1; i < len(instances); i++ {
		if instances[i].ID >= instances[i-1].ID {
			t.Errorf("instances not sorted newest-first: id %d >= %d at position %d", instances[i].ID, instances[i-1].ID, i)
		}
	}
}

func TestClearJobInstance(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, dir)

	instance := mgr.AddFailedJobInstance("j1", "d1", "err")

	ok := mgr.ClearJobInstance(instance.ID)
	if !ok {
		t.Error("expected ClearJobInstance to return true")
	}

	_, found := mgr.GetJobInstanceByID(instance.ID)
	if found {
		t.Error("instance should have been cleared")
	}

	// Clearing again should return false.
	ok = mgr.ClearJobInstance(instance.ID)
	if ok {
		t.Error("expected ClearJobInstance to return false for already-cleared instance")
	}
}

func TestClearJobInstances(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "job", Description: "desc", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	// Add some finished instances.
	mgr.AddFailedJobInstance("j1", "d1", "err1")
	mgr.AddFailedJobInstance("j2", "d2", "err2")

	// Start a running instance.
	blockCh := make(chan struct{})
	device := &mockDeviceConn{
		id:     "dev",
		origin: "origin",
		runFn: func(_ context.Context, _ string) (*RunJobResponse, error) {
			<-blockCh
			return &RunJobResponse{CommandResult: "done"}, nil
		},
	}
	runningInstance := mgr.RunJob(context.Background(), "job", device, 5*time.Second)

	// Clear all — should keep only the running instance.
	mgr.ClearJobInstances()

	instances := mgr.GetJobInstances()
	if len(instances) != 1 {
		t.Fatalf("expected 1 running instance after clear, got %d", len(instances))
	}
	if instances[0].ID != runningInstance.ID {
		t.Errorf("expected running instance to survive clear")
	}

	// Unblock and clean up.
	close(blockCh)
	mgr.Wait()
}

func TestClearJobInstancesEmpty(t *testing.T) {
	dir := t.TempDir()
	mgr := newTestManager(t, dir)

	// Should not panic on empty manager.
	mgr.ClearJobInstances()

	if instances := mgr.GetJobInstances(); len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}
}

func TestJobInstanceStatusString(t *testing.T) {
	tests := []struct {
		status   JobInstanceStatus
		expected string
	}{
		{JobInstanceStatusStarted, "started"},
		{JobInstanceStatusSucceeded, "succeeded"},
		{JobInstanceStatusFailed, "failed"},
		{JobInstanceStatus(99), "invalid"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("status %d: expected '%s', got '%s'", tt.status, tt.expected, got)
		}
	}
}

func TestIsErrJobNotFound(t *testing.T) {
	if !IsErrJobNotFound(errJobNotFound) {
		t.Error("expected true for errJobNotFound")
	}
	if !IsErrJobNotFound(fmt.Errorf("wrapped: %w", errJobNotFound)) {
		t.Error("expected true for wrapped errJobNotFound")
	}
	if IsErrJobNotFound(errors.New("some other error")) {
		t.Error("expected false for unrelated error")
	}
	if IsErrJobNotFound(nil) {
		t.Error("expected false for nil")
	}
}

func TestConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "concurrent", Description: "desc", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			mgr.GetJobs()
			mgr.GetJobByID("concurrent")
			mgr.GetJobInstances()
			mgr.AddFailedJobInstance("concurrent", "dev", "err")
		})
	}
	wg.Wait()

	instances := mgr.GetJobInstances()
	if len(instances) != 20 {
		t.Errorf("expected 20 instances from concurrent adds, got %d", len(instances))
	}
}

func TestGetJobsSorted(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "jobs.json", []Job{
		{ID: "zebra", Description: "z", Exec: "z"},
		{ID: "apple", Description: "a", Exec: "a"},
		{ID: "mango", Description: "m", Exec: "m"},
	})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	jobs := mgr.GetJobs()
	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}
	if jobs[0].ID != "apple" || jobs[1].ID != "mango" || jobs[2].ID != "zebra" {
		t.Errorf("jobs not sorted: %s, %s, %s", jobs[0].ID, jobs[1].ID, jobs[2].ID)
	}
}

func TestReloadEmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	mgr := newTestManager(t, dir)
	if err := mgr.Reload(); err != nil {
		t.Fatalf("reload of empty dir failed: %v", err)
	}

	if jobs := mgr.GetJobs(); len(jobs) != 0 {
		t.Errorf("expected 0 jobs from empty dir, got %d", len(jobs))
	}
}

func TestWait(t *testing.T) {
	dir := t.TempDir()
	writeJobFile(t, dir, "job.json", Job{ID: "wait-job", Description: "desc", Exec: "cmd"})

	mgr := newTestManager(t, dir)
	mgr.Reload()

	completed := make(chan struct{})
	device := &mockDeviceConn{
		id:     "dev",
		origin: "origin",
		runFn: func(_ context.Context, _ string) (*RunJobResponse, error) {
			time.Sleep(50 * time.Millisecond)
			close(completed)
			return &RunJobResponse{CommandResult: "done"}, nil
		},
	}

	mgr.RunJob(context.Background(), "wait-job", device, 5*time.Second)
	mgr.Wait()

	select {
	case <-completed:
		// Good — job finished before Wait returned.
	default:
		t.Error("Wait() returned before job completed")
	}
}
