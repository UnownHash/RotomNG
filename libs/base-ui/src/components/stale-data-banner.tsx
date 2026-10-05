import { AlertTriangle } from "lucide-react";
import type { FC } from "react";

export interface StaleDataBannerProps {
  /** The failure from the most recent poll. Nothing renders without one. */
  error: Error | null;
}

/**
 * Shown when a poll fails but earlier data is still on screen.
 *
 * The pages used to replace everything with a full-page error the moment a
 * single refresh failed, which threw away the table, the reader's page, and
 * whatever they had typed into the search box. A dashboard that polls every
 * few seconds will fail a poll occasionally, and losing the whole view over a
 * transient blip is worse than showing slightly old numbers.
 *
 * The rule is now: no data at all means the full-page error, and data plus a
 * failure means this, so the reader can tell the numbers have stopped moving
 * without being thrown out of what they were doing.
 */
export const StaleDataBanner: FC<StaleDataBannerProps> = ({ error }) => {
  if (!error) return null;

  return (
    <div
      role="status"
      className="mb-4 flex items-start gap-2 rounded-md border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-sm text-amber-200"
    >
      <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
      <span>
        Showing the last successful update. The most recent refresh failed
        {error.message ? `: ${error.message}` : "."}
      </span>
    </div>
  );
};
