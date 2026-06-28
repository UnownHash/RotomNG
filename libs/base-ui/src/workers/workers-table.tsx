import { CheckCircle2, ChevronDown, ChevronRight, XCircle } from "lucide-react";
import { motion } from "motion/react";
import React, {
  memo,
  useCallback,
  useDeferredValue,
  useMemo,
  useState,
} from "react";
import { Collapse } from "../anim/collapse";
import { CustomTablePagination } from "../components/custom-table-pagination";
import { Button } from "../components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "../components/ui/card";
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
  TABLE_BODY_ROW,
  TABLE_HEADER_ROW,
  TABLE_WRAPPER,
} from "../lib/aesthetic";
import { cn } from "../lib/utils";
import { Search } from "../search";
import {
  compareWorkerItems,
  getNextSortState,
  SortHeader,
  type SortOrder,
} from "../sorting";
import { RelativeTimeLabel } from "../time-label";
import type { Device, Worker } from "../types";
import { useDebounce } from "../utils";
import { WorkerDetails } from "./worker-details";

interface WorkersTableProps {
  devices: Device[];
}

const WorkersTableComponent: React.FC<{
  devices: Device[];
  search: string;
}> = ({ devices, search }) => {
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
  } = useTablePagination<Worker & { searchString: string }>({
    tableKey: "workers",
  });

  // Use both debounce and deferred value for optimal performance
  const debouncedSearch = useDebounce(search, 300);
  const deferredSearch = useDeferredValue(debouncedSearch);

  // Extract all workers from all devices with pre-computed search strings
  const allWorkers = useMemo(() => {
    const workers: (Worker & { searchString: string })[] = [];
    devices.forEach((device) => {
      if (device.workers) {
        device.workers.forEach((worker) => {
          // Pre-compute search string to avoid repeated toLowerCase() calls
          const searchString = [
            worker.origin,
            worker.id,
            worker.device_id,
            worker.version_code,
            worker.session?.controller?.id,
            worker.is_in_use ? "active" : "inactive",
          ]
            .filter(Boolean)
            .join(" ")
            .toLowerCase();

          workers.push({ ...worker, searchString });
        });
      }
    });
    return workers;
  }, [devices]);

  const toggleRowExpansion = useCallback((workerId: string) => {
    setExpandedRows((prev) => {
      const newExpandedRows = new Set(prev);
      if (newExpandedRows.has(workerId)) {
        newExpandedRows.delete(workerId);
      } else {
        newExpandedRows.add(workerId);
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

  const filteredAndSortedWorkers = useMemo(() => {
    // Use deferred search for better performance
    const lowercaseSearch = deferredSearch.toLowerCase().trim();

    // First filter the workers based on search using pre-computed search strings
    const filteredWorkers = lowercaseSearch
      ? allWorkers.filter((worker) =>
          worker.searchString.includes(lowercaseSearch),
        )
      : allWorkers;

    // Early return if no sorting needed
    if (!sortBy) return filteredWorkers;

    // Then sort the filtered workers using the worker-specific sorting library with id as secondary sort
    return [...filteredWorkers].sort((a, b) =>
      compareWorkerItems(a, b, sortBy, sortOrder),
    );
  }, [allWorkers, sortBy, sortOrder, deferredSearch]);

  // Calculate paginated workers using the hook
  const paginatedWorkers = useMemo(() => {
    return getPaginatedItems(filteredAndSortedWorkers);
  }, [filteredAndSortedWorkers, getPaginatedItems]);

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
                  ? "Collapse workers table"
                  : "Expand workers table"
              }
            >
              {isTableExpanded ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronRight className="size-4" />
              )}
            </Button>
            <CardTitle className="text-xl">
              Workers ({allWorkers.length})
            </CardTitle>
          </div>
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
                    <TableHead className="text-left">
                      <SortHeader
                        field="id"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        Worker ID
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">
                      <SortHeader
                        field="can_be_used"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        Enabled
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
                    <TableHead className="text-left">
                      <SortHeader
                        field="weight"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        Weight
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-left">
                      <SortHeader
                        field="session.controller.id"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        Controller ID
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-left">
                      <SortHeader
                        field="version_code"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        Version
                      </SortHeader>
                    </TableHead>
                    <TableHead className="w-[200px] text-center">
                      <SortHeader
                        field="session.connected_at_ms"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        Connected At
                      </SortHeader>
                    </TableHead>
                    <TableHead className="w-[200px] text-center">
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
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {paginatedWorkers.map((worker, index) => {
                    const workerId = worker.id || `worker-${index}`;
                    const isExpanded = expandedRows.has(workerId);

                    // Status-driven row tinting: disconnected → destructive,
                    // connected-but-unusable → warning.
                    const rowStateClass = !worker.is_connected
                      ? "bg-destructive/20 hover:bg-destructive/30"
                      : worker.is_connected && !worker.can_be_used
                        ? "bg-amber-500/20 hover:bg-amber-500/30"
                        : "";

                    return (
                      <React.Fragment key={workerId}>
                        <TableRow className={cn(TABLE_BODY_ROW, rowStateClass)}>
                          <TableCell className="text-center">
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => toggleRowExpansion(workerId)}
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
                          <TableCell className="text-left">
                            {worker.origin}
                          </TableCell>
                          <TableCell className="text-left">
                            {worker.id}
                          </TableCell>
                          <TableCell className="text-center">
                            <div className="flex justify-center">
                              {worker.can_be_used ? (
                                <CheckCircle2 className="size-5 text-emerald-500" />
                              ) : (
                                <XCircle className="size-5 text-red-500" />
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="text-center">
                            <div className="flex justify-center">
                              {worker.is_in_use ? (
                                <CheckCircle2 className="size-5 text-emerald-500" />
                              ) : (
                                <XCircle className="size-5 text-red-500" />
                              )}
                            </div>
                          </TableCell>
                          <TableCell className="text-left">
                            {worker.weight || "N/A"}
                          </TableCell>
                          <TableCell className="text-left">
                            {worker.session?.controller?.id || ""}
                          </TableCell>
                          <TableCell className="text-left">
                            {worker.version_code}
                          </TableCell>
                          <TableCell className="w-[200px] text-center">
                            <RelativeTimeLabel
                              timestamp={worker.session?.connected_at_ms || 0}
                            />
                          </TableCell>
                          <TableCell className="w-[200px] text-center">
                            <RelativeTimeLabel
                              timestamp={worker.last_seen_at_ms}
                            />
                          </TableCell>
                        </TableRow>

                        {/* Expanded worker details row */}
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
                                <WorkerDetails worker={worker} />
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
              count={filteredAndSortedWorkers.length}
              page={page}
              rowsPerPage={rowsPerPage}
              onPageChange={handleChangePage}
              onRowsPerPageChange={handleChangeRowsPerPage}
            />
          </CardContent>
        </Collapse>
      </Card>
    </motion.div>
  );
};

const WorkersTable = ({ devices }: WorkersTableProps) => {
  const [search, setSearch] = useState("");

  const filteredItems = useMemo(() => {
    const lowercaseSearch = search.toLowerCase();
    return devices.filter(
      (device) =>
        !lowercaseSearch ||
        device.origin?.toLowerCase().includes(lowercaseSearch) ||
        device.id?.toLowerCase().includes(lowercaseSearch) ||
        device.version.toString().toLowerCase().includes(lowercaseSearch) ||
        device.workers?.some((worker) =>
          [
            worker.origin,
            worker.id,
            worker.device_id,
            worker.version_code?.toString(),
            worker.session?.controller?.id,
          ].some((field) => field?.toLowerCase().includes(lowercaseSearch)),
        ),
    );
  }, [search, devices]);

  return (
    <div className="h-full flex flex-col overflow-hidden">
      <div className="shrink-0 mb-2">
        <Search
          value={search}
          onChange={setSearch}
          placeholder="Search workers..."
        />
      </div>
      <div className="flex-1 min-h-0 overflow-hidden">
        <WorkersTableComponent devices={filteredItems} search={search} />
      </div>
    </div>
  );
};

// Export memoized component
export const MemoizedWorkersTable = memo(WorkersTable);
export { WorkersTable };
