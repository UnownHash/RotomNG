import { useCallback } from "react";
import { useSearchParams } from "react-router";

/** The query parameter every table's search box reads and writes. */
export const SEARCH_PARAM = "q";

/**
 * Table search, stored in the URL rather than in component state.
 *
 * Three things fall out of that, and the third is the point:
 *
 *  - A search survives a reload, so a poll failure or an accidental refresh
 *    does not throw away what you were looking at.
 *  - A search can be pasted to someone else.
 *  - One page can link into another page's filter. Workers, devices and
 *    controllers live on separate routes with no edge between them in the
 *    data, so the only way to get from a worker to its device is to search
 *    for it. A link that carries the term does that in one click.
 *
 * Replaces rather than pushes, so typing does not bury the previous page
 * under one history entry per keystroke.
 */
export const useSearchParamState = (): [string, (next: string) => void] => {
  const [searchParams, setSearchParams] = useSearchParams();
  const search = searchParams.get(SEARCH_PARAM) ?? "";

  const setSearch = useCallback(
    (next: string) => {
      setSearchParams(
        (current) => {
          const params = new URLSearchParams(current);
          if (next) {
            params.set(SEARCH_PARAM, next);
          } else {
            params.delete(SEARCH_PARAM);
          }
          return params;
        },
        { replace: true },
      );
    },
    [setSearchParams],
  );

  return [search, setSearch];
};

/** A link into another table, pre-filtered to `term`. */
export const searchLink = (path: string, term: string): string =>
  `${path}?${SEARCH_PARAM}=${encodeURIComponent(term)}`;
