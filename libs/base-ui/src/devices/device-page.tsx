import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { useCallback, useState } from "react";
import { toast } from "react-toastify";
import { ConfirmationDialog } from "../components/confirmation-dialog";
import { apiFetch } from "../lib/api";
import { statusQuery } from "../lib/query-options";
import type { Device, Status } from "../types";
import { DeviceGrids } from "./device-grids";
import { MemoizedDevicesTable as DevicesTable } from "./devices-table";

/** Shape of the reply from PUT /api/device/_/action/delete. */
interface RemoveDeadResponse {
  devices_count?: number;
}

const REMOVE_DEAD_TIMEOUT_MS = 5000;

export const DevicePage = () => {
  const queryClient = useQueryClient();
  const [confirmRemoveDead, setConfirmRemoveDead] = useState(false);
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

  // The button used to fire silently: no confirmation before deleting, and no
  // report afterwards. The reply carries the number of entries removed, which
  // was parsed and thrown away, so a working call and a failing one looked
  // exactly alike -- nothing happened on screen either way.
  const removeDeadMutation = useMutation({
    mutationFn: async (): Promise<RemoveDeadResponse> => {
      const cancel = new AbortController();
      const timer = setTimeout(() => cancel.abort(), REMOVE_DEAD_TIMEOUT_MS);
      try {
        const res = await apiFetch("/api/device/_/action/delete", {
          method: "PUT",
          signal: cancel.signal,
        });
        if (!res.ok) {
          throw new Error(
            `Remove dead failed: ${res.status} ${res.statusText}`,
          );
        }
        return (await res.json()) as RemoveDeadResponse;
      } finally {
        clearTimeout(timer);
      }
    },
    onSuccess: (result) => {
      const removed = result.devices_count ?? 0;
      if (removed === 0) {
        toast.info("No dead devices to remove");
        return;
      }
      toast.success(
        `Removed ${removed} dead device${removed === 1 ? "" : "s"}`,
      );
    },
    onError: (error: Error) => {
      toast.error(error.message || "Failed to remove dead devices");
    },
    onSettled: () => queryClient.invalidateQueries(statusQuery()),
  });

  const handleRemoveDead = useCallback(() => {
    setConfirmRemoveDead(true);
  }, []);

  const confirmRemoveDeadDevices = useCallback(() => {
    setConfirmRemoveDead(false);
    removeDeadMutation.mutate();
  }, [removeDeadMutation]);

  const cancelRemoveDead = useCallback(() => setConfirmRemoveDead(false), []);

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
      <ConfirmationDialog
        open={confirmRemoveDead}
        title="Remove dead devices"
        message="Deletes the entry for every device that is not currently connected. Devices that reconnect will register again."
        confirmText="Remove"
        cancelText="Cancel"
        destructive
        onConfirm={confirmRemoveDeadDevices}
        onCancel={cancelRemoveDead}
      />
    </div>
  );
};
