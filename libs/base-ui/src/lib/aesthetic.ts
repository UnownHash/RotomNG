/**
 * Shared className strings — restrained Linear/Vercel-style aesthetic.
 *
 * Single brand accent (violet) on hover/active states only. No rainbow
 * gradients, no glows, no aurora. Subtle gradient hints in card headers.
 * Compose with `cn()` for per-call overrides.
 */

/** Card wrap: solid card background, defined border, overflow clip. */
export const AESTHETIC_CARD = "bg-card border border-border/60 overflow-hidden";

/**
 * CardHeader: faint brand-tinted gradient sweep (low opacity, single hue),
 * bottom border, symmetric vertical padding.
 */
export const AESTHETIC_CARD_HEADER =
  "border-b border-border/60 bg-linear-to-r from-(--brand)/8 to-transparent flex flex-row items-center justify-between py-3!";

/** Table header row: muted band that doesn't react to hover. */
export const TABLE_HEADER_ROW =
  "bg-muted/40 hover:bg-muted/40 border-border/60";

/** Table body row: subtle brand-tinted hover. */
export const TABLE_BODY_ROW =
  "border-border/60 hover:bg-(--brand)/8 transition-colors";

/** Wrapper around a bare Table: rounded edges, defined border, overflow clip. */
export const TABLE_WRAPPER =
  "rounded-lg border border-border/60 overflow-hidden";

/**
 * "Danger" CTA — shadcn destructive variant baseline. Apply via
 * `variant="destructive"` on Button; this constant exists so callers that
 * compose a custom danger class stay one-line.
 */
export const AESTHETIC_DANGER_BUTTON =
  "bg-destructive/70 text-destructive-foreground hover:bg-destructive transition-colors";
