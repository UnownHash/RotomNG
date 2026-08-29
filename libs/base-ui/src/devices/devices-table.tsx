import { CheckCircle2, ChevronDown, ChevronRight, XCircle } from "lucide-react";
import { motion } from "motion/react";
import React, { memo, useCallback, useMemo, useState } from "react";
import { toast } from "react-toastify";
import { Collapse } from "../anim/collapse";
import { MagneticButton } from "../anim/magnetic-button";
import { CustomTablePagination } from "../components/custom-table-pagination";
import { Button } from "../components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "../components/ui/card";
import { Checkbox } from "../components/ui/checkbox";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import { useTablePagination } from "../hooks/use-table-pagination";
import {
  AESTHETIC_CARD,
  AESTHETIC_CARD_HEADER,
  AESTHETIC_DANGER_BUTTON,
  TABLE_BODY_ROW,
  TABLE_HEADER_ROW,
  TABLE_WRAPPER,
} from "../lib/aesthetic";
import { apiFetch } from "../lib/api";
import { cn } from "../lib/utils";
import { Search } from "../search";
import {
  createDeviceSorter,
  getNextSortState,
  SortHeader,
  type SortOrder,
} from "../sorting";
import { RelativeTimeLabel } from "../time-label";
import type { Device } from "../types";
import { DeviceActions } from "./device-actions";
import { DeviceDetails } from "./device-details";

interface DevicesTableComponentProps {
  devices: Device[];
  onDeviceAction: (
    deviceId: string,
    action: "reboot" | "restart" | "logcat" | "delete" | "disconnect",
  ) => void;
  onToggleDeviceEnabled: (deviceId: string, currentEnabled: boolean) => void;
  onRemoveDead?: () => void;
}

const DevicesTableComponent: React.FC<DevicesTableComponentProps> = ({
  devices,
  onDeviceAction,
  onToggleDeviceEnabled,
  onRemoveDead,
}) => {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [sortBy, setSortBy] = useState<string>("origin");
  const [sortOrder, setSortOrder] = useState<SortOrder>("asc");
  const [isTableExpanded, setIsTableExpanded] = useState(true);

  // Use the custom pagination hook with persistent storage
  const {
    page,
    rowsPerPage,
    handleChangePage,
    handleChangeRowsPerPage,
    getPaginatedItems,
  } = useTablePagination<Device>({
    tableKey: "devices",
  });

  const toggleRowExpansion = useCallback((deviceId: string) => {
    setExpandedRows((prev) => {
      const newExpandedRows = new Set(prev);
      if (newExpandedRows.has(deviceId)) {
        newExpandedRows.delete(deviceId);
      } else {
        newExpandedRows.add(deviceId);
      }
      return newExpandedRows;
    });
  }, []);

  const handleSort = useCallback(
    (field: string) => {
      const nextState = getNextSortState(sortBy, sortOrder, field);
      setSortBy(nextState.sortBy);
      setSortOrder(nextState.sortOrder);
    },
    [sortBy, sortOrder],
  );

  // Memoize the sorting function to avoid recreation using the device-specific sorting library
  const sortFunction = useMemo(() => {
    return createDeviceSorter<Device>(sortBy, sortOrder);
  }, [sortBy, sortOrder]);

  // Only recalculate sorted devices when devices array or sort parameters change
  const sortedDevices = useMemo(() => {
    return [...devices].sort(sortFunction);
  }, [devices, sortFunction]);

  // Calculate paginated devices using the hook
  const paginatedDevices = useMemo(() => {
    return getPaginatedItems(sortedDevices);
  }, [sortedDevices, getPaginatedItems]);

  // Enhanced pagination handler with scroll behavior
  const handleChangePageWithScroll = useCallback(
    (event: unknown, newPage: number) => {
      const currentPage = page;
      const totalPages = Math.ceil(sortedDevices.length / rowsPerPage);
      const currentPageRowCount = paginatedDevices.length;

      // Check if we're navigating back from a partially filled last page
      const isNavigatingBack = newPage < currentPage;
      const wasOnLastPage = currentPage === totalPages - 1;
      const wasPartiallyFilled =
        currentPageRowCount < rowsPerPage && currentPageRowCount > 0;

      handleChangePage(event, newPage);

      // If navigating back from a partially filled last page, scroll to bottom
      // to keep the user's mouse over the back button
      if (isNavigatingBack && wasOnLastPage && wasPartiallyFilled) {
        // Use setTimeout to ensure the page change has been processed
        setTimeout(() => {
          window.scrollTo({
            top: document.documentElement.scrollHeight,
            behavior: "smooth",
          });
        }, 0);
      }
    },
    [
      page,
      sortedDevices.length,
      rowsPerPage,
      paginatedDevices.length,
      handleChangePage,
    ],
  );

  // Memoize toggle handler to avoid recreation on every render
  const handleToggleDeviceEnabled = useCallback(
    (deviceId: string, currentEnabled: boolean) => {
      onToggleDeviceEnabled(deviceId, currentEnabled);
    },
    [onToggleDeviceEnabled],
  );

  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.45, ease: [0.16, 1, 0.3, 1] }}
    >
      <Card className={AESTHETIC_CARD}>
        <CardHeader className={AESTHETIC_CARD_HEADER}>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => setIsTableExpanded(!isTableExpanded)}
              aria-label={
                isTableExpanded
                  ? "Collapse devices table"
                  : "Expand devices table"
              }
            >
              {isTableExpanded ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronRight className="size-4" />
              )}
            </Button>
            <CardTitle className="text-xl">
              Devices ({devices.length})
            </CardTitle>
          </div>
          {onRemoveDead && (
            <MagneticButton
              size="sm"
              onClick={onRemoveDead}
              className={AESTHETIC_DANGER_BUTTON}
            >
              Remove Dead
            </MagneticButton>
          )}
        </CardHeader>
        <Collapse open={isTableExpanded}>
          <CardContent className="p-4">
            <div className={TABLE_WRAPPER}>
              <Table>
                <TableHeader>
                  <TableRow className={TABLE_HEADER_ROW}>
                    <TableHead className="w-[50px] text-center" />
                    <TableHead className="text-left">
                      <SortHeader
                        field="origin"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        Origin
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">
                      <SortHeader
                        field="is_connected"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        Connected
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">
                      <SortHeader
                        field="is_in_use"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        In Use
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-left">Workers</TableHead>
                    <TableHead className="text-left">Weight</TableHead>
                    <TableHead className="text-left whitespace-nowrap">
                      <SortHeader
                        field="id"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        Device ID
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center whitespace-nowrap">
                      <SortHeader
                        field="version"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        MITM Version
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">
                      <SortHeader
                        field="last_seen_at_ms"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        Last Seen
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">
                      <SortHeader
                        field="enabled"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        Enabled
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {paginatedDevices.map((device, index) => {
                    const deviceId = device.id || `device-${index}`;
                    const isExpanded = expandedRows.has(deviceId);
                    const workersValue =
                      `${device.worker_in_use_count}/${device.worker_count} ` +
                      `(${device.worker_in_use_percent.toFixed(1)}%)`;
                    const weightValue =
                      `${device.worker_in_use_weight} ` +
                      `(${device.worker_in_use_weight_percent.toFixed(1)}%)`;

                    // Status-driven row tinting: disconnected → destructive,
                    // connected-but-unusable → warning.
                    const rowStateClass = !device.is_connected
                      ? "bg-destructive/20 hover:bg-destructive/30"
                      : !device.can_be_used
                        ? "bg-amber-500/20 hover:bg-amber-500/30"
                        : "";

                    return (
                      <React.Fragment key={deviceId}>
                        <TableRow className={cn(TABLE_BODY_ROW, rowStateClass)}>
                          <TableCell className="text-center">
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => toggleRowExpansion(deviceId)}
                              aria-label={
                                isExpanded ? "Collapse row" : "Expand row"
                              }
                            >
                              {isExpanded ? (
                                <ChevronDown className="size-4" />
                              ) : (
                                <ChevronRight className="size-4" />
                              )}
                            </Button>
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-left">
                            {device.origin}
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-center">
                            <div className="flex justify-center">
                              {device.is_connected ? (
                                <CheckCircle2 className="size-5 text-emerald-500" />
                              ) : (
                                <XCircle className="size-5 text-red-500" />
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-center">
                            <div className="flex justify-center">
                              {device.is_in_use ? (
                                <CheckCircle2 className="size-5 text-emerald-500" />
                              ) : (
                                <XCircle className="size-5 text-red-500" />
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-left">
                            <p className="text-base">{workersValue}</p>
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-left">
                            <p className="text-base">{weightValue}</p>
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-left">
                            {device.id}
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-center">
                            {device.version}
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-center">
                            <RelativeTimeLabel
                              timestamp={device.last_seen_at_ms}
                            />
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-center">
                            <div className="flex justify-center">
                              <Checkbox
                                checked={device.enabled}
                                onCheckedChange={
                                  device.id
                                    ? () =>
                                        handleToggleDeviceEnabled(
                                          device.id,
                                          device.enabled,
                                        )
                                    : undefined
                                }
                                disabled={!device.id}
                                title={
                                  device.id
                                    ? device.enabled
                                      ? "Click to disable device"
                                      : "Click to enable device"
                                    : "Cannot toggle"
                                }
                              />
                            </div>
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-center">
                            {device.id && (
                              <DeviceActions
                                deviceId={device.id}
                                isConnected={device.is_connected}
                                onAction={onDeviceAction}
                              />
                            )}
                          </TableCell>
                        </TableRow>

                        {/* Expanded memory details row */}
                        {isExpanded && (
                          <TableRow className="border-border/40 hover:bg-transparent">
                            <TableCell
                              colSpan={11}
                              className="p-0 whitespace-normal"
                            >
                              <motion.div
                                initial={{ opacity: 0, y: -8 }}
                                animate={{ opacity: 1, y: 0 }}
                                transition={{
                                  duration: 0.25,
                                  ease: [0.16, 1, 0.3, 1],
                                }}
                              >
                                <DeviceDetails device={device} />
                              </motion.div>
                            </TableCell>
                          </TableRow>
                        )}
                      </React.Fragment>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
            <CustomTablePagination
              count={sortedDevices.length}
              page={page}
              rowsPerPage={rowsPerPage}
              onPageChange={handleChangePageWithScroll}
              onRowsPerPageChange={handleChangeRowsPerPage}
            />
          </CardContent>
        </Collapse>
      </Card>
    </motion.div>
  );
};

const DevicesTable = ({
  devices,
  onDeviceUpdate,
  onRemoveDead,
}: {
  devices: Device[];
  onDeviceUpdate?: (updatedDevice: Device) => void;
  onRemoveDead?: () => void;
}) => {
  const [search, setSearch] = useState("");
  const executeAction = useCallback(
    async ({
      deviceId,
      action,
    }: {
      deviceId: string;
      action: "reboot" | "restart" | "logcat" | "delete" | "disconnect";
    }) => {
      const promise = apiFetch(`/api/device/${deviceId}/action/${action}`, {
        method: "PUT",
      }).then(async (response) => {
        if (response.status !== 200) {
          const baseMessage = {
            reboot: "Failed to reboot device",
            restart: "Failed to restart MITM",
            logcat: "Failed to download logcat, please check logs",
            delete: "Failed to remove device entry. Make sure it's not alive.",
            disconnect: "Failed to disconnect device",
          }[action];

          let errorDetails = "";
          try {
            const errorResult = await response.json();
            if (errorResult.error?.trim()) {
              errorDetails = `: ${errorResult.error}`;
            }
          } catch {
            // If we can't decode JSON, use empty error details
          }
          throw new Error(baseMessage + errorDetails);
        }

        if (action === "logcat") {
          const blob = await response.blob();
          const fileUrl = window.URL.createObjectURL(blob);
          const fileName =
            response.headers.get("Content-Disposition")?.slice(22, -1) ||
            "logcat.zip";

          const anchorElement = document.createElement("a");
          anchorElement.style.display = "none";
          document.body.appendChild(anchorElement);
          anchorElement.href = fileUrl;
          anchorElement.download = fileName;
          anchorElement.click();

          window.URL.revokeObjectURL(fileUrl);
          document.body.removeChild(anchorElement);
        }
      });

      toast.promise(promise, {
        pending: "Please wait...",
        success: {
          reboot: "Device Rebooted",
          restart: "MITM restarted",
          logcat: "Download started",
          delete: "Device entry removed",
          disconnect: "Device disconnected",
        }[action],
        error: {
          render({ data }: { data?: Error }) {
            return data?.message || "Unknown error occurred";
          },
        },
      });
    },
    [],
  );

  const toggleDeviceEnabled = useCallback(
    async (deviceId: string, currentEnabled: boolean) => {
      const action = currentEnabled ? "disable" : "enable";

      try {
        const response = await apiFetch(
          `/api/device/${deviceId}/action/${action}`,
          { method: "PUT" },
        );

        if (response.status !== 200) {
          let errorDetails = "";
          try {
            const errorResult = await response.json();
            if (errorResult.error?.trim()) {
              errorDetails = `: ${errorResult.error}`;
            }
          } catch {
            // If we can't decode JSON, use empty error details
          }
          toast.error(`Failed to ${action} device${errorDetails}`);
          return;
        }

        const result = await response.json();
        if (result.device && onDeviceUpdate) {
          onDeviceUpdate(result.device);
        }
      } catch (_error) {
        toast.error(`Failed to ${action} device: An unknown error occurred`);
      }
    },
    [onDeviceUpdate],
  );

  const filteredItems = useMemo(() => {
    const lowercaseSearch = search.toLowerCase();
    return devices.filter(
      (device) =>
        !lowercaseSearch ||
        device.origin?.toLowerCase().includes(lowercaseSearch) ||
        device.id?.toLowerCase().includes(lowercaseSearch) ||
        device.version.toString().toLowerCase().includes(lowercaseSearch),
    );
  }, [search, devices]);

  const handleDeviceAction = useCallback(
    (
      deviceId: string,
      action: "reboot" | "restart" | "logcat" | "delete" | "disconnect",
    ) => {
      executeAction({ deviceId, action });
    },
    [executeAction],
  );

  return (
    <div className="h-full flex flex-col overflow-hidden">
      <div className="shrink-0 mb-2">
        <Search value={search} onChange={setSearch} />
      </div>
      <div className="flex-1 min-h-0 overflow-hidden">
        <DevicesTableComponent
          devices={filteredItems}
          onDeviceAction={handleDeviceAction}
          onToggleDeviceEnabled={toggleDeviceEnabled}
          onRemoveDead={onRemoveDead}
        />
      </div>
    </div>
  );
};

export const MemoizedDevicesTable = memo(DevicesTable) as React.FC<{
  devices: Device[];
  onDeviceUpdate?: (updatedDevice: Device) => void;
  onRemoveDead?: () => void;
}>;
