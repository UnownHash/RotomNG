/**
 * Shared QueryClient factory. Both apps mount their own QueryClient via
 * `createAppQueryClient()` so defaults stay in sync as the dashboard
 * evolves. The defaults below are tuned for a polling dashboard:
 *
 *  - staleTime ~one poll: avoids the duplicate fetch a fresh mount would
 *    otherwise fire on top of the in-flight poll.
 *  - refetchOnWindowFocus: false — polling already covers freshness; the
 *    refocus-triggered refetch only burned bandwidth on tab switches.
 *  - retry: 1 — the next poll IS the retry. Three retries with exponential
 *    backoff just delays the user seeing the error UI. A 401 is never
 *    retried: the credential will not materialise on its own, and retrying
 *    only delays `AuthGate` showing the login form.
 *  - gcTime: 5 min — keeps recently-unmounted page data warm during
 *    navigation, dropped soon enough not to leak.
 */

import { QueryClient } from "@tanstack/react-query";
import { isAuthError } from "./api";
import { POLL_INTERVAL_MS } from "./query-options";

export const createAppQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: POLL_INTERVAL_MS - 1000,
        gcTime: 5 * 60_000,
        refetchOnWindowFocus: false,
        retry: (failureCount, error) => !isAuthError(error) && failureCount < 1,
      },
    },
  });
