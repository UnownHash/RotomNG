import { motion } from "motion/react";
import { StatCard, type StatCardAccent } from "../anim/stat-card";
import { AESTHETIC_CARD, AESTHETIC_CARD_HEADER } from "../lib/aesthetic";
import { cn } from "../lib/utils";
import { Card, CardHeader, CardTitle } from "./ui/card";

interface StatusGridProps {
  title: string;
  headers: string[];
  values: (number | string)[];
  className?: string;
  /** Tints each child StatCard's left border + gradient number. */
  accent?: StatCardAccent;
}

export const StatusGrid = ({
  title,
  headers,
  values,
  className,
  accent,
}: StatusGridProps) => (
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
      <div className="grid grid-cols-2 md:grid-cols-4 gap-6 p-4">
        {headers.map((header, i) => {
          const value = values[i];
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
    </Card>
  </motion.div>
);
