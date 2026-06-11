// Package jobs manages job definitions and execution on devices.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"log/slog"

	"github.com/UnownHash/RotomNG/libs/logging"
	"github.com/UnownHash/RotomNG/libs/settings"
)

var (
	errJobNotFound = errors.New("job not found")
)

// IsErrJobNotFound checks if the error is a job-not-found error.
func IsErrJobNotFound(err error) bool {
	return errors.Is(err, errJobNotFound)
}

// RunJobResponse contains the result of a job execution on a device.
type RunJobResponse struct {
	CommandResult string `json:"commandResult"`
}

// DeviceConn defines the interface for a device connection capable of running jobs.
type DeviceConn interface {
	ID() string
	Origin() string
	RunJob(ctx context.Context, command string) (*RunJobResponse, error)
}

// Job represents a configured job definition loaded from the jobs directory.
type Job struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Exec        string `json:"exec"`
}

// JobInstanceStatus represents the current status of a job instance.
type JobInstanceStatus uint8

const (
	// JobInstanceStatusStarted indicates the job has been started.
	JobInstanceStatusStarted JobInstanceStatus = iota
	// JobInstanceStatusSucceeded indicates the job completed successfully.
	JobInstanceStatusSucceeded
	// JobInstanceStatusFailed indicates the job failed.
	JobInstanceStatusFailed

	maxJobInstanceStatusIndex
)

var jobInstanceStatusStrings = []string{"started", "succeeded", "failed"}

func (status JobInstanceStatus) String() string {
	if status >= maxJobInstanceStatusIndex {
		return "invalid"
	}
	return jobInstanceStatusStrings[status]
}

// JobInstance represents a single execution of a job on a device.
type JobInstance struct {
	ID           uint64
	JobID        string
	StartedAt    time.Time
	FinishedAt   time.Time
	DeviceID     string
	DeviceOrigin string
	Result       string
	Status       JobInstanceStatus
}

// ManagerSettings holds configuration for the jobs manager.
type ManagerSettings struct {
	JobsPath string
}

// Validate checks that the ManagerSettings fields are valid.
func (s ManagerSettings) Validate() error {
	if s.JobsPath == "" {
		return errors.New("jobs path cannot be empty string")
	}
	return nil
}

type managerSettingsContainer = settings.Container[ManagerSettings]

// ManagerConfig provides the configuration needed to create a Manager.
type ManagerConfig struct {
	*managerSettingsContainer

	Logger *slog.Logger
}

// Init initializes the ManagerConfig with the given settings.
func (cfg *ManagerConfig) Init(s ManagerSettings) (err error) {
	cfg.managerSettingsContainer, err = settings.NewContainer(s)
	return
}

// Manager manages job definitions and job instance execution.
type Manager struct {
	logger      *slog.Logger
	getSettings func() ManagerSettings

	wg               sync.WaitGroup
	mu               sync.Mutex
	jobsByID         map[string]Job
	jobInstanceID    uint64
	jobInstancesByID map[uint64]JobInstance
}

// NewManager creates a new Manager with the given configuration.
func NewManager(cfg ManagerConfig) *Manager {
	return &Manager{
		logger:           cfg.Logger,
		getSettings:      cfg.GetSettings,
		jobsByID:         make(map[string]Job),
		jobInstancesByID: make(map[uint64]JobInstance),
	}
}

// GetJobs returns all loaded jobs sorted by ID.
func (mgr *Manager) GetJobs() []Job {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	jobs := make([]Job, len(mgr.jobsByID))
	idx := 0
	for _, job := range mgr.jobsByID {
		jobs[idx] = job
		idx++
	}
	slices.SortFunc(jobs, func(a, b Job) int {
		if a.ID < b.ID {
			return -1
		}
		return 1
	})
	return jobs
}

// GetJobByID returns a job by its ID.
func (mgr *Manager) GetJobByID(jobID string) (Job, bool) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	job, ok := mgr.jobsByID[jobID]
	return job, ok
}

// GetJobInstances returns all job instances sorted newest first.
func (mgr *Manager) GetJobInstances() []JobInstance {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	jobInstances := make([]JobInstance, len(mgr.jobInstancesByID))
	idx := 0
	for _, jobInstance := range mgr.jobInstancesByID {
		jobInstances[idx] = jobInstance
		idx++
	}
	// newest first
	slices.SortFunc(jobInstances, func(a, b JobInstance) int {
		if a.ID < b.ID {
			return 1
		}
		return -1
	})
	return jobInstances
}

// GetJobInstanceByID returns a job instance by its ID.
func (mgr *Manager) GetJobInstanceByID(jobInstanceID uint64) (JobInstance, bool) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	jobInstance, ok := mgr.jobInstancesByID[jobInstanceID]
	return jobInstance, ok
}

// ClearJobInstances removes all completed job instances.
func (mgr *Manager) ClearJobInstances() {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	for _, jobInstance := range mgr.jobInstancesByID {
		if jobInstance.Status != JobInstanceStatusStarted {
			delete(mgr.jobInstancesByID, jobInstance.ID)
		}
	}
}

// ClearJobInstance removes a specific job instance by ID and returns whether it existed.
func (mgr *Manager) ClearJobInstance(jobInstanceID uint64) bool {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	_, ok := mgr.jobInstancesByID[jobInstanceID]
	if ok {
		delete(mgr.jobInstancesByID, jobInstanceID)
	}
	return ok
}

// AddFailedJobInstance creates and stores a failed job instance.
func (mgr *Manager) AddFailedJobInstance(jobID string, deviceID string, result string) JobInstance {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	mgr.jobInstanceID++

	now := time.Now()
	jobInstance := JobInstance{
		ID:         mgr.jobInstanceID,
		JobID:      jobID,
		StartedAt:  now,
		FinishedAt: now,
		DeviceID:   deviceID,
		Result:     result,
		Status:     JobInstanceStatusFailed,
	}

	mgr.jobInstancesByID[jobInstance.ID] = jobInstance
	return jobInstance
}

// RunJob executes a job on a device connection with the given timeout.
func (mgr *Manager) RunJob(ctx context.Context, jobID string, deviceConn DeviceConn, timeout time.Duration) JobInstance {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	mgr.jobInstanceID++

	jobInstance := JobInstance{
		ID:           mgr.jobInstanceID,
		JobID:        jobID,
		StartedAt:    time.Now(),
		DeviceID:     deviceConn.ID(),
		DeviceOrigin: deviceConn.Origin(),
	}

	job, ok := mgr.jobsByID[jobID]
	if !ok {
		jobInstance.FinishedAt = jobInstance.StartedAt
		jobInstance.Result = fmt.Sprintf("couldn't get job for id '%s': %v", jobID, errJobNotFound)
		jobInstance.Status = JobInstanceStatusFailed
		return jobInstance
	}

	mgr.jobInstancesByID[jobInstance.ID] = jobInstance

	ctx, cancelFn := context.WithTimeout(ctx, timeout)

	mgr.wg.Add(1)
	go func(jobInstance JobInstance) {
		var err error
		var runJobResponse *RunJobResponse

		defer func() { //nolint:contextcheck // deferred recovery handler, no parent context available
			defer mgr.wg.Done()
			cancelFn()
			if r := recover(); r != nil {
				logging.LogRecovery(mgr.logger, "panic caught during job run", r)
				err = fmt.Errorf("panic caught during job run: %v", r)
			}

			jobInstance.FinishedAt = time.Now()
			if err == nil {
				jobInstance.Status = JobInstanceStatusSucceeded
				jobInstance.Result = runJobResponse.CommandResult
			} else {
				jobInstance.Status = JobInstanceStatusFailed
				jobInstance.Result = err.Error()
			}

			mgr.mu.Lock()
			defer mgr.mu.Unlock()

			mgr.jobInstancesByID[jobInstance.ID] = jobInstance
		}()

		runJobResponse, err = deviceConn.RunJob(ctx, job.Exec)
	}(jobInstance)

	return jobInstance
}

// Reload reloads job definitions from the configured jobs directory.
func (mgr *Manager) Reload() error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	jobs, err := mgr.load()
	if err != nil {
		return err
	}

	mgr.jobsByID = jobs
	return nil
}

// Wait blocks until all running job goroutines have completed.
func (mgr *Manager) Wait() {
	mgr.wg.Wait()
}

func (mgr *Manager) load() (map[string]Job, error) {
	settings := mgr.getSettings()
	jobsPath := settings.JobsPath

	jobs := make(map[string]Job)

	// Walk through the jobs directory and find all .json files
	err := filepath.WalkDir(jobsPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-json files
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}

		// Read the file (path is from WalkDir, not user input)
		data, err := os.ReadFile(path) //nolint:gosec // path comes from filepath.WalkDir
		if err != nil {
			return fmt.Errorf("failed to read job file %s: %w", path, err)
		}

		// Try to unmarshal as a single Job first
		var singleJob Job
		if err := json.Unmarshal(data, &singleJob); err == nil {
			// Check for duplicate job ID
			if _, exists := jobs[singleJob.ID]; exists {
				return fmt.Errorf("duplicate job id '%s' found in file %s", singleJob.ID, path)
			}
			jobs[singleJob.ID] = singleJob
			return nil
		}

		// Try to unmarshal as a slice of Jobs
		var jobSlice []Job
		if err := json.Unmarshal(data, &jobSlice); err != nil {
			return fmt.Errorf("failed to unmarshal job file %s as either Job or []Job: %w", path, err)
		}

		// Add all jobs from the slice, checking for duplicates
		for _, job := range jobSlice {
			if _, exists := jobs[job.ID]; exists {
				return fmt.Errorf("duplicate job id '%s' found in file %s", job.ID, path)
			}
			jobs[job.ID] = job
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk jobs directory: %w", err)
	}

	return jobs, nil
}
