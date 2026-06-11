import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { statusQuery } from "../lib/query-options";
import { StatusGrids } from "./status-grids";

export const StatusPage = () => {
  const { isLoading, isFetching, error, data, isSuccess } = useQuery(
    statusQuery(),
  );

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
      <div className="flex items-center mb-4">
        <h1 className="text-3xl font-bold tracking-tight gradient-text mr-2">
          Status
        </h1>
        {(isLoading || isFetching) && (
          <Loader2 className="size-6 animate-spin text-(--brand)" />
        )}
      </div>
      <StatusGrids {...data} />
    </div>
  );
};
