import { useCallback, useEffect, useState } from "react";

export interface UseTablePaginationOptions {
  initialPage?: number;
  initialRowsPerPage?: number;
  tableKey?: string; // Unique key for localStorage persistence
}

export interface UseTablePaginationReturn<T> {
  page: number;
  rowsPerPage: number;
  handleChangePage: (event: unknown, newPage: number) => void;
  handleChangeRowsPerPage: (event: React.ChangeEvent<HTMLInputElement>) => void;
  getPaginatedItems: (items: T[]) => T[];
  resetPage: () => void;
}

// Helper function to safely access localStorage
const getStoredRowsPerPage = (
  tableKey: string,
  defaultValue: number,
): number => {
  if (typeof window === "undefined") return defaultValue;

  try {
    const stored = localStorage.getItem(`table-pagination-${tableKey}`);
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
    localStorage.setItem(`table-pagination-${tableKey}`, value.toString());
  } catch (error) {
    console.warn("Failed to store table pagination to localStorage:", error);
  }
};

export function useTablePagination<T>({
  initialPage = 0,
  initialRowsPerPage = 25,
  tableKey,
}: UseTablePaginationOptions = {}): UseTablePaginationReturn<T> {
  // Get initial rows per page from localStorage if tableKey is provided
  const getInitialRowsPerPage = useCallback(() => {
    return tableKey
      ? getStoredRowsPerPage(tableKey, initialRowsPerPage)
      : initialRowsPerPage;
  }, [tableKey, initialRowsPerPage]);

  const [page, setPage] = useState(initialPage);
  const [rowsPerPage, setRowsPerPage] = useState(getInitialRowsPerPage);

  // Update localStorage when rowsPerPage changes (only if tableKey is provided)
  useEffect(() => {
    if (tableKey && rowsPerPage !== initialRowsPerPage) {
      storeRowsPerPage(tableKey, rowsPerPage);
    }
  }, [rowsPerPage, tableKey, initialRowsPerPage]);

  const handleChangePage = useCallback((_event: unknown, newPage: number) => {
    setPage(newPage);
  }, []);

  const handleChangeRowsPerPage = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const newRowsPerPage = parseInt(event.target.value, 10);
      setRowsPerPage(newRowsPerPage);
      setPage(0);

      // Store to localStorage immediately if tableKey is provided
      if (tableKey) {
        storeRowsPerPage(tableKey, newRowsPerPage);
      }
    },
    [tableKey],
  );

  const resetPage = useCallback(() => {
    setPage(0);
  }, []);

  const getPaginatedItems = useCallback(
    (items: T[]) => {
      const startIndex = page * rowsPerPage;
      const endIndex = startIndex + rowsPerPage;
      return items.slice(startIndex, endIndex);
    },
    [page, rowsPerPage],
  );

  return {
    page,
    rowsPerPage,
    handleChangePage,
    handleChangeRowsPerPage,
    getPaginatedItems,
    resetPage,
  };
}
