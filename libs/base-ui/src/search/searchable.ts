/**
 * One declaration of what a table's search box looks at, used for three things
 * at once: building the haystack a row is matched against, telling the reader
 * which fields were searched when nothing matched, and keeping those two from
 * drifting apart.
 *
 * They used to drift. The worker tables ran two filters over one search box --
 * an outer pass over devices and an inner pass over workers, each with its own
 * field list. A term only the inner list knew about could never reach it,
 * because the outer pass had already discarded the device, so searching
 * "active" always came back empty.
 */

export interface SearchableField<T> {
  /** Shown to the reader when a search matches nothing. */
  label: string;
  get: (item: T) => string | number | null | undefined;
}

/**
 * Trim, then lower-case. Both halves matter: the old code trimmed in one filter
 * and not the other, so a worker id pasted out of a log with a trailing space
 * matched nothing.
 */
export const normalizeSearchTerm = (term: string): string =>
  term.trim().toLowerCase();

/**
 * Flattens a row into the single lower-cased string its search term is tested
 * against. Worth precomputing once per row rather than per keystroke -- these
 * tables carry thousands of rows.
 */
export const buildSearchIndex = <T>(
  item: T,
  fields: SearchableField<T>[],
): string =>
  fields
    .map((field) => field.get(item))
    .filter((value) => value !== null && value !== undefined && value !== "")
    .join(" ")
    .toLowerCase();

/** "origin, worker ID, device ID" -- for the no-matches message. */
export const describeSearchableFields = <T>(
  fields: SearchableField<T>[],
): string => fields.map((field) => field.label).join(", ");

/** Convenience for tables that filter a modest list without precomputing. */
export const matchesSearch = <T>(
  item: T,
  fields: SearchableField<T>[],
  normalizedTerm: string,
): boolean =>
  !normalizedTerm || buildSearchIndex(item, fields).includes(normalizedTerm);
