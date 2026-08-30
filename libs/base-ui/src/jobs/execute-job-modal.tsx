import type React from "react";
import { useCallback, useMemo, useState } from "react";
import { toast } from "react-toastify";
import { MagneticButton } from "../anim/magnetic-button";
import { Checkbox } from "../components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "../components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import { TABLE_BODY_ROW, TABLE_HEADER_ROW } from "../lib/aesthetic";
import { apiFetch } from "../lib/api";
import { Search } from "../search";
import { compareAlphanumeric } from "../sorting";
import type { Device } from "../types";

interface ExecuteJobModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  devices?: Device[];
  jobId: string;
  onSuccess?: () => void;
}

export const ExecuteJobModal: React.FC<ExecuteJobModalProps> = ({
  open,
  onOpenChange,
  devices,
  jobId,
  onSuccess,
}) => {
  const [selectedDevices, setSelectedDevices] = useState<Set<string>>(
    new Set(),
  );
  const [search, setSearch] = useState("");

  const handleCloseModal = useCallback(() => {
    onOpenChange(false);
  }, [onOpenChange]);

  const filteredDevices = useMemo(() => {
    const filtered =
      devices?.filter(
        (device) =>
          device.is_connected &&
          (device.id?.toLowerCase().includes(search.toLowerCase()) ||
            device.origin?.toLowerCase().includes(search.toLowerCase())),
      ) || [];

    // Sort by origin using the same alphanumeric sorting as the devices table
    return filtered.sort((a, b) => {
      const originA = a.origin || "";
      const originB = b.origin || "";
      return compareAlphanumeric(originA.toLowerCase(), originB.toLowerCase());
    });
  }, [devices, search]);

  const executeJob = useCallback(
    async ({ deviceIds }: { deviceIds: string[] | number[] }) => {
      const promise = apiFetch(`/api/job/${jobId}/run`, {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ device_ids: deviceIds }),
      }).then(async (response) => {
        if (response.status !== 200) {
          const errorData = await response.json().catch(() => ({}));
          throw new Error(errorData.error || "Failed to execute job");
        }
        handleCloseModal();
        onSuccess?.();
      });

      toast.promise(promise, {
        pending: `Running ${jobId}...`,
        success: `${jobId} started successfully`,
        error: `Failed to start job ${jobId}`,
      });
    },
    [jobId, handleCloseModal, onSuccess],
  );

  const handleSelectAll = useCallback(() => {
    if (selectedDevices.size === filteredDevices.length) {
      setSelectedDevices(new Set());
    } else {
      setSelectedDevices(
        new Set(
          filteredDevices.flatMap((device) => (device.id ? [device.id] : [])),
        ),
      );
    }
  }, [filteredDevices, selectedDevices.size]);

  const handleSelectDevice = useCallback((deviceId: string) => {
    setSelectedDevices((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(deviceId)) {
        newSet.delete(deviceId);
      } else {
        newSet.add(deviceId);
      }
      return newSet;
    });
  }, []);

  const isAllSelected =
    selectedDevices.size === filteredDevices.length &&
    filteredDevices.length > 0;
  const isIndeterminate =
    selectedDevices.size > 0 && selectedDevices.size < filteredDevices.length;

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        // Reset selection + search when closing
        setSelectedDevices(new Set());
        setSearch("");
      }
      onOpenChange(nextOpen);
    },
    [onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-3xl max-h-[90vh] flex flex-col bg-card/95 backdrop-blur-xl border-border/40">
        <DialogHeader>
          <DialogTitle className="gradient-text text-2xl">
            Execute {jobId}
          </DialogTitle>
        </DialogHeader>

        <div className="flex-1 overflow-hidden flex flex-col gap-3 min-h-0">
          <Search
            value={search}
            onChange={setSearch}
            placeholder="Search devices..."
          />

          <div className="rounded-lg border border-border/40 overflow-auto flex-1 min-h-0">
            <Table>
              <TableHeader className="sticky top-0 z-10 bg-card">
                <TableRow className={TABLE_HEADER_ROW}>
                  <TableHead className="w-[50px] text-center">
                    <div className="flex justify-center">
                      <Checkbox
                        checked={
                          isIndeterminate ? "indeterminate" : isAllSelected
                        }
                        onCheckedChange={handleSelectAll}
                        aria-label="Select all devices"
                      />
                    </div>
                  </TableHead>
                  <TableHead className="text-left">Origin</TableHead>
                  <TableHead className="text-left">ID</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredDevices.map((device) => (
                  <TableRow key={`${device.id}`} className={TABLE_BODY_ROW}>
                    <TableCell className="w-[50px] text-center">
                      <div className="flex justify-center">
                        <Checkbox
                          checked={selectedDevices.has(device.id || "")}
                          onCheckedChange={() =>
                            device.id && handleSelectDevice(device.id)
                          }
                          aria-label={`Select device ${device.id || ""}`}
                        />
                      </div>
                    </TableCell>
                    <TableCell className="text-left">{device.origin}</TableCell>
                    <TableCell className="text-left">{device.id}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </div>

        <div className="flex justify-end pt-2 border-t border-border/40">
          <MagneticButton
            disabled={selectedDevices.size === 0 || !devices}
            onClick={() => {
              if (selectedDevices.size === 0 || !devices) {
                return;
              }

              executeJob({
                deviceIds: Array.from(selectedDevices) as string[],
              });
            }}
          >
            Run ({selectedDevices.size} selected)
          </MagneticButton>
        </div>
      </DialogContent>
    </Dialog>
  );
};
