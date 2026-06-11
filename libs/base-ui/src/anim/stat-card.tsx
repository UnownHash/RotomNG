import NumberFlow from "@number-flow/react";
import { motion } from "motion/react";
import { type CSSProperties, useEffect, useState } from "react";
import { Card, CardContent } from "../components/ui/card";
import { cn } from "../lib/utils";

/** Named accent hues for KPI stat cards. Each maps to a `--accent-*` CSS var. */
export type StatCardAccent = "violet" | "cyan" | "emerald" | "amber";

const ACCENT_VARS: Record<StatCardAccent, string> = {
  violet: "var(--accent-violet)",
  cyan: "var(--accent-cyan)",
  emerald: "var(--accent-emerald)",
  amber: "var(--accent-amber)",
};

interface StatCardProps {
  label: string;
  value: number;
  suffix?: string;
  index?: number;
  className?: string;
  /** Per-card accent hue. Tints the left border + the NumberFlow gradient. */
  accent?: StatCardAccent;
}

export const StatCard = ({
  label,
  value,
  suffix,
  index = 0,
  className,
  accent = "violet",
}: StatCardProps) => {
  const accentVar = ACCENT_VARS[accent];
  // Drive both the border accent and the gradient-num hue from one variable
  // so theme tweaks stay centralized in globals.css.
  const cssVars = {
    "--accent-hue": accentVar,
  } as CSSProperties;

  // Initial-mount count-up: NumberFlow snaps to its `value` prop on first
  // render, so we seed with 0 and flip to the real value after mount.
  // Pairs with the staggered motion entrance (delay = index * 0.05) so each
  // card spins up in cascade rather than all at once. Subsequent value
  // updates from the parent still animate via NumberFlow's built-in
  // transition — this only affects the first paint.
  const [displayValue, setDisplayValue] = useState(0);
  useEffect(() => {
    // Defer one frame so the card's entrance motion has a chance to start
    // before the digits begin spinning. Empirically reads as more polished.
    const id = requestAnimationFrame(() => setDisplayValue(value));
    return () => cancelAnimationFrame(id);
  }, [value]);

  return (
    <motion.div
      initial={{ opacity: 0, y: 20 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{
        duration: 0.5,
        ease: [0.16, 1, 0.3, 1],
        delay: index * 0.05,
      }}
    >
      <Card
        className={cn(
          "lift bg-card border border-border/60 border-l-2",
          className,
        )}
        style={{ ...cssVars, borderLeftColor: accentVar }}
      >
        <CardContent className="p-4 flex flex-col gap-1">
          <span className="text-xs uppercase tracking-wider text-muted-foreground">
            {label}
          </span>
          <span className="text-3xl font-bold tabular-nums">
            <NumberFlow value={displayValue} style={{ color: accentVar }} />
            {suffix && (
              <span className="text-xl text-muted-foreground/70 ml-1">
                {suffix}
              </span>
            )}
          </span>
        </CardContent>
      </Card>
    </motion.div>
  );
};
