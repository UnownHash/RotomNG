import { generateConfig } from "./data/config";
import { generateControllers } from "./data/controllers";
import { generateDevices } from "./data/devices";
import { generateJobInstances } from "./data/job-instances";
import { generateJobs } from "./data/jobs";
import { PROFILE_SIZES, readMockOptions } from "./profiles";
import type {
  MockController,
  MockDevice,
  MockJobInstance,
  MockJobInstances,
  MockJobs,
} from "./types";

const options = readMockOptions();
const counts = PROFILE_SIZES[options.profile];

const initialControllers = generateControllers(counts.controllers);
const initialDevices = generateDevices(counts.devices, {
  workersTotal: counts.workers,
});
const initialJobs = generateJobs(counts.jobs);
const initialInstances = generateJobInstances(counts.instances, initialJobs);

interface MockState {
  devices: MockDevice[];
  controllers: MockController[];
  jobs: MockJobs;
  instances: MockJobInstances;
  live: boolean;
}

export const mockState: MockState = {
  devices: initialDevices,
  controllers: initialControllers,
  jobs: initialJobs,
  instances: initialInstances,
  live: options.live,
};

export const buildConfigResponse = () => generateConfig();

const randInRange = (n: number, jitter: number) =>
  n * (1 + (Math.random() - 0.5) * 2 * jitter);

/** Apply live-mode jitter to every status response. */
export const applyLiveJitter = () => {
  if (!mockState.live) return;
  const now = Date.now();
  for (const device of mockState.devices) {
    device.last_seen_at_ms = now;
    device.message_last_received_at_ms = now;
    device.message_last_sent_at_ms = now - 500;
    for (const w of device.workers ?? []) {
      w.last_seen_at_ms = now;
      w.message_last_received_at_ms = now;
      w.message_last_sent_at_ms = now - 500;
      if (Math.random() < 0.1) w.is_connected = !w.is_connected;
      if (w.time_windowed_stats) {
        const s = w.time_windowed_stats;
        s.requests_rate_over_30_seconds = Math.max(
          0,
          randInRange(s.requests_rate_over_30_seconds, 0.1),
        );
        s.requests_rate_over_1_min = Math.max(
          0,
          randInRange(s.requests_rate_over_1_min, 0.1),
        );
        s.requests_rate_over_5_min = Math.max(
          0,
          randInRange(s.requests_rate_over_5_min, 0.1),
        );
        s.requests_rate_over_15_min = Math.max(
          0,
          randInRange(s.requests_rate_over_15_min, 0.1),
        );
        s.request_ms_avg_over_30_seconds = Math.max(
          0,
          randInRange(s.request_ms_avg_over_30_seconds, 0.05),
        );
        s.request_ms_avg_over_1_min = Math.max(
          0,
          randInRange(s.request_ms_avg_over_1_min, 0.05),
        );
        s.request_ms_avg_over_5_min = Math.max(
          0,
          randInRange(s.request_ms_avg_over_5_min, 0.05),
        );
        s.request_ms_avg_over_15_min = Math.max(
          0,
          randInRange(s.request_ms_avg_over_15_min, 0.05),
        );
      }
    }
  }
};

let appendCounter = 0;

export const appendJobInstance = (jobId: string): MockJobInstance => {
  const instance: MockJobInstance = {
    // Monotonic suffix avoids same-ms collisions when the UI fires
    // successive "Run" clicks before the timer rolls.
    id: `instance-${Date.now()}-${++appendCounter}`,
    job_id: jobId,
    started_at_ms: Date.now(),
    status: "running",
  };
  mockState.instances.push(instance);
  return instance;
};

export const clearJobInstances = () => {
  mockState.instances = [];
};

export const removeDevice = (id: string) => {
  mockState.devices = mockState.devices.filter((d) => d.id !== id);
};

export const removeDeadDevices = () => {
  mockState.devices = mockState.devices.filter((d) => d.is_connected);
};

export const setDeviceConnected = (id: string, connected: boolean) => {
  const device = mockState.devices.find((d) => d.id === id);
  if (device) device.is_connected = connected;
};
