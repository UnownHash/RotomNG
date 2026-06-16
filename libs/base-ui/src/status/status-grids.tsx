import { StatusGrid } from "../components/status-grid";
import { TimeWindowStatsGrid } from "../components/time-window-stats-grid";
import { useWorkerStatsEnabled } from "../hooks/use-config";
import type { Status } from "../types";
import { calculateControllerMetrics } from "../utils/controller-calculations";
import { calculateDeviceMetrics } from "../utils/device-calculations";
import {
  calculateWorkerMetrics,
  requestStatsValues,
} from "../utils/worker-calculations";

export const StatusGrids = ({ devices, controllers, global_stats }: Status) => {
  const workerStatsEnabled = useWorkerStatsEnabled();
  const { totalControllers } = calculateControllerMetrics(controllers);
  const { enabledDevices, connectedDevices, totalDevices, inUseDevices } =
    calculateDeviceMetrics(devices);
  const { workersInUse, workersAvailable, workersEnabled, workersTotal } =
    calculateWorkerMetrics(devices);

  return (
    <div className="mb-4">
      {/* Controllers Grid */}
      <StatusGrid
        title="Controllers"
        accent="cyan"
        headers={["Total"]}
        values={[totalControllers]}
      />

      {/* Devices Grid */}
      <StatusGrid
        title="Devices"
        accent="violet"
        headers={["In Use", "Connected", "Enabled", "Total"]}
        values={[inUseDevices, connectedDevices, enabledDevices, totalDevices]}
      />

      {/* Workers Grid - with or without Request Stats */}
      {workerStatsEnabled ? (
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
        />
      ) : workersTotal > 0 ? (
        <StatusGrid
          title="Workers"
          accent="emerald"
          headers={["In Use", "Available", "Enabled", "Total"]}
          values={[
            workersInUse,
            workersAvailable,
            workersEnabled,
            workersTotal,
          ]}
        />
      ) : null}
    </div>
  );
};
