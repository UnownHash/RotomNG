import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import {
  type FC,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
} from "react";
import { Button } from "../components/ui/button";
import {
  type AuthState,
  fetchAuthState,
  isAuthError,
  logout as logoutRequest,
} from "../lib/api";
import { AuthContextProvider } from "./auth-context";
import { LoginCard } from "./login-card";

const AUTH_QUERY_KEY = ["auth", "me"] as const;

export interface AuthGateProps {
  children: ReactNode;
  appName: string;
  appIcon?: string;
}

/**
 * Gates the app behind the API secret when one is configured.
 *
 * Rather than a `/login` route, this swaps the rendered tree in place: the URL
 * never changes, so an expired session mid-poll leaves the operator exactly
 * where they were and a deep link survives logging in without any
 * return-to-path plumbing.
 *
 * When no secret is configured the server reports `auth_required: false` and
 * this renders `children` straight through, so unauthenticated deployments
 * behave exactly as they did before.
 */
export const AuthGate: FC<AuthGateProps> = ({ children, appName, appIcon }) => {
  const queryClient = useQueryClient();

  const {
    data: authState,
    isPending,
    isError,
    refetch,
  } = useQuery({
    queryKey: AUTH_QUERY_KEY,
    queryFn: fetchAuthState,
    // The probe is not a polling endpoint; session loss is detected from the
    // 401s that real queries throw (see the cache subscription below), which
    // is both faster and one fewer request every poll interval.
    staleTime: Number.POSITIVE_INFINITY,
    refetchInterval: false,
    retry: 1,
  });

  // Any query or mutation that 401s means the session is gone. Flipping the
  // cached auth state here is what turns an expired cookie into a login form
  // without every page needing to handle 401 itself.
  useEffect(() => {
    // A 401 can only happen when a secret is configured, so auth_required is
    // necessarily true here regardless of what the last probe said.
    const markUnauthenticated = () => {
      queryClient.setQueryData<AuthState>(AUTH_QUERY_KEY, {
        auth_required: true,
        authenticated: false,
      });
    };

    const unsubscribeQueries = queryClient
      .getQueryCache()
      .subscribe((event) => {
        if (isAuthError(event.query.state.error)) {
          markUnauthenticated();
        }
      });

    const unsubscribeMutations = queryClient
      .getMutationCache()
      .subscribe((event) => {
        if (isAuthError(event.mutation?.state.error)) {
          markUnauthenticated();
        }
      });

    return () => {
      unsubscribeQueries();
      unsubscribeMutations();
    };
  }, [queryClient]);

  const logoutMutation = useMutation({
    mutationFn: logoutRequest,
    onSuccess: (state) => {
      queryClient.setQueryData<AuthState>(AUTH_QUERY_KEY, state);
      // Drop every cached page so the next operator to sign in never sees the
      // previous session's data flash before the first poll lands.
      queryClient.removeQueries({ predicate: (q) => q.queryKey[0] !== "auth" });
    },
  });

  const handleLoginSuccess = useCallback(() => {
    queryClient.setQueryData<AuthState>(AUTH_QUERY_KEY, {
      auth_required: true,
      authenticated: true,
    });
    // Queries that failed while logged out are in an error state and would sit
    // there until their next poll; refetch now so the app paints immediately.
    queryClient.invalidateQueries();
  }, [queryClient]);

  const contextValue = useMemo(
    () => ({
      authRequired: authState?.auth_required ?? false,
      logout: () => logoutMutation.mutate(),
      loggingOut: logoutMutation.isPending,
    }),
    [authState?.auth_required, logoutMutation],
  );

  if (isPending) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-app text-muted-foreground">
        <Loader2 className="size-6 animate-spin" />
      </div>
    );
  }

  // A failed probe means the server is unreachable or erroring — distinct from
  // being logged out, and showing a login form here would just be misleading.
  if (isError) {
    return (
      <div className="min-h-screen flex flex-col gap-3 items-center justify-center bg-app text-foreground p-4">
        <p className="text-sm text-muted-foreground">
          Could not reach the {appName} API.
        </p>
        <Button variant="outline" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    );
  }

  if (authState.auth_required && !authState.authenticated) {
    return (
      <LoginCard
        appName={appName}
        appIcon={appIcon}
        onSuccess={handleLoginSuccess}
      />
    );
  }

  return (
    <AuthContextProvider value={contextValue}>{children}</AuthContextProvider>
  );
};
