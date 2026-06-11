import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { useCallback } from "react";
import type { Device, Status } from "../types";
import { statusQuery } from "../lib/query-options";
import { DeviceGrids } from "./device-grids";
import { MemoizedDevicesTable as DevicesTable } from "./devices-table";

export const DevicePage = () => {
  const queryClient = useQueryClient();
  const { isLoading, isFetching, error, data, isSuccess } = useQuery(
    statusQuery(),
  );

  const handleDeviceUpdate = useCallback(
    (updatedDevice: Device) => {
      queryClient.setQueryData<Status>(["status"], (oldData) => {
        if (!oldData) return oldData;

        return {
          ...oldData,
          devices: oldData.devices.map((device) => {
            if (device.id === updatedDevice.id) {
              // An updated device returned from an API call like
              // enabling/disabling a device typically does not
              // include its workers, so we need to copy them.
              // Currently the only thing that can change is the
              // 'can_be_used'.. which needs copied to the workers.
              if (updatedDevice.workers === undefined) {
                updatedDevice.workers = device.workers;
                if (updatedDevice.workers) {
                  updatedDevice.workers.forEach((worker) => {
                    worker.can_be_used = updatedDevice.can_be_used;
                  });
                }
              }
              return updatedDevice;
            }
            return device;
          }),
        };
      });
    },
    [queryClient],
  );

  const removeDeadMutation = useMutation({
    mutationFn: async () => {
      const cancel = new AbortController();
      const timer = setTimeout(() => cancel.abort(), 5000);
      try {
        const res = await fetch("/api/device/_/action/delete", {
          method: "PUT",
          signal: cancel.signal,
        });
        if (!res.ok) throw new Error(`Remove dead failed: ${res.status}`);
        return res.json();
      } finally {
        clearTimeout(timer);
      }
    },
    onSettled: () => queryClient.invalidateQueries(statusQuery()),
  });

  const handleRemoveDead = useCallback(() => {
    removeDeadMutation.mutate();
  }, [removeDeadMutation]);

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
            Devices
          </h1>
          {(isLoading || isFetching) && (
            <Loader2 className="size-6 animate-spin text-(--brand)" />
          )}
        </div>
        <DeviceGrids {...data} />
      </div>
      <div>
        <DevicesTable
          devices={data.devices}
          onDeviceUpdate={handleDeviceUpdate}
          onRemoveDead={handleRemoveDead}
        />
      </div>
    </div>
  );
};
