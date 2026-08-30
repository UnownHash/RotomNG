import type { AppConfig, ConfigResponse, InstanceInfo } from "../../types";

export const generateConfig = (): ConfigResponse => ({
  status: "ok",
  config: {
    version: "1.0.5-mock",
    sha: "mockmockmockmock",
    instance: "mock-instance",
    tuning: { profiling: false },
  },
});

/** The per-instance configs the mock admin service reports. */
const MOCK_INSTANCES: { name: string; url: string; config: AppConfig }[] = [
  {
    name: "mock-east",
    url: "http://10.0.0.10:7072",
    config: {
      version: "1.0.5-mock",
      sha: "mockmockmockmock",
      instance: "mock-east",
      tuning: { profiling: false },
      jobs: { enable: true, path: "./jobs" },
    },
  },
  {
    // No instance name set upstream, so the picker falls back to the url --
    // worth having in the mock because it is easy to get wrong.
    name: "",
    url: "http://10.0.0.11:7072",
    config: {
      version: "1.0.5-mock",
      sha: "mockmockmockmock",
      // Jobs off here, so switching visibly adds and removes the Jobs tab.
      tuning: { profiling: false },
    },
  },
  {
    name: "mock-west",
    url: "http://10.0.0.12:7072",
    config: {
      version: "1.0.4-mock",
      sha: "mockmockmockmock",
      instance: "mock-west",
      tuning: { profiling: false, disable_worker_stats: true },
    },
  },
];

/**
 * The config the admin UI service serves: this service's own details plus the
 * instances it fronts.
 *
 * `flapping` toggles the first instance's reachability, which is how the mock
 * reaches the "current instance not reachable" state — an operator cannot
 * select an unreachable instance, so it has to go down under them.
 */
export const generateAdminConfig = (flapping: boolean): ConfigResponse => {
  const instances: InstanceInfo[] = MOCK_INSTANCES.map((instance, index) => {
    // The last instance is always down, so the picker always has a disabled
    // entry to show.
    const permanentlyDown = index === MOCK_INSTANCES.length - 1;
    const reachable = permanentlyDown
      ? false
      : !(index === 0 && flapping && flapIsDown());

    return {
      instance: instance.name,
      url: instance.url,
      reachable,
      // Retained even while unreachable, exactly as the service does.
      config: instance.config,
    };
  });

  return {
    status: "ok",
    config: {
      version: "1.0.5-mock",
      sha: "mockmockmockmock",
      instance: "mock-admin",
      instances,
    },
  };
};

/** Down for 15s out of every 45, off the wall clock so it needs no state. */
const flapIsDown = (): boolean => Date.now() % 45_000 > 30_000;
