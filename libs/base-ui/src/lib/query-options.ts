/**
 * TanStack Query `queryOptions()` factories — the single source of truth
 * for every queryKey, queryFn, refetchInterval, and staleTime used by the
 * app. Callers spread the returned object into `useQuery`:
 *
 *     const { data } = useQuery(statusQuery())
 *     const { data: devices } = useQuery({
 *       ...statusQuery(),
 *       select: (s) => s.devices,
 *     })
 *
 * Renaming a key or tuning a poll interval changes one line here instead
 * of N call sites. The `as const` on each queryKey gives v5's strict typing
 * — invalidations land on the right cache entries.
 */

import { queryOptions } from "@tanstack/react-query";
import { fetchConfig, fetchJobInstances, fetchJobs, fetchStatus } from "./api";

/** Master poll interval for live-ish dashboard endpoints. */
export const POLL_INTERVAL_MS = 5000;

/** GET /api/status — devices + controllers. Polls. */
export const statusQuery = () =>
  queryOptions({
    queryKey: ["status"] as const,
    queryFn: fetchStatus,
    refetchInterval: POLL_INTERVAL_MS,
  });

/** GET /api/config — app config (version, tuning, etc). Polls. */
export const configQuery = () =>
  queryOptions({
    queryKey: ["config"] as const,
    queryFn: fetchConfig,
    refetchInterval: POLL_INTERVAL_MS,
  });

/** GET /api/job — job definitions. Jobs change rarely; staleTime: Infinity. */
export const jobsQuery = () =>
  queryOptions({
    queryKey: ["jobs"] as const,
    queryFn: fetchJobs,
    staleTime: Number.POSITIVE_INFINITY,
  });

/** GET /api/job-instance — recent job runs. Polls. */
export const jobInstancesQuery = () =>
  queryOptions({
    queryKey: ["jobInstances"] as const,
    queryFn: fetchJobInstances,
    refetchInterval: POLL_INTERVAL_MS,
  });
