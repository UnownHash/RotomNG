import { ChevronDown, ChevronRight } from "lucide-react";
import { motion } from "motion/react";
import React, { memo, useCallback, useMemo, useState } from "react";
import { toast } from "react-toastify";
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
import { apiFetch } from "../lib/api";
import { Search } from "../search";
import {
  createControllerSorter,
  getNextSortState,
  SortHeader,
  type SortOrder,
} from "../sorting";
import { RelativeTimeLabel } from "../time-label";
import type { Controller } from "../types";
import { ControllerActions } from "./controller-actions";
import { ControllerDetails } from "./controller-details";

interface ControllersTableComponentProps {
  controllers: Controller[];
  onControllerAction: (
    controllerUuid: string,
    action: "disconnect" | "reconnect",
  ) => void;
}

const ControllersTableComponent: React.FC<ControllersTableComponentProps> = ({
  controllers,
  onControllerAction,
}) => {
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const [sortBy, setSortBy] = useState<string>("id");
  const [sortOrder, setSortOrder] = useState<SortOrder>("asc");
  const [isTableExpanded, setIsTableExpanded] = useState(true);

  // Use the custom pagination hook with persistent storage
  const {
    page,
    rowsPerPage,
    handleChangePage,
    handleChangeRowsPerPage,
    getPaginatedItems,
  } = useTablePagination<Controller>({
    tableKey: "controllers",
  });

  const toggleRowExpansion = useCallback((controllerId: string) => {
    setExpandedRows((prev) => {
      const newExpandedRows = new Set(prev);
      if (newExpandedRows.has(controllerId)) {
        newExpandedRows.delete(controllerId);
      } else {
        newExpandedRows.add(controllerId);
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

  // Memoize the sorting function to avoid recreation using the controller-specific sorting library
  const sortFunction = useMemo(() => {
    return createControllerSorter<Controller>(sortBy, sortOrder);
  }, [sortBy, sortOrder]);

  // Only recalculate sorted controllers when controllers array or sort parameters change
  const sortedControllers = useMemo(() => {
    return [...controllers].sort(sortFunction);
  }, [controllers, sortFunction]);

  // Calculate paginated controllers using the hook
  const paginatedControllers = useMemo(() => {
    return getPaginatedItems(sortedControllers);
  }, [sortedControllers, getPaginatedItems]);

  // Enhanced pagination handler with scroll behavior
  const handleChangePageWithScroll = useCallback(
    (event: unknown, newPage: number) => {
      const currentPage = page;
      const totalPages = Math.ceil(sortedControllers.length / rowsPerPage);
      const currentPageRowCount = paginatedControllers.length;

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
      sortedControllers.length,
      rowsPerPage,
      paginatedControllers.length,
      handleChangePage,
    ],
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
                  ? "Collapse controllers table"
                  : "Expand controllers table"
              }
            >
              {isTableExpanded ? (
                <ChevronDown className="size-4" />
              ) : (
                <ChevronRight className="size-4" />
              )}
            </Button>
            <CardTitle className="text-xl">
              Controllers ({controllers.length})
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
                        field="id"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        Controller ID
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-left">
                      <SortHeader
                        field="user_agent"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                      >
                        User Agent
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">
                      <SortHeader
                        field="weight"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        Weight
                      </SortHeader>
                    </TableHead>
                    <TableHead className="text-center">
                      <SortHeader
                        field="worker_id"
                        sortBy={sortBy}
                        sortOrder={sortOrder}
                        onSort={handleSort}
                        align="center"
                      >
                        Worker ID
                      </SortHeader>
                    </TableHead>
                    <TableHead className="w-[200px] text-center">
                      <SortHeader
                        field="connected_at_ms"
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
                    <TableHead className="text-center">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {paginatedControllers.map((controller, index) => {
                    const controllerId = controller.id || `controller-${index}`;
                    const isExpanded = expandedRows.has(controllerId);

                    return (
                      <React.Fragment key={controllerId}>
                        <TableRow className={TABLE_BODY_ROW}>
                          <TableCell className="text-center">
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              onClick={() => toggleRowExpansion(controllerId)}
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
                            {controller.id}
                          </TableCell>
                          <TableCell className="text-left whitespace-nowrap overflow-hidden text-ellipsis max-w-[300px]">
                            {controller.user_agent}
                          </TableCell>
                          <TableCell className="text-center">
                            {controller.weight}
                          </TableCell>
                          <TableCell className="text-center">
                            {controller.worker_id || ""}
                          </TableCell>
                          <TableCell className="w-[200px] text-center">
                            <RelativeTimeLabel
                              timestamp={controller.connected_at_ms}
                            />
                          </TableCell>
                          <TableCell className="w-[200px] text-center">
                            <RelativeTimeLabel
                              timestamp={controller.last_seen_at_ms}
                            />
                          </TableCell>
                          <TableCell className="whitespace-nowrap text-center">
                            {controller.id && controller.uuid && (
                              <ControllerActions
                                controllerId={controller.id}
                                controllerUuid={controller.uuid}
                                onAction={onControllerAction}
                              />
                            )}
                          </TableCell>
                        </TableRow>

                        {/* Expanded controller details row */}
                        {isExpanded && (
                          <TableRow className="border-border/40 hover:bg-transparent">
                            <TableCell
                              colSpan={8}
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
                                <ControllerDetails controller={controller} />
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
              count={sortedControllers.length}
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

const ControllersTable = ({ controllers }: { controllers: Controller[] }) => {
  const [search, setSearch] = useState("");

  const executeAction = useCallback(
    async ({
      controllerUuid,
      action,
    }: {
      controllerUuid: string;
      action: "disconnect" | "reconnect";
    }) => {
      const actionMessages = {
        disconnect: {
          base: "Failed to disconnect controller",
          success: "Controller disconnected",
        },
        reconnect: {
          base: "Failed to reconnect controller",
          success: "Controller set to reconnect",
        },
      };

      const promise = apiFetch(
        `/api/controller/${controllerUuid}/action/${action}`,
        { method: "PUT" },
      ).then(async (response) => {
        if (response.status !== 200) {
          const baseMessage = actionMessages[action].base;

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
      });

      toast.promise(promise, {
        pending: "Please wait...",
        success: actionMessages[action].success,
        error: {
          render({ data }: { data?: Error }) {
            return data?.message || "Unknown error occurred";
          },
        },
      });
    },
    [],
  );

  const filteredItems = useMemo(() => {
    const lowercaseSearch = search.toLowerCase();
    return controllers.filter(
      (controller) =>
        !lowercaseSearch ||
        controller.id?.toLowerCase().includes(lowercaseSearch) ||
        controller.user_agent?.toLowerCase().includes(lowercaseSearch) ||
        controller.worker_id?.toLowerCase().includes(lowercaseSearch),
    );
  }, [search, controllers]);

  const handleControllerAction = useCallback(
    (controllerUuid: string, action: "disconnect" | "reconnect") => {
      executeAction({ controllerUuid, action });
    },
    [executeAction],
  );

  return (
    <div className="h-full flex flex-col overflow-hidden">
      <div className="shrink-0 mb-2">
        <Search
          value={search}
          onChange={setSearch}
          placeholder="Search controllers..."
        />
      </div>
      <div className="flex-1 min-h-0 overflow-hidden">
        <ControllersTableComponent
          controllers={filteredItems}
          onControllerAction={handleControllerAction}
        />
      </div>
    </div>
  );
};

export const MemoizedControllersTable = memo(ControllersTable) as React.FC<{
  controllers: Controller[];
}>;
