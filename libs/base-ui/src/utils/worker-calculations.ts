import type { Device, TimeWindowedStats } from "../types";

export interface WorkerMetrics {
  workersInUse: number;
  workersAvailable: number;
  workersEnabled: number;
  workersTotal: number;
}

// RequestStatsValues maps directly onto TimeWindowStatsGrid's request-stat props.
export interface RequestStatsValues {
  requestsPerSecond30s: number;
  requestsPerSecond1m: number;
  requestsPerSecond5m: number;
  requestsPerSecond15m: number;
  avgRequestDuration30s: number;
  avgRequestDuration1m: number;
  avgRequestDuration5m: number;
  avgRequestDuration15m: number;
}

// requestStatsValues converts the server's global aggregate request stats into
// the values displayed by TimeWindowStatsGrid. The global aggregate is accurate
// even as workers connect and disconnect, unlike summing per-worker stats.
export const requestStatsValues = (
  stats?: TimeWindowedStats,
): RequestStatsValues => ({
  requestsPerSecond30s: stats?.requests_rate_over_30_seconds ?? 0,
  requestsPerSecond1m: stats?.requests_rate_over_1_min ?? 0,
  requestsPerSecond5m: stats?.requests_rate_over_5_min ?? 0,
  requestsPerSecond15m: stats?.requests_rate_over_15_min ?? 0,
  avgRequestDuration30s: stats?.request_ms_avg_over_30_seconds ?? 0,
  avgRequestDuration1m: stats?.request_ms_avg_over_1_min ?? 0,
  avgRequestDuration5m: stats?.request_ms_avg_over_5_min ?? 0,
  avgRequestDuration15m: stats?.request_ms_avg_over_15_min ?? 0,
});

export const calculateWorkerMetrics = (devices: Device[]): WorkerMetrics => {
  let workersInUse = 0;
  let workersAvailable = 0;
  let workersEnabled = 0;
  let workersTotal = 0;

  devices.forEach((device) => {
    (device.workers || []).forEach((worker) => {
      workersTotal++;
      if (worker.is_in_use) workersInUse++;
      if (worker.can_be_used && !worker.is_in_use) workersAvailable++;
      if (worker.can_be_used) workersEnabled++;
    });
  });

  return {
    workersInUse,
    workersAvailable,
    workersEnabled,
    workersTotal,
  };
};
