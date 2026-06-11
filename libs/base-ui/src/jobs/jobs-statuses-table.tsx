import { Loader2 } from "lucide-react";
import { motion } from "motion/react";
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
import { cn } from "../lib/utils";
import type { JobInstances, Jobs } from "../types";

interface JobsStatusesTableProps {
  isLoading: boolean;
  jobs?: Jobs;
  jobInstances: JobInstances;
}

const formatTimestamp = (timestampMs: number): string => {
  if (!timestampMs) return "-";
  return new Date(timestampMs).toLocaleString();
};

const getStatusClass = (status: string): string => {
  switch (status.toLowerCase()) {
    case "success":
      return "text-emerald-500";
    case "failed":
      return "text-red-500";
    case "started":
      return "text-amber-500";
    default:
      return "text-foreground";
  }
};

export const JobsStatusesTable = ({
  isLoading,
  jobInstances,
}: JobsStatusesTableProps) => {
  if (isLoading) {
    return (
      <div className="flex items-center justify-center p-8">
        <Loader2 className="size-6 animate-spin text-(--brand)" />
      </div>
    );
  }

  const sortedInstances = [...jobInstances].sort(
    (a, b) => (b.id || 0) - (a.id || 0),
  );

  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.35, ease: [0.16, 1, 0.3, 1] }}
      className={TABLE_WRAPPER}
    >
      <Table>
        <TableHeader>
          <TableRow className={TABLE_HEADER_ROW}>
            <TableHead className="text-left">ID</TableHead>
            <TableHead className="text-left">Job ID</TableHead>
            <TableHead className="text-left">Device ID</TableHead>
            <TableHead className="text-left">Device Origin</TableHead>
            <TableHead className="text-left">Started At</TableHead>
            <TableHead className="text-left">Finished At</TableHead>
            <TableHead className="text-left">Status</TableHead>
            <TableHead className="text-left">Result</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sortedInstances.map((jobInstance) => (
            <TableRow key={jobInstance.id} className={TABLE_BODY_ROW}>
              <TableCell className="text-left">{jobInstance.id}</TableCell>
              <TableCell className="text-left">{jobInstance.job_id}</TableCell>
              <TableCell className="text-left">
                {jobInstance.device_id}
              </TableCell>
              <TableCell className="text-left">
                {jobInstance.device_origin ?? "-"}
              </TableCell>
              <TableCell className="text-left whitespace-nowrap">
                {formatTimestamp(jobInstance.started_at_ms)}
              </TableCell>
              <TableCell className="text-left whitespace-nowrap">
                {formatTimestamp(jobInstance.finished_at_ms || 0)}
              </TableCell>
              <TableCell className="text-left">
                <span
                  className={cn(
                    "font-medium",
                    getStatusClass(jobInstance.status),
                  )}
                >
                  {jobInstance.status}
                </span>
              </TableCell>
              <TableCell
                className="text-left max-w-[200px] overflow-hidden text-ellipsis whitespace-nowrap"
                title={jobInstance.result}
              >
                {jobInstance.result || "-"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </motion.div>
  );
};
