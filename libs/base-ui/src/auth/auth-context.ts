import { createContext, useContext } from "react";

export interface AuthContextValue {
  /** Whether the server has an api secret configured at all. */
  authRequired: boolean;
  /** Ends the session and returns the gate to the login form. */
  logout: () => void;
  /** True while a logout request is in flight. */
  loggingOut: boolean;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export const AuthContextProvider = AuthContext.Provider;

/**
 * Auth state for components rendered inside `AuthGate`.
 *
 * Returns `null` outside a provider rather than throwing, so shared chrome
 * like `Layout` can offer a logout control when it happens to be wrapped and
 * render normally when it isn't. Consuming apps adopt `AuthGate` on their own
 * schedule.
 */
export const useAuthOptional = (): AuthContextValue | null =>
  useContext(AuthContext);
