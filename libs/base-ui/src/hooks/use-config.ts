import { useQuery } from "@tanstack/react-query";
import { configQuery } from "../lib/query-options";

export const useConfig = () => {
  return useQuery(configQuery());
};

// useWorkerStatsEnabled reports whether the server is collecting worker request
// stats, based on /api/config. The server only sends disable_worker_stats when
// it's true, so an absent flag (or not-yet-loaded config) means enabled.
export const useWorkerStatsEnabled = (): boolean => {
  const { data } = useConfig();
  return data?.config?.tuning?.disable_worker_stats !== true;
};
