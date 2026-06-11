import type React from "react";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

export interface CustomTablePaginationProps {
  count: number;
  page: number;
  rowsPerPage: number;
  onPageChange: (event: unknown, newPage: number) => void;
  onRowsPerPageChange: (event: React.ChangeEvent<HTMLInputElement>) => void;
  rowsPerPageOptions?: number[];
  className?: string;
}

export const CustomTablePagination: React.FC<CustomTablePaginationProps> = ({
  count,
  page,
  rowsPerPage,
  onPageChange,
  onRowsPerPageChange,
  rowsPerPageOptions = [25, 50, 100, 200, 500, 1000],
  className,
}) => {
  const totalPages = Math.max(1, Math.ceil(count / rowsPerPage));
  const startItem = count === 0 ? 0 : page * rowsPerPage + 1;
  const endItem = Math.min((page + 1) * rowsPerPage, count);
  const isFirst = page <= 0;
  const isLast = page >= totalPages - 1;

  const goPrev = (e: React.MouseEvent) => {
    e.preventDefault();
    if (!isFirst) onPageChange(e, page - 1);
  };
  const goNext = (e: React.MouseEvent) => {
    e.preventDefault();
    if (!isLast) onPageChange(e, page + 1);
  };

  const handleRowsChange = (value: string) => {
    // Synthesize a minimal ChangeEvent-shaped object so existing callers
    // that read `event.target.value` keep working without modification.
    onRowsPerPageChange({
      target: { value },
    } as React.ChangeEvent<HTMLInputElement>);
  };

  return (
    <div
      className={cn(
        "flex flex-wrap items-center justify-between gap-3 border-t px-2 py-3",
        className,
      )}
    >
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <span>Rows per page</span>
        <Select value={String(rowsPerPage)} onValueChange={handleRowsChange}>
          <SelectTrigger className="h-8 w-[88px]" size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {rowsPerPageOptions.map((opt) => (
              <SelectItem key={opt} value={String(opt)}>
                {opt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="ml-2 tabular-nums">
          {startItem}–{endItem} of {count}
        </span>
      </div>

      <Pagination className="mx-0 w-auto justify-end">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              onClick={goPrev}
              aria-disabled={isFirst}
              className={cn(isFirst && "pointer-events-none opacity-50")}
            />
          </PaginationItem>
          <PaginationItem>
            <span className="px-3 text-sm tabular-nums">
              Page {page + 1} of {totalPages}
            </span>
          </PaginationItem>
          <PaginationItem>
            <PaginationNext
              href="#"
              onClick={goNext}
              aria-disabled={isLast}
              className={cn(isLast && "pointer-events-none opacity-50")}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
};
