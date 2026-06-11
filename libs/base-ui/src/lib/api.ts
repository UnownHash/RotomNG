/**
 * Centralized fetch helpers for the rotom UI. Each function returns the
 * parsed JSON body typed against the matching base-ui type. Pages should
 * import the `*Query` factory from `query-options.ts` rather than calling
 * these directly — the factories pin the queryKey + cache options.
 *
 * Throwing on non-2xx means TanStack Query sees the failure, hits its retry
 * logic, and surfaces an `error` to the consumer. Without the check, a 500
 * + html body would silently parse to `null` and the UI would mis-render.
 */

import type { ConfigResponse, JobInstances, Jobs, Status } from "../types";

const okJson = async <T>(res: Response): Promise<T> => {
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
};

export const fetchStatus = (): Promise<Status> =>
  fetch("/api/status").then(okJson<Status>);

export const fetchConfig = (): Promise<ConfigResponse> =>
  fetch("/api/config").then(okJson<ConfigResponse>);

export const fetchJobs = (): Promise<{ jobs: Jobs }> =>
  fetch("/api/job").then(okJson<{ jobs: Jobs }>);

export const fetchJobInstances = (): Promise<{ instances: JobInstances }> =>
  fetch("/api/job-instance").then(okJson<{ instances: JobInstances }>);
