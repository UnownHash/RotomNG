import type { MockJob, MockJobs } from "../types";

const SAMPLE_NAMES = [
  "daily-sync",
  "hourly-cleanup",
  "reload-controllers",
  "flush-cache",
  "refresh-tokens",
  "warm-cache",
  "prune-instances",
  "scan-devices",
];

export const generateJobs = (count: number): MockJobs => {
  const out: MockJobs = [];
  for (let i = 0; i < count; i++) {
    const job: MockJob = {
      id: `job-${i}`,
      name: SAMPLE_NAMES[i % SAMPLE_NAMES.length] ?? `job-${i}`,
      enabled: Math.random() > 0.2,
      cron_schedule: i % 3 === 0 ? "0 */6 * * *" : "0 0 * * *",
    };
    out.push(job);
  }
  return out;
};
