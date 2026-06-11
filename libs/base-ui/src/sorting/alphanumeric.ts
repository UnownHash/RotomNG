/**
 * Alphanumeric sorting utilities for table columns
 * Consolidates sorting logic used across multiple table components
 */

// Cache for parsed alphanumeric values to avoid repeated parsing
const parseAlphanumericCache = new Map<
  string,
  { text: string; number: number | null }
>();

/**
 * Parses a string into text and numeric components for alphanumeric sorting
 * @param value - The string value to parse
 * @returns Object containing text part and numeric part (if any)
 */
export const parseAlphanumeric = (
  value: string,
): { text: string; number: number | null } => {
  const cached = parseAlphanumericCache.get(value);
  if (cached !== undefined) {
    return cached;
  }

  const match = value.match(/^(.+?)(\d+)$/);
  const result = match
    ? {
        text: match[1],
        number: parseInt(match[2], 10),
      }
    : {
        text: value,
        number: null,
      };

  // Cache the result to avoid repeated parsing
  parseAlphanumericCache.set(value, result);
  return result;
};

/**
 * Compares two strings using alphanumeric sorting logic
 * First compares text parts, then numeric parts if text parts are equal
 * @param a - First string to compare
 * @param b - Second string to compare
 * @returns Negative if a < b, positive if a > b, zero if equal
 */
export const compareAlphanumeric = (a: string, b: string): number => {
  const parsedA = parseAlphanumeric(a);
  const parsedB = parseAlphanumeric(b);

  // First compare the text parts
  const textComparison = parsedA.text.localeCompare(parsedB.text);
  if (textComparison !== 0) {
    return textComparison;
  }

  // If text parts are equal, compare the numeric parts
  if (parsedA.number === null && parsedB.number === null) {
    return 0;
  }
  if (parsedA.number === null) {
    return -1; // Strings without numbers come before strings with numbers
  }
  if (parsedB.number === null) {
    return 1;
  }

  return parsedA.number - parsedB.number;
};

/**
 * Clears the alphanumeric parsing cache
 * Useful for memory management in long-running applications
 */
export const clearAlphanumericCache = (): void => {
  parseAlphanumericCache.clear();
};

/**
 * Gets the current size of the alphanumeric parsing cache
 * @returns Number of cached entries
 */
export const getAlphanumericCacheSize = (): number => {
  return parseAlphanumericCache.size;
};
