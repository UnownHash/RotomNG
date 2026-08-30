import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { StaleDataBanner } from "../components/stale-data-banner";
import { statusQuery } from "../lib/query-options";

import { WorkerGrids } from "./worker-grids";
import { MemoizedWorkersTable as WorkersTable } from "./workers-table";

export const WorkersPage = () => {
  const { isLoading, isFetching, error, data } = useQuery({
    ...statusQuery(),
    select: (s) => ({ devices: s.devices, global_stats: s.global_stats }),
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[200px]">
        <Loader2 className="size-8 animate-spin text-(--brand)" />
      </div>
    );
  }

  // Guard on the data, not on isSuccess. A refetch that fails while cached data
  // is present flips status to "error" and isSuccess to false but leaves data
  // in place, so keying the full-page error off isSuccess would still throw the
  // whole view away on a single failed poll.
  if (!data) {
    return (
      <div className="p-4">
        <p className="text-destructive">
          An error has occurred: {error?.message}
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <StaleDataBanner error={error} />
      <div>
        <div className="flex items-center mb-4">
          <h1 className="text-3xl font-bold tracking-tight gradient-text mr-2">
            Workers
          </h1>
          {(isLoading || isFetching) && (
            <Loader2 className="size-6 animate-spin text-(--brand)" />
          )}
        </div>
        <WorkerGrids devices={data.devices} global_stats={data.global_stats} />
      </div>
      <div>
        <WorkersTable devices={data.devices} />
      </div>
    </div>
  );
};
