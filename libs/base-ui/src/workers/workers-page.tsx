import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { statusQuery } from "../lib/query-options";

import { WorkerGrids } from "./worker-grids";
import { MemoizedWorkersTable as WorkersTable } from "./workers-table";

export const WorkersPage = () => {
  const { isLoading, isFetching, error, data, isSuccess } = useQuery({
    ...statusQuery(),
    select: (s) => s.devices,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center min-h-[200px]">
        <Loader2 className="size-8 animate-spin text-(--brand)" />
      </div>
    );
  }

  if (error || !isSuccess) {
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
      <div>
        <div className="flex items-center mb-4">
          <h1 className="text-3xl font-bold tracking-tight gradient-text mr-2">
            Workers
          </h1>
          {(isLoading || isFetching) && (
            <Loader2 className="size-6 animate-spin text-(--brand)" />
          )}
        </div>
        <WorkerGrids devices={data} />
      </div>
      <div>
        <WorkersTable devices={data} />
      </div>
    </div>
  );
};
