export interface RateLimitConfig {
  enable: boolean;
  max_selections: number;
  duration: string;
}

export interface TuningConfig {
  profiling: boolean;
  worker_selection_type?: string;
}

export interface JobsConfig {
  enable: boolean;
  path: string;
}

export interface AppConfig {
  version: string;
  sha: string;
  instance?: string;
  tuning: TuningConfig;
  rate_limit?: RateLimitConfig;
  jobs?: JobsConfig;
}

export interface ConfigResponse {
  status: string;
  config: AppConfig;
}
