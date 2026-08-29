import { type RefObject, useCallback, useState } from "react";

export interface UseTablePaginationOptions {
  /**
   * Rows left after filtering. Required, because the page index has to be kept
   * inside the list: without it a reader who searches from page 4 lands on a
   * slice past the end of the results and sees an empty table.
   */
  itemCount: number;
  /**
   * Page jumps back to the first whenever this value changes. Pass the search
   * term -- filtering changes what "page 4" refers to, so staying on it is
   * never what the reader meant.
   */
  resetKey?: unknown;
  /**
   * Scrolled back into view on every page change. Without it, paging forward
   * leaves the reader at the bottom of the page they just left, looking at the
   * footer of a table whose rows have all been replaced.
   */
  scrollTargetRef?: RefObject<HTMLElement | null>;
  initialRowsPerPage?: number;
  /** Unique key for persisting rows-per-page across visits. */
  tableKey?: string;
}

export interface UseTablePaginationReturn<T> {
  page: number;
  rowsPerPage: number;
  handleChangePage: (event: unknown, newPage: number) => void;
  handleChangeRowsPerPage: (event: React.ChangeEvent<HTMLInputElement>) => void;
  getPaginatedItems: (items: T[]) => T[];
}

/**
 * Clears the sticky header plus a little breathing room, so the scrolled-to
 * table does not start underneath it.
 */
const SCROLL_HEADER_OFFSET = 72;

const storageKey = (tableKey: string) => `table-pagination-${tableKey}`;

// Helper function to safely access localStorage
const getStoredRowsPerPage = (
  tableKey: string,
  defaultValue: number,
): number => {
  if (typeof window === "undefined") return defaultValue;

  try {
    const stored = localStorage.getItem(storageKey(tableKey));
    if (stored) {
      const parsed = parseInt(stored, 10);
      // Validate that the stored value is a reasonable pagination size
      if (parsed > 0 && parsed <= 10000) {
        return parsed;
      }
    }
  } catch (error) {
    console.warn("Failed to read table pagination from localStorage:", error);
  }

  return defaultValue;
};

// Helper function to safely store to localStorage
const storeRowsPerPage = (tableKey: string, value: number): void => {
  if (typeof window === "undefined") return;

  try {
    localStorage.setItem(storageKey(tableKey), value.toString());
  } catch (error) {
    console.warn("Failed to store table pagination to localStorage:", error);
  }
};

const prefersReducedMotion = (): boolean => {
  if (typeof window === "undefined" || !window.matchMedia) return false;
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
};

export function useTablePagination<T>({
  itemCount,
  resetKey,
  scrollTargetRef,
  initialRowsPerPage = 25,
  tableKey,
}: UseTablePaginationOptions): UseTablePaginationReturn<T> {
  const [rowsPerPage, setRowsPerPage] = useState(() =>
    tableKey
      ? getStoredRowsPerPage(tableKey, initialRowsPerPage)
      : initialRowsPerPage,
  );
  const [requestedPage, setRequestedPage] = useState(0);

  // Adjusting during render rather than in an effect: an effect would paint one
  // frame of the stale page against the new results before correcting itself.
  const [lastResetKey, setLastResetKey] = useState(resetKey);
  if (resetKey !== lastResetKey) {
    setLastResetKey(resetKey);
    setRequestedPage(0);
  }

  // Clamped rather than reset, because itemCount also moves on its own as
  // devices join and drop off the poll. Resetting on that would drag the reader
  // back to the first page every few seconds; clamping only intervenes when the
  // requested page has nothing left on it, and restores the reader's position
  // if the rows come back.
  const lastPage = Math.max(0, Math.ceil(itemCount / rowsPerPage) - 1);
  const page = Math.min(requestedPage, lastPage);

  const scrollTableIntoView = useCallback(() => {
    const target = scrollTargetRef?.current;
    if (!target || typeof window === "undefined") return;

    const top =
      target.getBoundingClientRect().top +
      window.scrollY -
      SCROLL_HEADER_OFFSET;
    window.scrollTo({
      top: Math.max(0, top),
      behavior: prefersReducedMotion() ? "auto" : "smooth",
    });
  }, [scrollTargetRef]);

  const handleChangePage = useCallback(
    (_event: unknown, newPage: number) => {
      setRequestedPage(newPage);
      scrollTableIntoView();
    },
    [scrollTableIntoView],
  );

  const handleChangeRowsPerPage = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const newRowsPerPage = parseInt(event.target.value, 10);
      setRowsPerPage(newRowsPerPage);
      setRequestedPage(0);

      if (tableKey) {
        storeRowsPerPage(tableKey, newRowsPerPage);
      }
      scrollTableIntoView();
    },
    [tableKey, scrollTableIntoView],
  );

  const getPaginatedItems = useCallback(
    (items: T[]) => {
      const startIndex = page * rowsPerPage;
      return items.slice(startIndex, startIndex + rowsPerPage);
    },
    [page, rowsPerPage],
  );

  return {
    page,
    rowsPerPage,
    handleChangePage,
    handleChangeRowsPerPage,
    getPaginatedItems,
  };
}
