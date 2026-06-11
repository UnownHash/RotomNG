import type React from "react";
import { memo } from "react";
import { TABLE_HEADER_ROW, TABLE_WRAPPER } from "../lib/aesthetic";
import type { TimeWindowedStats } from "../types";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";

interface TimeWindowStatsProps {
  stats: TimeWindowedStats;
  title?: string;
}

const formatRate = (value: number): string =>
  value > 0 && value < 1 ? value.toFixed(2) : String(Math.round(value));

const TimeWindowStatsComponent: React.FC<TimeWindowStatsProps> = ({
  stats,
  title = "Time Window Statistics",
}) => (
  <div>
    <p className="text-sm font-medium text-muted-foreground mb-2">{title}</p>
    <div className={TABLE_WRAPPER}>
      <Table>
        <TableHeader>
          <TableRow className={TABLE_HEADER_ROW}>
            <TableHead className="text-xs uppercase tracking-wider">
              Time Period
            </TableHead>
            <TableHead className="text-xs uppercase tracking-wider text-right">
              Requests/s
            </TableHead>
            <TableHead className="text-xs uppercase tracking-wider text-right">
              Avg Request Duration (ms)
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow>
            <TableCell className="font-medium">30 seconds</TableCell>
            <TableCell className="text-right tabular-nums">
              {formatRate(stats.requests_rate_over_30_seconds)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {Math.round(stats.request_ms_avg_over_30_seconds)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell className="font-medium">1 minute</TableCell>
            <TableCell className="text-right tabular-nums">
              {formatRate(stats.requests_rate_over_1_min)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {Math.round(stats.request_ms_avg_over_1_min)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell className="font-medium">5 minutes</TableCell>
            <TableCell className="text-right tabular-nums">
              {formatRate(stats.requests_rate_over_5_min)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {Math.round(stats.request_ms_avg_over_5_min)}
            </TableCell>
          </TableRow>
          <TableRow>
            <TableCell className="font-medium">15 minutes</TableCell>
            <TableCell className="text-right tabular-nums">
              {formatRate(stats.requests_rate_over_15_min)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {Math.round(stats.request_ms_avg_over_15_min)}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
);

export const TimeWindowStats = memo(TimeWindowStatsComponent);
