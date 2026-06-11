/**
 * Generic table sorting utilities
 * Provides reusable sorting functions for table components
 */

import { compareAlphanumeric } from "./alphanumeric";

export type SortOrder = "asc" | "desc";

/**
 * Generic comparison function for table sorting
 * Handles various data types including strings, numbers, booleans, and nested properties
 * @param a - First item to compare
 * @param b - Second item to compare
 * @param sortBy - Property path to sort by (supports nested properties like 'session.controller.id')
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns Negative if a < b, positive if a > b, zero if equal
 */
export const compareTableItems = <T extends object>(
  a: T,
  b: T,
  sortBy: string,
  sortOrder: SortOrder = "asc",
): number => {
  let aValue: unknown;
  let bValue: unknown;

  // Handle nested properties (e.g., 'session.controller.id')
  if (sortBy.includes(".")) {
    const keys = sortBy.split(".");
    aValue = keys.reduce<unknown>(
      (obj, key) =>
        obj != null ? (obj as Record<string, unknown>)[key] : undefined,
      a,
    );
    bValue = keys.reduce<unknown>(
      (obj, key) =>
        obj != null ? (obj as Record<string, unknown>)[key] : undefined,
      b,
    );
  } else {
    aValue = (a as Record<string, unknown>)[sortBy];
    bValue = (b as Record<string, unknown>)[sortBy];
  }

  // Handle null/undefined values
  if (aValue == null && bValue == null) return 0;
  if (aValue == null) return sortOrder === "asc" ? -1 : 1;
  if (bValue == null) return sortOrder === "asc" ? 1 : -1;

  let comparison = 0;

  if (typeof aValue === "string" && typeof bValue === "string") {
    // Use alphanumeric comparison for strings
    comparison = compareAlphanumeric(
      aValue.toLowerCase(),
      bValue.toLowerCase(),
    );
  } else if (typeof aValue === "number" && typeof bValue === "number") {
    // Direct numeric comparison
    comparison = aValue - bValue;
  } else if (typeof aValue === "boolean" && typeof bValue === "boolean") {
    // Boolean comparison (false < true)
    comparison = aValue === bValue ? 0 : aValue ? 1 : -1;
  } else {
    // Fallback to string comparison for mixed types
    const aStr = String(aValue).toLowerCase();
    const bStr = String(bValue).toLowerCase();
    comparison = compareAlphanumeric(aStr, bStr);
  }

  return sortOrder === "asc" ? comparison : -comparison;
};

/**
 * Worker-specific comparison function that uses worker.id as secondary sort for tie breakers
 * @param a - First worker to compare
 * @param b - Second worker to compare
 * @param sortBy - Property path to sort by (supports nested properties like 'session.controller.id')
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns Negative if a < b, positive if a > b, zero if equal
 */
export const compareWorkerItems = <T extends object & { id?: string }>(
  a: T,
  b: T,
  sortBy: string,
  sortOrder: SortOrder = "asc",
): number => {
  // First, perform the primary sort comparison
  const primaryComparison = compareTableItems(a, b, sortBy, sortOrder);

  // If primary comparison results in a tie, use worker.id as secondary sort
  if (primaryComparison === 0) {
    // Handle cases where one or both workers might not have an id
    if (a.id && b.id) {
      return compareAlphanumeric(a.id.toLowerCase(), b.id.toLowerCase());
    }
    if (a.id && !b.id) {
      return -1; // Workers with id come before workers without id
    }
    if (!a.id && b.id) {
      return 1; // Workers without id come after workers with id
    }
    // If neither has an id, they remain equal (return 0)
  }

  return primaryComparison;
};

/**
 * Device-specific comparison function that uses device.id as secondary sort for tie breakers
 * @param a - First device to compare
 * @param b - Second device to compare
 * @param sortBy - Property path to sort by (supports nested properties like 'session.controller.id')
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns Negative if a < b, positive if a > b, zero if equal
 */
export const compareDeviceItems = <T extends object & { id?: string }>(
  a: T,
  b: T,
  sortBy: string,
  sortOrder: SortOrder = "asc",
): number => {
  // First, perform the primary sort comparison
  const primaryComparison = compareTableItems(a, b, sortBy, sortOrder);

  // If primary comparison results in a tie, use device.id as secondary sort
  if (primaryComparison === 0) {
    // Handle cases where one or both devices might not have an id
    if (a.id && b.id) {
      return compareAlphanumeric(a.id.toLowerCase(), b.id.toLowerCase());
    }
    if (a.id && !b.id) {
      return -1; // Devices with id come before devices without id
    }
    if (!a.id && b.id) {
      return 1; // Devices without id come after devices with id
    }
    // If neither has an id, they remain equal (return 0)
  }

  return primaryComparison;
};

/**
 * Controller-specific comparison function that uses controller.id as secondary sort for tie breakers
 * @param a - First controller to compare
 * @param b - Second controller to compare
 * @param sortBy - Property path to sort by (supports nested properties like 'session.controller.id')
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns Negative if a < b, positive if a > b, zero if equal
 */
export const compareControllerItems = <T extends object & { id?: string }>(
  a: T,
  b: T,
  sortBy: string,
  sortOrder: SortOrder = "asc",
): number => {
  // First, perform the primary sort comparison
  const primaryComparison = compareTableItems(a, b, sortBy, sortOrder);

  // If primary comparison results in a tie, use controller.id as secondary sort
  if (primaryComparison === 0) {
    // Handle cases where one or both controllers might not have an id
    if (a.id && b.id) {
      return compareAlphanumeric(a.id.toLowerCase(), b.id.toLowerCase());
    }
    if (a.id && !b.id) {
      return -1; // Controllers with id come before controllers without id
    }
    if (!a.id && b.id) {
      return 1; // Controllers without id come after controllers with id
    }
    // If neither has an id, they remain equal (return 0)
  }

  return primaryComparison;
};

/**
 * Creates a sorting function for use with Array.sort()
 * @param sortBy - Property path to sort by
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns Sorting function compatible with Array.sort()
 */
export const createTableSorter = <T extends object>(
  sortBy: string,
  sortOrder: SortOrder = "asc",
) => {
  return (a: T, b: T) => compareTableItems(a, b, sortBy, sortOrder);
};

/**
 * Creates a device-specific sorting function for use with Array.sort()
 * Uses device.id as secondary sort for tie breaking
 * @param sortBy - Property path to sort by
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns Sorting function compatible with Array.sort()
 */
export const createDeviceSorter = <T extends object & { id?: string }>(
  sortBy: string,
  sortOrder: SortOrder = "asc",
) => {
  return (a: T, b: T) => compareDeviceItems(a, b, sortBy, sortOrder);
};

/**
 * Creates a controller-specific sorting function for use with Array.sort()
 * Uses controller.id as secondary sort for tie breaking
 * @param sortBy - Property path to sort by
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns Sorting function compatible with Array.sort()
 */
export const createControllerSorter = <T extends object & { id?: string }>(
  sortBy: string,
  sortOrder: SortOrder = "asc",
) => {
  return (a: T, b: T) => compareControllerItems(a, b, sortBy, sortOrder);
};

/**
 * Sorts an array of items using table sorting logic
 * @param items - Array of items to sort
 * @param sortBy - Property path to sort by
 * @param sortOrder - Sort direction ('asc' or 'desc')
 * @returns New sorted array (does not mutate original)
 */
export const sortTableItems = <T extends object>(
  items: T[],
  sortBy: string,
  sortOrder: SortOrder = "asc",
): T[] => {
  return [...items].sort(createTableSorter(sortBy, sortOrder));
};

/**
 * Hook-like function to handle sort state changes
 * Returns the new sort state based on current state and clicked field
 * @param currentSortBy - Current sort field
 * @param currentSortOrder - Current sort order
 * @param clickedField - Field that was clicked
 * @returns New sort state
 */
export const getNextSortState = (
  currentSortBy: string,
  currentSortOrder: SortOrder,
  clickedField: string,
): { sortBy: string; sortOrder: SortOrder } => {
  if (currentSortBy === clickedField) {
    // Same field clicked, toggle order
    return {
      sortBy: clickedField,
      sortOrder: currentSortOrder === "asc" ? "desc" : "asc",
    };
  }
  // Different field clicked, start with ascending
  return {
    sortBy: clickedField,
    sortOrder: "asc",
  };
};
