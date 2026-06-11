package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/UnownHash/RotomNG/apps/rotom-ng/app/config"
	"github.com/UnownHash/RotomNG/libs/mitm"
	"github.com/UnownHash/RotomNG/libs/testutil"
)

// writeTestJobFile creates a single-job JSON file in the given directory.
// Returns the job ID for use in API calls.
func writeTestJobFile(t *testing.T, dir, jobID, exec string) string {
	t.Helper()
	job := map[string]string{
		"id":          jobID,
		"description": "test job",
		"exec":        exec,
	}
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	path := filepath.Join(dir, jobID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write job file: %v", err)
	}
	return jobID
}

// putRunJob sends PUT /api/job/:jobId/run with the given device IDs.
func putRunJob(t *testing.T, httpAddr, jobID string, deviceIDs []string) *http.Response {
	t.Helper()
	body := map[string][]string{"device_ids": deviceIDs}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal run job request: %v", err)
	}
	url := fmt.Sprintf("http://%s/api/job/%s/run", httpAddr, jobID)
	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("create PUT request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /api/job/%s/run: %v", jobID, err)
	}
	return resp
}

// TestJobs_DisabledReturnsNotFound verifies that all job API endpoints return
// 404 with "jobs are not enabled" when Jobs.Enable is false in the config.
func TestJobs_DisabledReturnsNotFound(t *testing.T) {
	env, err := testutil.NewTestEnv(testutil.WithJobsEnabled(false))
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	type endpoint struct {
		name   string
		method string
		path   string
		body   string
	}

	endpoints := []endpoint{
		{"GetJobs", http.MethodGet, "/api/job", ""},
		{"GetJob", http.MethodGet, "/api/job/some-id", ""},
		{"ReloadJobs", http.MethodPut, "/api/job/-/reload", ""},
		{"GetJobInstances", http.MethodGet, "/api/job-instance", ""},
		{"GetJobInstance", http.MethodGet, "/api/job-instance/123", ""},
		{"ClearJobInstance", http.MethodPut, "/api/job-instance/-/clear", ""},
		{"RunJob", http.MethodPut, "/api/job/some-id/run", `{"device_ids":["dev1"]}`},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			url := fmt.Sprintf("http://%s%s", env.HTTPAddr, ep.path)
			var req *http.Request
			if ep.body != "" {
				req, err = http.NewRequest(ep.method, url, bytes.NewBufferString(ep.body))
			} else {
				req, err = http.NewRequest(ep.method, url, nil)
			}
			if err != nil {
				t.Fatalf("create request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := testHTTPClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", ep.method, ep.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
			}

			var result map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got := result["error"]; got != "jobs are not enabled" {
				t.Errorf("error = %q, want %q", got, "jobs are not enabled")
			}
		})
	}
}

// TestJobs_ToggleEnabledViaSIGHUP verifies that toggling Jobs.Enable at
// runtime via SIGHUP config reload changes API behavior on the fly.
func TestJobs_ToggleEnabledViaSIGHUP(t *testing.T) {
	// Start with jobs enabled.
	var jobsEnabled atomic.Bool
	jobsEnabled.Store(true)
	reloadFn := func() (*config.Config, error) {
		cfg, _, err := testutil.NewTestConfig(
			testutil.WithJobsEnabled(jobsEnabled.Load()),
		)
		return cfg, err
	}

	env, err := testutil.NewTestEnv(
		testutil.WithJobsEnabled(true),
		testutil.WithReloadConfig(reloadFn),
	)
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	// Verify jobs endpoint works while enabled.
	resp, err := testHTTPClient.Get(fmt.Sprintf("http://%s/api/job", env.HTTPAddr))
	if err != nil {
		t.Fatalf("GET /api/job: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-reload: status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Disable jobs and reload via SIGHUP.
	jobsEnabled.Store(false)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("sending SIGHUP: %v", err)
	}

	// Wait for the setting change to take effect.
	err = testutil.WaitForCondition(func() bool {
		r, e := testHTTPClient.Get(fmt.Sprintf("http://%s/api/job", env.HTTPAddr))
		if e != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode == http.StatusNotFound
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for jobs disabled after SIGHUP: %v", err)
	}

	// Re-enable jobs and reload again.
	jobsEnabled.Store(true)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("sending SIGHUP: %v", err)
	}

	err = testutil.WaitForCondition(func() bool {
		r, e := testHTTPClient.Get(fmt.Sprintf("http://%s/api/job", env.HTTPAddr))
		if e != nil {
			return false
		}
		r.Body.Close()
		return r.StatusCode == http.StatusOK
	}, waitTimeout)
	if err != nil {
		t.Fatalf("waiting for jobs re-enabled after SIGHUP: %v", err)
	}
}

func TestJobs_DispatchAndResult(t *testing.T) {
	const knownResult = "test-result-abc123"

	// Custom handler for runJob commands
	customHandler := func(cmd mitm.DeviceCommandRequest) mitm.DeviceCommandReply {
		if cmd.Method == "runJob" {
			body, _ := json.Marshal(map[string]string{"commandResult": knownResult}) //nolint:errchkjson
			return mitm.DeviceCommandReply{ID: cmd.ID, Status: 200, Body: body}
		}
		// For other commands (getMemoryUsage, getScreenSize), return generic success
		return mitm.DeviceCommandReply{ID: cmd.ID, Status: 200, Body: json.RawMessage("{}")}
	}

	// Create env manually (not startTestEnv) to write job file before Start
	env, err := testutil.NewTestEnv()
	if err != nil {
		t.Fatalf("NewTestEnv: %v", err)
	}
	jobID := writeTestJobFile(t, env.Config.Jobs.Path, "test-job", "echo hello")
	if err := env.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { env.Stop() })
	if err := env.WaitReady(5 * time.Second); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}

	ctx := context.Background()
	device, worker := connectDeviceWithWorker(ctx, t, env.DeviceAddr,
		testutil.WithCommandHandler(customHandler))
	defer device.Close()
	defer worker.Close()

	// Wait for device to be registered
	err = testutil.WaitForCondition(func() bool {
		status := getStatus(t, env.HTTPAddr)
		return findDeviceInList(status.Devices, device.DeviceID()) != nil
	}, waitTimeout)
	if err != nil {
		t.Fatalf("device not registered: %v", err)
	}

	// Track instance ID across subtests
	var instanceID uint64

	t.Run("job_dispatched_to_device", func(t *testing.T) {
		resp := putRunJob(t, env.HTTPAddr, jobID, []string{device.DeviceID()})
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PUT /api/job/%s/run: status %d, want %d", jobID, resp.StatusCode, http.StatusOK)
		}

		var runResp jobInstancesResponse
		if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
			t.Fatalf("decode run job response: %v", err)
		}

		if len(runResp.Instances) == 0 {
			t.Fatal("no job instances returned from PUT /api/job/:jobId/run")
		}

		instanceID = runResp.Instances[0].ID

		// Verify the instance has the correct job ID
		if got, want := runResp.Instances[0].JobID, jobID; got != want {
			t.Errorf("instance JobId = %q, want %q", got, want)
		}
	})

	t.Run("result_collected_correctly", func(t *testing.T) {
		if instanceID == 0 {
			t.Skip("skipping: no instance ID from previous subtest")
		}

		var finalInstance apiJobInstance

		err := testutil.WaitForCondition(func() bool {
			instances := getJobInstances(t, env.HTTPAddr)
			for _, inst := range instances.Instances {
				if inst.ID == instanceID {
					finalInstance = inst
					return inst.Status == "succeeded"
				}
			}
			return false
		}, 15*time.Second)
		if err != nil {
			t.Fatalf("job did not succeed within timeout: %v", err)
		}

		if finalInstance.Status != "succeeded" {
			t.Errorf("status = %q, want %q", finalInstance.Status, "succeeded")
		}
		if finalInstance.Result != knownResult {
			t.Errorf("result = %q, want %q", finalInstance.Result, knownResult)
		}
		if finalInstance.JobID != jobID {
			t.Errorf("job_id = %q, want %q", finalInstance.JobID, jobID)
		}
		if finalInstance.DeviceID != device.DeviceID() {
			t.Errorf("device_id = %q, want %q", finalInstance.DeviceID, device.DeviceID())
		}
	})
}
