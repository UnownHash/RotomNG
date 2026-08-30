export interface RateLimitConfig {
  enable: boolean;
  max_selections: number;
  duration: string;
}

export interface TuningConfig {
  profiling: boolean;
  worker_selection_type?: string;
  // Only present (true) when worker stats collection is disabled server-side.
  disable_worker_stats?: boolean;
}

export interface JobsConfig {
  enable: boolean;
  path: string;
}

/**
 * One rotom-ng server fronted by the admin UI service, as reported in that
 * service's `/api/config`.
 */
export interface InstanceInfo {
  /**
   * The name that instance reports in its own config. Empty when it has no
   * instance name set, or when it has not been reached yet — the picker falls
   * back to showing the url in that case.
   */
  instance: string;
  /**
   * The instance's base url. This is its stable identity: it is what the UI
   * sends back to select an instance, and what it keys its stored selection
   * on. Names can be empty or repeated; urls cannot.
   */
  url: string;
  /**
   * True only while the service's most recent probe of the instance
   * succeeded, which also means `config` is populated. The UI refuses to
   * select an instance that is not reachable.
   */
  reachable: boolean;
  /**
   * That instance's own config, exactly as it would answer `/api/config` if
   * the UI were pointed at it directly. Absent until first contact. Feature
   * gating reads this rather than the top-level config, so what the UI shows
   * follows the selected instance.
   */
  config?: AppConfig;
}

export interface AppConfig {
  version: string;
  sha: string;
  instance?: string;
  /** Absent on the admin UI service, which runs no workers of its own. */
  tuning?: TuningConfig;
  rate_limit?: RateLimitConfig;
  jobs?: JobsConfig;
  /**
   * Present only when talking to the admin UI service — always, even as an
   * empty list. A plain rotom-ng never sends it, so its presence is what puts
   * the UI into multi-instance mode.
   */
  instances?: InstanceInfo[];
}

export interface ConfigResponse {
  status: string;
  config: AppConfig;
}
