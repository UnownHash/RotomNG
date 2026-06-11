import { StatusGrid } from "../components/status-grid";
import type { Status } from "../types";
import { calculateDeviceMetrics } from "../utils/device-calculations";
import { calculateWorkerMetrics } from "../utils/worker-calculations";

export const DeviceGrids = ({ devices }: Status) => {
  // Calculate device metrics using shared utility
  const { enabledDevices, connectedDevices, totalDevices, inUseDevices } =
    calculateDeviceMetrics(devices);

  // Calculate worker metrics
  const { workersInUse, workersAvailable, workersEnabled, workersTotal } =
    calculateWorkerMetrics(devices);

  return (
    <div className="mb-4">
      {/* Devices Grid */}
      <StatusGrid
        title="Devices"
        accent="violet"
        headers={["In Use", "Connected", "Enabled", "Total"]}
        values={[inUseDevices, connectedDevices, enabledDevices, totalDevices]}
      />

      {/* Workers Grid */}
      <StatusGrid
        title="Workers"
        accent="emerald"
        headers={["In Use", "Available", "Enabled", "Total"]}
        values={[workersInUse, workersAvailable, workersEnabled, workersTotal]}
      />
    </div>
  );
};
