import { Loader2 } from "lucide-react";
import { motion } from "motion/react";
import { useCallback, useMemo, useState } from "react";
import { MagneticButton } from "../anim/magnetic-button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import {
  TABLE_BODY_ROW,
  TABLE_HEADER_ROW,
  TABLE_WRAPPER,
} from "../lib/aesthetic";
import {
  createTableSorter,
  getNextSortState,
  SortHeader,
  type SortOrder,
} from "../sorting";
import type { Device, Jobs } from "../types";

import { ExecuteJobModal } from "./execute-job-modal";

interface JobsTableProps {
  devices?: Device[];
  isLoading: boolean;
  jobs: Jobs;
  refetchDevices: () => void;
  refreshJobInstances: () => void;
}

export const JobsTable = ({
  devices,
  isLoading,
  jobs,
  refetchDevices,
  refreshJobInstances,
}: JobsTableProps) => {
  const [jobId, setJobId] = useState<string>("");
  const [modalOpen, setModalOpen] = useState(false);
  const [sortBy, setSortBy] = useState<string>("id");
  const [sortOrder, setSortOrder] = useState<SortOrder>("asc");

  const handleSort = useCallback(
    (field: string) => {
      const nextState = getNextSortState(sortBy, sortOrder, field);
      setSortBy(nextState.sortBy);
      setSortOrder(nextState.sortOrder);
    },
    [sortBy, sortOrder],
  );

  // Memoize the sorting function to avoid recreation
  const sortFunction = useMemo(() => {
    return createTableSorter(sortBy, sortOrder);
  }, [sortBy, sortOrder]);

  // Sort jobs by the selected field
  const sortedJobs = useMemo(() => {
    return [...jobs].sort(sortFunction);
  }, [jobs, sortFunction]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8">
        <Loader2 className="size-6 animate-spin text-(--brand)" />
      </div>
    );
  }

  return (
    <>
      <motion.div
        initial={{ opacity: 0, y: 12 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.35, ease: [0.16, 1, 0.3, 1] }}
        className={TABLE_WRAPPER}
      >
        <Table>
          <TableHeader>
            <TableRow className={TABLE_HEADER_ROW}>
              <TableHead className="text-left">
                <SortHeader
                  field="id"
                  sortBy={sortBy}
                  sortOrder={sortOrder}
                  onSort={handleSort}
                >
                  Name
                </SortHeader>
              </TableHead>
              <TableHead className="text-left">Description</TableHead>
              <TableHead className="text-left">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sortedJobs.map((job) => (
              <TableRow key={job.id} className={TABLE_BODY_ROW}>
                <TableCell className="text-left">{job.id}</TableCell>
                <TableCell className="text-left">{job.description}</TableCell>
                <TableCell className="text-left">
                  <MagneticButton
                    size="sm"
                    onClick={() => {
                      refetchDevices();
                      setJobId(job.id);
                      setModalOpen(true);
                    }}
                  >
                    Run
                  </MagneticButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </motion.div>
      <ExecuteJobModal
        open={modalOpen}
        onOpenChange={setModalOpen}
        devices={devices}
        jobId={jobId}
        onSuccess={refreshJobInstances}
      />
    </>
  );
};
