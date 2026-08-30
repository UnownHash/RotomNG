import { useQuery } from "@tanstack/react-query";
import { configQuery } from "../lib/query-options";

/**
 * The raw `/api/config` reply.
 *
 * Fronted by the admin service this describes the *service*, not the rotom-ng
 * whose devices are on screen. Feature gating wants `useActiveConfig` instead,
 * which follows the selected instance.
 */
export const useConfig = () => {
  return useQuery(configQuery());
};
