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

/**
 * Request durations are routinely sub-millisecond, and rounding those to an
 * integer printed 0 across every window, so a healthy deployment was
 * indistinguishable from a metric that had stopped collecting. Small values
 * keep two decimals; anything under a hundredth of a millisecond says so
 * rather than collapsing to zero.
 */
const formatDurationMs = (value: number): string => {
  if (!Number.isFinite(value) || value <= 0) return "0";
  if (value < 0.01) return "<0.01";
  if (value < 10) return value.toFixed(2);
  return String(Math.round(value));
};

const WINDOWS: {
  label: string;
  rate: keyof TimeWindowedStats;
  duration: keyof TimeWindowedStats;
}[] = [
  {
    label: "30 seconds",
    rate: "requests_rate_over_30_seconds",
    duration: "request_ms_avg_over_30_seconds",
  },
  {
    label: "1 minute",
    rate: "requests_rate_over_1_min",
    duration: "request_ms_avg_over_1_min",
  },
  {
    label: "5 minutes",
    rate: "requests_rate_over_5_min",
    duration: "request_ms_avg_over_5_min",
  },
  {
    label: "15 minutes",
    rate: "requests_rate_over_15_min",
    duration: "request_ms_avg_over_15_min",
  },
];

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
          {WINDOWS.map((window) => (
            <TableRow key={window.label}>
              <TableCell className="font-medium">{window.label}</TableCell>
              <TableCell className="text-right tabular-nums">
                {formatRate(stats[window.rate])}
              </TableCell>
              <TableCell className="text-right tabular-nums">
                {formatDurationMs(stats[window.duration])}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  </div>
);

export const TimeWindowStats = memo(TimeWindowStatsComponent);
