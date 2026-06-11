import { ChevronDown, ChevronUp } from "lucide-react";
import type { FC, ReactNode } from "react";
import { cn } from "../lib/utils";
import type { SortOrder } from "./table-sorting";

interface SortHeaderProps {
  field: string;
  sortBy: string;
  sortOrder: SortOrder;
  onSort: (field: string) => void;
  children: ReactNode;
  align?: "left" | "center" | "right";
}

export const SortHeader: FC<SortHeaderProps> = ({
  field,
  sortBy,
  sortOrder,
  onSort,
  children,
  align = "left",
}) => (
  <button
    type="button"
    onClick={() => onSort(field)}
    className={cn(
      "inline-flex items-center gap-1 hover:text-foreground transition-colors",
      align === "center" && "justify-center w-full",
      align === "right" && "justify-end w-full",
    )}
  >
    <span>{children}</span>
    {sortBy === field &&
      (sortOrder === "asc" ? (
        <ChevronUp className="size-3" />
      ) : (
        <ChevronDown className="size-3" />
      ))}
  </button>
);
