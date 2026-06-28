// Common stats interface - matches Go CommonStats struct
export interface CommonStats {
  message_last_received_at_ms: number;
  messages_received: number;
  bytes_received: number;
  message_last_sent_at_ms: number;
  messages_sent: number;
  bytes_sent: number;
  // Most recent activity (incl. ping/pong keep-alive), not just data messages.
  last_seen_at_ms: number;
}

// Device Memory interface - matches Go DeviceMemory struct
export interface DeviceMemory {
  free: number;
  mitm: number;
  start: number;
  [key: string]: number;
}

// Device Session - matches Go DeviceSession structure
export interface DeviceSession extends CommonStats {
  connected_at_ms: number;
}

// Device - matches Go Device structure (base fields only)
export interface Device extends CommonStats {
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
  enabled: boolean;
  is_connected: boolean;
  is_in_use: boolean;
  can_be_used: boolean;
  last_memory?: DeviceMemory;
  workers?: Worker[];
  session?: DeviceSession;
  [key: string]: unknown;
}

// Controller - matches Go ControllerResponse structure (base fields only)
export interface Controller extends CommonStats {
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

// Time Window Statistics interface - matches Go TimeWindowedStats struct
export interface TimeWindowedStats {
  requests_rate_over_30_seconds: number;
  requests_rate_over_1_min: number;
  requests_rate_over_5_min: number;
  requests_rate_over_15_min: number;
  request_ms_avg_over_30_seconds: number;
  request_ms_avg_over_1_min: number;
  request_ms_avg_over_5_min: number;
  request_ms_avg_over_15_min: number;
}

// Worker Session - matches Go WorkerSession structure
export interface WorkerSession extends CommonStats {
  connected_at_ms: number;
  controller?: Controller;
}

// Worker - matches Go Worker structure (base fields only)
export interface Worker extends CommonStats {
  id: string;
  device_id: string;
  origin: string;
  version_code: number;
  version_name: string;
  stats_disabled?: boolean;
  user_agent: string;
  last_connected_at_ms: number;
  is_connected: boolean;
  is_in_use: boolean;
  weight?: number;
  can_be_used: boolean;
  platform: string;
  session?: WorkerSession;
  time_windowed_stats?: TimeWindowedStats;
  [key: string]: unknown;
}
