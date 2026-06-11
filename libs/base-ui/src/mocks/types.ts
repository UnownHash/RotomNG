/**
 * Mock-only types that mirror the runtime shape returned by the RotomNG API.
 * Handlers return JSON with these shapes so the UI can be developed without
 * a running backend.
 */

import type {
  CommonStats,
  DeviceMemory,
  DeviceSession,
  TimeWindowedStats,
  WorkerSession,
} from "../types";

export interface MockController extends CommonStats {
  id: string;
  uuid: string;
  user_agent: string;
  weight: number;
  proto_major_version: number;
  proto_minor_version: number;
  worker_id?: string;
  account_username: string;
  account_source: string;
  connected_at_ms: number;
}

export interface MockWorker extends CommonStats {
  id: string;
  device_id: string;
  origin: string;
  version_code: number;
  version_name: string;
  stats_disabled?: boolean;
  user_agent: string;
  last_connected_at_ms: number;
  last_seen_at_ms: number;
  is_connected: boolean;
  is_in_use: boolean;
  weight?: number;
  can_be_used: boolean;
  platform: string;
  session?: WorkerSession;
  time_windowed_stats?: TimeWindowedStats;
}

export interface MockDevice extends CommonStats {
  id: string;
  origin: string;
  version: string;
  public_ip: string;
  worker_count: number;
  worker_in_use_count: number;
  worker_in_use_percent: number;
  worker_in_use_weight: number;
  worker_in_use_weight_percent: number;
  worker_max_weight: number;
  last_connected_at_ms: number;
  last_seen_at_ms: number;
  enabled: boolean;
  is_connected: boolean;
  is_in_use: boolean;
  can_be_used: boolean;
  last_memory?: DeviceMemory;
  workers?: MockWorker[];
  session?: DeviceSession;
}

export interface MockJob {
  id: string;
  name: string;
  enabled: boolean;
  cron_schedule?: string;
}
export type MockJobs = MockJob[];

export interface MockJobInstance {
  id: string;
  job_id: string;
  started_at_ms: number;
  finished_at_ms?: number;
  status: "running" | "success" | "failed";
  error?: string;
}
export type MockJobInstances = MockJobInstance[];

export interface MockProfileCounts {
  devices: number;
  controllers: number;
  workers: number;
  jobs: number;
  instances: number;
}
