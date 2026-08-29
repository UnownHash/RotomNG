/**
 * Centralized fetch helpers for the rotom UI. Each function returns the
 * parsed JSON body typed against the matching base-ui type. Pages should
 * import the `*Query` factory from `query-options.ts` rather than calling
 * these directly — the factories pin the queryKey + cache options.
 *
 * Throwing on non-2xx means TanStack Query sees the failure, hits its retry
 * logic, and surfaces an `error` to the consumer. Without the check, a 500
 * + html body would silently parse to `null` and the UI would mis-render.
 *
 * Every request goes through `apiFetch`, which attaches the header the server
 * requires on cookie-authenticated requests. Calling `fetch` directly against
 * `/api` will 401 whenever an api secret is configured — the cookie alone is
 * not enough.
 */

import type { ConfigResponse, JobInstances, Jobs, Status } from "../types";

/**
 * Header marking a request as intending to authenticate with the session
 * cookie. The server only honours the cookie when this is present, which is
 * what stops a cross-site form post from riding a logged-in operator's
 * session — a cross-origin caller cannot set it without a preflight.
 */
const SESSION_REQUEST_HEADER = "X-Rotom-Session";

/**
 * Thrown when the API rejects a request for want of a valid credential.
 * `AuthGate` watches for this to send the operator back to the login form,
 * and `query-client.ts` skips retries on it — retrying a 401 just burns
 * requests, the credential will not appear on its own.
 */
export class AuthError extends Error {
  constructor(message = "Unauthorized") {
    super(message);
    this.name = "AuthError";
  }
}

export const isAuthError = (error: unknown): error is AuthError =>
  error instanceof AuthError;

/**
 * Core request: adds the UI header and keeps same-origin cookies on the
 * request. Returns the raw response, 401s included, so callers that need to
 * read an error body off a 401 (login) can do so.
 */
const requestApi = (input: string, init?: RequestInit): Promise<Response> => {
  const headers = new Headers(init?.headers);
  headers.set(SESSION_REQUEST_HEADER, "1");

  return fetch(input, { ...init, headers, credentials: "same-origin" });
};

/**
 * `fetch` for the rotom API. As `requestApi`, but converts a 401 into an
 * `AuthError` so an expired session surfaces as a login prompt rather than a
 * generic failure toast.
 */
export const apiFetch = async (
  input: string,
  init?: RequestInit,
): Promise<Response> => {
  const res = await requestApi(input, init);
  if (res.status === 401) {
    throw new AuthError();
  }
  return res;
};

const okJson = async <T>(res: Response): Promise<T> => {
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
};

/** Shape of the `/api/auth/me` probe used to decide whether to show login. */
export interface AuthState {
  auth_required: boolean;
  authenticated: boolean;
}

export const fetchAuthState = (): Promise<AuthState> =>
  apiFetch("/api/auth/me").then(okJson<AuthState>);

export const login = (secret: string): Promise<AuthState> =>
  requestApi("/api/auth/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ secret }),
  }).then(async (res) => {
    if (!res.ok) {
      // The server distinguishes a wrong secret from a malformed request;
      // surface its message so the form can say which.
      let message = `${res.status} ${res.statusText}`;
      try {
        const body = await res.json();
        if (typeof body?.error === "string" && body.error.trim()) {
          message = body.error;
        }
      } catch {
        // Keep the status-line fallback.
      }
      throw new Error(message);
    }
    return res.json() as Promise<AuthState>;
  });

export const logout = (): Promise<AuthState> =>
  apiFetch("/api/auth/logout", { method: "POST" }).then(okJson<AuthState>);

export const fetchStatus = (): Promise<Status> =>
  apiFetch("/api/status").then(okJson<Status>);

export const fetchConfig = (): Promise<ConfigResponse> =>
  apiFetch("/api/config").then(okJson<ConfigResponse>);

export const fetchJobs = (): Promise<{ jobs: Jobs }> =>
  apiFetch("/api/job").then(okJson<{ jobs: Jobs }>);

export const fetchJobInstances = (): Promise<{ instances: JobInstances }> =>
  apiFetch("/api/job-instance").then(okJson<{ instances: JobInstances }>);
