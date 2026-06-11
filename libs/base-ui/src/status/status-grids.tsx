import { StatusGrid } from "../components/status-grid";
import { TimeWindowStatsGrid } from "../components/time-window-stats-grid";
import type { Status } from "../types";
import { calculateControllerMetrics } from "../utils/controller-calculations";
import { calculateDeviceMetrics } from "../utils/device-calculations";
import { calculateWorkerMetrics } from "../utils/worker-calculations";

export const StatusGrids = ({ devices, controllers }: Status) => {
  const { totalControllers } = calculateControllerMetrics(controllers);
  const { enabledDevices, connectedDevices, totalDevices, inUseDevices } =
    calculateDeviceMetrics(devices);
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
      {hasWorkerStats ? (
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
