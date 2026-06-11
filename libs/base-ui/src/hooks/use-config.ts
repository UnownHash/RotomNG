import { useQuery } from "@tanstack/react-query";
import { configQuery } from "../lib/query-options";

export const useConfig = () => {
  return useQuery(configQuery());
};
