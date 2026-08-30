/**
 * Feature gating reads the config of whichever rotom-ng the operator is
 * looking at. Pointed at one directly that is just its config; fronted by the
 * admin service it is the selected instance's, so the UI's shape follows the
 * instance rather than the service proxying to it.
 */

import type { AppConfig } from "../types";
import { useConfig } from "./use-config";
import { useInstances } from "./use-instances";

export const useActiveConfig = (): AppConfig | undefined => {
  const { data } = useConfig();
  const { multiInstance, selected } = useInstances();

  if (!multiInstance) {
    return data?.config;
  }
  // Undefined until an instance has been selected and reached, which is
  // exactly when `InstanceGate` is showing its message instead of the app.
  return selected?.config;
};

/**
 * Whether the active instance is collecting worker request stats. The server
 * only sends `disable_worker_stats` when it is true, so an absent flag — or a
 * config that has not loaded yet — means enabled.
 */
export const useWorkerStatsEnabled = (): boolean => {
  const config = useActiveConfig();
  return config?.tuning?.disable_worker_stats !== true;
};
