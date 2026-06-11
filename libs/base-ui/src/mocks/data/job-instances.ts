import type { MockJobInstance, MockJobInstances, MockJobs } from "../types";

const randInt = (min: number, max: number) =>
  Math.floor(Math.random() * (max - min + 1)) + min;

export const generateJobInstances = (
  count: number,
  jobs: MockJobs,
): MockJobInstances => {
  if (jobs.length === 0) return [];
  const statuses: MockJobInstance["status"][] = [
    "running",
    "success",
    "failed",
  ];
  return Array.from({ length: count }, (_, i) => {
    const job = jobs[i % jobs.length];
    const status = statuses[i % statuses.length] ?? ("success" as const);
    const startedAt = Date.now() - randInt(1000, 60 * 60_000 * 24);
    return {
      id: `instance-${i}`,
      job_id: job?.id ?? "job-0",
      started_at_ms: startedAt,
      finished_at_ms:
        status === "running" ? undefined : startedAt + randInt(1000, 60_000),
      status,
      error: status === "failed" ? "mock error: timeout" : undefined,
    };
  });
};
