import { SearchX } from "lucide-react";
import type { FC } from "react";
import { TableCell, TableRow } from "./ui/table";

export interface TableEmptyStateProps {
  /** Must span the full table, or the message sits under the first column. */
  colSpan: number;
  /** The term as typed. Empty when the table itself has nothing in it. */
  search: string;
  /** Plural, lower case: "devices", "workers", "controllers". */
  noun: string;
  /** Human-readable field list, from describeSearchableFields. */
  searchableFields: string;
}

/**
 * The row a table shows when it has nothing to show.
 *
 * Previously these tables just rendered an empty tbody, which reads the same
 * whether the search matched nothing, the fleet is empty, or the request
 * failed. Naming the term and the fields it was matched against turns "the
 * search is broken" into "that is not a field I search".
 */
export const TableEmptyState: FC<TableEmptyStateProps> = ({
  colSpan,
  search,
  noun,
  searchableFields,
}) => (
  <TableRow className="hover:bg-transparent">
    <TableCell colSpan={colSpan} className="py-12 text-center">
      <div className="flex flex-col items-center gap-2 text-muted-foreground">
        <SearchX className="size-6 opacity-60" aria-hidden="true" />
        {search ? (
          <>
            <p className="text-sm">
              No {noun} match{" "}
              <span className="font-medium text-foreground">“{search}”</span>
            </p>
            <p className="text-xs">Searched {searchableFields}.</p>
          </>
        ) : (
          <p className="text-sm">No {noun} connected.</p>
        )}
      </div>
    </TableCell>
  </TableRow>
);
