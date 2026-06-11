const kBNumberFormat = new Intl.NumberFormat("en", {
  style: "unit",
  unit: "kilobyte",
  unitDisplay: "short",
});

const formatCache = new Map<number, string>();

/** Format a kilobyte value with caching to skip repeated NumberFormat allocations. */
export const formatMemory = (value: number): string => {
  const cached = formatCache.get(value);
  if (cached !== undefined) return cached;

  const formatted = kBNumberFormat.format(value);
  formatCache.set(value, formatted);
  return formatted;
};
