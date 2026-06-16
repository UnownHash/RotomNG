import type { Controller, Device, TimeWindowedStats } from "./connections";

// New status structure matching the server API
export interface Status {
  devices: Device[];
  controllers: Controller[];
  // Aggregate request stats across all workers, including disconnected ones.
  // Preferred over summing per-worker stats for overall req/s and avg ms.
  global_stats?: TimeWindowedStats;
}
