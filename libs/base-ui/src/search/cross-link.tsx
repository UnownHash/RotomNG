import type { FC, ReactNode } from "react";
import { Link } from "react-router";
import { cn } from "../lib/utils";
import { searchLink } from "./use-search-param";

export interface CrossLinkProps {
  /** Route to open, e.g. "/devices". */
  to: string;
  /** Term to pre-fill that page's search with. */
  term: string;
  /** What the reader is being taken to, for the tooltip and screen readers. */
  label: string;
  children?: ReactNode;
  className?: string;
}

/**
 * A table cell that navigates to another table, pre-filtered.
 *
 * The data has no edge between a worker and its device -- worker.device_id
 * and device.id are the only thing relating them, and both were rendered as
 * inert text. Following one meant reading the id, switching tabs, and typing
 * it back in.
 */
export const CrossLink: FC<CrossLinkProps> = ({
  to,
  term,
  label,
  children,
  className,
}) => (
  <Link
    to={searchLink(to, term)}
    title={label}
    aria-label={label}
    className={cn(
      "underline decoration-dotted decoration-muted-foreground/60 underline-offset-4",
      "transition-colors hover:text-(--brand-1) hover:decoration-(--brand-1)",
      "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--brand-1)",
      className,
    )}
  >
    {children ?? term}
  </Link>
);
