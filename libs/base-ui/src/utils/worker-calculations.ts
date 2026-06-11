import type { Device } from "../types";

export interface WorkerMetrics {
  workersInUse: number;
  workersAvailable: number;
  workersEnabled: number;
  workersTotal: number;
  workersRequestsPerSecond30s: number;
  workersAvgRequestDuration30s: number;
  workersRequestsPerSecond1m: number;
  workersAvgRequestDuration1m: number;
  workersRequestsPerSecond5m: number;
  workersAvgRequestDuration5m: number;
  workersRequestsPerSecond15m: number;
  workersAvgRequestDuration15m: number;
  hasWorkerStats: boolean;
}

export const calculateWorkerMetrics = (devices: Device[]): WorkerMetrics => {
  let workersInUse = 0;
  let workersAvailable = 0;
  let workersEnabled = 0;
  let workersTotal = 0;
  let workersRequestsPerSecond30s = 0;
  let workersAvgRequestDuration30s = 0;
  let workersRequestsPerSecond1m = 0;
  let workersAvgRequestDuration1m = 0;
  let workersRequestsPerSecond5m = 0;
  let workersAvgRequestDuration5m = 0;
  let workersRequestsPerSecond15m = 0;
  let workersAvgRequestDuration15m = 0;
  let hasWorkerStats = false;
  let workersWithStats = 0;

  devices.forEach((device) => {
    (device.workers || []).forEach((worker) => {
      workersTotal++;
      if (worker.is_in_use) workersInUse++;
      if (worker.can_be_used && !worker.is_in_use) workersAvailable++;
      if (worker.can_be_used) workersEnabled++;
      if (worker.time_windowed_stats) {
        hasWorkerStats = true;
        workersWithStats++;
        workersRequestsPerSecond30s +=
          worker.time_windowed_stats.requests_rate_over_30_seconds;
        workersAvgRequestDuration30s +=
          worker.time_windowed_stats.request_ms_avg_over_30_seconds;
        workersRequestsPerSecond1m +=
          worker.time_windowed_stats.requests_rate_over_1_min;
        workersAvgRequestDuration1m +=
          worker.time_windowed_stats.request_ms_avg_over_1_min;
        workersRequestsPerSecond5m +=
          worker.time_windowed_stats.requests_rate_over_5_min;
        workersAvgRequestDuration5m +=
          worker.time_windowed_stats.request_ms_avg_over_5_min;
        workersRequestsPerSecond15m +=
          worker.time_windowed_stats.requests_rate_over_15_min;
        workersAvgRequestDuration15m +=
          worker.time_windowed_stats.request_ms_avg_over_15_min;
      }
    });
  });

  if (workersWithStats > 0) {
    workersAvgRequestDuration30s /= workersWithStats;
    workersAvgRequestDuration1m /= workersWithStats;
    workersAvgRequestDuration5m /= workersWithStats;
    workersAvgRequestDuration15m /= workersWithStats;
  }

  return {
    workersInUse,
    workersAvailable,
    workersEnabled,
    workersTotal,
    workersRequestsPerSecond30s,
    workersAvgRequestDuration30s,
    workersRequestsPerSecond1m,
    workersAvgRequestDuration1m,
    workersRequestsPerSecond5m,
    workersAvgRequestDuration5m,
    workersRequestsPerSecond15m,
    workersAvgRequestDuration15m,
    hasWorkerStats,
  };
};
