/**
 * Multi-instance mode: the UI's view of the rotom-ng servers the admin service
 * fronts, and which of them is currently selected.
 *
 * Mode is detected from the config reply rather than from a build flag, so one
 * UI bundle serves both the admin service and a plain rotom-ng: the admin
 * service always sends `instances` (an empty list when none are configured)
 * and rotom-ng never does.
 */

import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo } from "react";
import {
  setSelectedInstance,
  useSelectedInstance,
} from "../lib/instance-store";
import type { AppConfig, InstanceInfo } from "../types";
import { useConfig } from "./use-config";

export interface InstancesState {
  /** True when the UI is talking to the admin service. */
  multiInstance: boolean;
  /** Every configured instance, in the order the operator listed them. */
  instances: InstanceInfo[];
  /**
   * The selected instance, or null when nothing is selected — either because
   * none is configured, or because none has been reachable yet.
   */
  selected: InstanceInfo | null;
  /** True while the selection points at an instance that is not reachable. */
  selectedUnreachable: boolean;
  /** Selects an instance by url. Ignored for an unreachable one. */
  select: (url: string) => void;
}

/** Display name for an instance: its own name, falling back to its url. */
export const instanceLabel = (instance: InstanceInfo): string =>
  instance.instance || instance.url;

/**
 * Stands in for the name of a rotom-ng that has no instance name configured.
 *
 * Only used in single-instance mode. Fronted by the admin service an unnamed
 * instance shows its url instead, because there the name also has to tell one
 * server from another -- "<unnamed>" twice over would be useless.
 */
export const UNNAMED_INSTANCE = "<unnamed>";

/**
 * The instance name for the header, or null when there is nothing to name yet.
 *
 * Pure so the rules are testable without rendering:
 *
 *   - config not loaded            -> null, so the header does not flicker
 *   - admin service, one selected  -> that instance's name, or its url if unnamed
 *   - admin service, none selected -> null; nothing is being looked at
 *   - a rotom-ng directly          -> its own instance name, or "<unnamed>"
 */
export const resolveInstanceLabel = (
  config: AppConfig | undefined,
  selected: InstanceInfo | null,
): string | null => {
  if (!config) {
    return null;
  }
  // Presence of `instances` is what marks the admin service; see above.
  if (config.instances !== undefined) {
    // Named whether or not it is reachable: the gate says "<x> is not
    // responding" in that case, so the header agrees with it rather than going
    // blank underneath a message about that very instance.
    return selected ? instanceLabel(selected) : null;
  }
  return config.instance || UNNAMED_INSTANCE;
};

/**
 * The instance to adopt when there is no usable selection -- a first visit, or
 * a stored selection whose instance has since gone from the config.
 *
 * Prefers a reachable one, so a first visit lands somewhere that works rather
 * than on an error page, and otherwise takes the first: an all-down fleet still
 * ends up with a selection, which is what lets the UI always be about a
 * specific instance instead of explaining that it is about none of them.
 *
 * Null only when nothing is configured.
 */
export const pickInstance = (instances: InstanceInfo[]): InstanceInfo | null =>
  instances.find((instance) => instance.reachable) ?? instances[0] ?? null;

export const useInstances = (): InstancesState => {
  const { data } = useConfig();
  const selectedUrl = useSelectedInstance();
  const queryClient = useQueryClient();

  const config = data?.config;
  const instances = useMemo(() => config?.instances ?? [], [config?.instances]);
  const multiInstance = config?.instances !== undefined;

  const selected = useMemo(
    () => instances.find((instance) => instance.url === selectedUrl) ?? null,
    [instances, selectedUrl],
  );

  const select = useCallback(
    (url: string) => {
      const target = instances.find((instance) => instance.url === url);
      if (!target?.reachable) {
        return;
      }
      setSelectedInstance(url);
      // Every cached page belongs to the instance it was fetched from, so it
      // all has to go: leaving it would show one instance's devices under
      // another's name until the next poll landed.
      queryClient.removeQueries({
        predicate: (query) =>
          query.queryKey[0] !== "auth" && query.queryKey[0] !== "config",
      });
    },
    [instances, queryClient],
  );

  // Adopt a selection whenever there is not a usable one: a first visit, or a
  // stored selection whose instance has since gone from the config. Either way
  // it ends with something selected, so the UI is always about a specific
  // instance and never has to explain that it is about none of them.
  //
  // Deliberately does NOT move off a selection that has merely gone
  // unreachable: switching servers under an operator mid-task is worse than
  // telling them the one they chose is down.
  useEffect(() => {
    if (!multiInstance || selected !== null) {
      return;
    }
    const adopted = pickInstance(instances);
    setSelectedInstance(adopted ? adopted.url : null);
  }, [multiInstance, selected, instances]);

  return {
    multiInstance,
    instances,
    selected,
    selectedUnreachable: selected !== null && !selected.reachable,
    select,
  };
};

/** The instance name to show in the header. See resolveInstanceLabel. */
export const useInstanceLabel = (): string | null => {
  const { data } = useConfig();
  const { selected } = useInstances();
  return resolveInstanceLabel(data?.config, selected);
};
