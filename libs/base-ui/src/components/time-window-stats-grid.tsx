import { motion } from "motion/react";
import { StatCard, type StatCardAccent } from "../anim/stat-card";
import { AESTHETIC_CARD, AESTHETIC_CARD_HEADER } from "../lib/aesthetic";
import { cn } from "../lib/utils";
import { TimeWindowStats } from "./time-window-stats";
import { Card, CardHeader, CardTitle } from "./ui/card";

interface TimeWindowStatsGridProps {
  title: string;
  basicHeaders: string[];
  basicValues: (number | string)[];
  requestsPerSecond30s: number;
  requestsPerSecond1m: number;
  requestsPerSecond5m: number;
  requestsPerSecond15m: number;
  avgRequestDuration30s: number;
  avgRequestDuration1m: number;
  avgRequestDuration5m: number;
  avgRequestDuration15m: number;
  hasStats: boolean;
  hideIfNoStats?: boolean;
  className?: string;
  /** Tints each child StatCard's left border + gradient number. */
  accent?: StatCardAccent;
}

export const TimeWindowStatsGrid = ({
  title,
  basicHeaders,
  basicValues,
  requestsPerSecond30s,
  requestsPerSecond1m,
  requestsPerSecond5m,
  requestsPerSecond15m,
  avgRequestDuration30s,
  avgRequestDuration1m,
  avgRequestDuration5m,
  avgRequestDuration15m,
  hasStats,
  hideIfNoStats = false,
  className,
  accent,
}: TimeWindowStatsGridProps) => {
  const showStatsSection = hasStats || !hideIfNoStats;

  return (
    <motion.div
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.45, ease: [0.16, 1, 0.3, 1] }}
      className={cn("mb-6", className)}
    >
      <Card className={cn(AESTHETIC_CARD, "lift")}>
        <CardHeader className={cn(AESTHETIC_CARD_HEADER, "items-center")}>
          <CardTitle className="text-base">{title}</CardTitle>
        </CardHeader>
        <div className="p-4 space-y-4">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
            {basicHeaders.map((header, i) => {
              const value = basicValues[i];
              if (typeof value === "number") {
                return (
                  <StatCard
                    key={header}
                    label={header}
                    value={value}
                    index={i}
                    accent={accent}
                  />
                );
              }
              return (
                <motion.div
                  key={header}
                  initial={{ opacity: 0, y: 20 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{
                    duration: 0.5,
                    ease: [0.16, 1, 0.3, 1],
                    delay: i * 0.05,
                  }}
                  className="lift bg-card border border-border/60 rounded-xl"
                >
                  <div className="p-4 flex flex-col gap-1">
                    <span className="text-xs uppercase tracking-wider text-muted-foreground">
                      {header}
                    </span>
                    <span className="text-3xl font-bold tabular-nums">
                      {value ?? ""}
                    </span>
                  </div>
                </motion.div>
              );
            })}
          </div>
          {showStatsSection &&
            (hasStats ? (
              <TimeWindowStats
                title="Request Stats"
                stats={{
                  requests_rate_over_30_seconds: requestsPerSecond30s,
                  requests_rate_over_1_min: requestsPerSecond1m,
                  requests_rate_over_5_min: requestsPerSecond5m,
                  requests_rate_over_15_min: requestsPerSecond15m,
                  request_ms_avg_over_30_seconds: avgRequestDuration30s,
                  request_ms_avg_over_1_min: avgRequestDuration1m,
                  request_ms_avg_over_5_min: avgRequestDuration5m,
                  request_ms_avg_over_15_min: avgRequestDuration15m,
                }}
              />
            ) : (
              <p className="text-sm text-muted-foreground">
                No request stats available.
              </p>
            ))}
        </div>
      </Card>
    </motion.div>
  );
};
