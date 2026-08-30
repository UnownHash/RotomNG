import { TimeWindowStatsGrid } from "../components/time-window-stats-grid";
import { useWorkerStatsEnabled } from "../hooks/use-active-config";
import type { Status } from "../types";
import {
  calculateWorkerMetrics,
  requestStatsValues,
} from "../utils/worker-calculations";

export const WorkerGrids = ({
  devices,
  global_stats,
}: Pick<Status, "devices" | "global_stats">) => {
  const workerStatsEnabled = useWorkerStatsEnabled();
  const { workersInUse, workersAvailable, workersEnabled, workersTotal } =
    calculateWorkerMetrics(devices);

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
        {...requestStatsValues(global_stats)}
        hasStats={workerStatsEnabled}
        hideIfNoStats={true}
      />
    </div>
  );
};
