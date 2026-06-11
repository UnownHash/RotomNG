import { TimeWindowStatsGrid } from "../components/time-window-stats-grid";
import type { Status } from "../types";
import { calculateWorkerMetrics } from "../utils/worker-calculations";

export const WorkerGrids = ({ devices }: Pick<Status, "devices">) => {
  const {
    workersInUse,
    workersAvailable,
    workersEnabled,
    workersTotal,
    workersRequestsPerSecond30s,
    workersAvgRequestDuration30s,
    workersRequestsPerSecond1m,
    workersAvgRequestDuration1m,
    workersRequestsPerSecond5m,
    workersAvgRequestDuration5m,
    workersRequestsPerSecond15m,
    workersAvgRequestDuration15m,
    hasWorkerStats,
  } = calculateWorkerMetrics(devices);

  return (
    <div className="mb-4">
      <TimeWindowStatsGrid
        title="Workers"
        accent="emerald"
        basicHeaders={["In Use", "Available", "Enabled", "Total"]}
        basicValues={[
          workersInUse,
          workersAvailable,
          workersEnabled,
          workersTotal,
        ]}
        requestsPerSecond30s={workersRequestsPerSecond30s}
        requestsPerSecond1m={workersRequestsPerSecond1m}
        requestsPerSecond5m={workersRequestsPerSecond5m}
        requestsPerSecond15m={workersRequestsPerSecond15m}
        avgRequestDuration30s={workersAvgRequestDuration30s}
        avgRequestDuration1m={workersAvgRequestDuration1m}
        avgRequestDuration5m={workersAvgRequestDuration5m}
        avgRequestDuration15m={workersAvgRequestDuration15m}
        hasStats={hasWorkerStats}
        hideIfNoStats={true}
      />
    </div>
  );
};
