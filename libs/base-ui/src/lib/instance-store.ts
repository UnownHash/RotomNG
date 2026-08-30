/**
 * The selected rotom-ng instance, when the UI is fronted by the admin service.
 *
 * This is a module-level store rather than React state because `apiFetch` has
 * to read the selection synchronously, outside any component — every request
 * carries it as a header. `useSyncExternalStore` then lets components
 * re-render on a change without a second copy of the value drifting out of
 * sync with the one requests actually use.
 *
 * The value stored is an instance's base url, which the server guarantees is
 * unique; instance names can be empty or shared between servers.
 */

import { useSyncExternalStore } from "react";

/**
 * Header naming the instance a request is for. The server also accepts an
 * instance name here, but the UI always sends the url.
 */
export const INSTANCE_HEADER = "X-Rotom-Instance";

const STORAGE_KEY = "rotom-ng.selected-instance";

let selectedInstance: string | null = readStoredInstance();

const listeners = new Set<() => void>();

function readStoredInstance(): string | null {
  // Storage access throws in private-browsing modes and when cookies are
  // blocked; an unremembered selection is a far better outcome than a blank
  // page, so failures fall back to "nothing selected".
  try {
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeStoredInstance(url: string | null): void {
  try {
    if (url === null) {
      window.localStorage.removeItem(STORAGE_KEY);
    } else {
      window.localStorage.setItem(STORAGE_KEY, url);
    }
  } catch {
    // Selection still applies for this session; it just won't be remembered.
  }
}

/** The selected instance url, or null when none has been chosen. */
export const getSelectedInstance = (): string | null => selectedInstance;

/**
 * Selects an instance and persists the choice. Passing null clears it.
 * No-ops when the value is unchanged, so callers can set it unconditionally
 * without causing a re-render loop.
 */
export const setSelectedInstance = (url: string | null): void => {
  if (selectedInstance === url) {
    return;
  }
  selectedInstance = url;
  writeStoredInstance(url);
  for (const listener of listeners) {
    listener();
  }
};

const subscribe = (listener: () => void): (() => void) => {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
};

/** Subscribes a component to the selected instance. */
export const useSelectedInstance = (): string | null =>
  useSyncExternalStore(subscribe, getSelectedInstance, getSelectedInstance);
