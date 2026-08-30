/**
 * Priority order for the multi-instance gate. This is the substance of the
 * component, and it is easy to get subtly wrong: a misordered branch produces a
 * message that is true but misleading rather than an obvious break.
 *
 * Run with `bun test`.
 */

import { describe, expect, test } from "bun:test";
import type { InstancesState } from "../hooks/use-instances";
import type { InstanceInfo } from "../types";
import { resolveInstanceGate } from "./instance-gate";

const instance = (over: Partial<InstanceInfo> = {}): InstanceInfo => ({
  instance: "east",
  url: "http://10.0.0.10:7072",
  reachable: true,
  ...over,
});

/** Builds the state useInstances would produce for a given list + selection. */
const state = (
  instances: InstanceInfo[],
  selectedUrl: string | null = null,
): InstancesState => {
  const selected = instances.find((entry) => entry.url === selectedUrl) ?? null;
  return {
    multiInstance: true,
    instances,
    selected,
    selectedUnreachable: selected !== null && !selected.reachable,
    select: () => undefined,
  };
};

describe("resolveInstanceGate", () => {
  test("passes through when not fronted by the admin service", () => {
    expect(
      resolveInstanceGate({ ...state([]), multiInstance: false }).kind,
    ).toBe("ready");
  });

  test("says so when none are configured", () => {
    expect(resolveInstanceGate(state([])).kind).toBe("none-configured");
  });

  test("shows the pages once a reachable instance is selected", () => {
    const up = instance();
    expect(resolveInstanceGate(state([up], up.url)).kind).toBe("ready");
  });

  test("waits quietly while a selection is being adopted", () => {
    // Reachable but nothing chosen yet: one frame, so no message.
    expect(resolveInstanceGate(state([instance()])).kind).toBe("pending");
  });

  test("reports the selected instance being down while others are up", () => {
    const down = instance({
      instance: "west",
      url: "http://down:7072",
      reachable: false,
    });
    const gate = resolveInstanceGate(state([instance(), down], down.url));
    expect(gate.kind).toBe("selected-unreachable");
    expect(gate).toHaveProperty("label", "west");
  });

  test("names an unnamed instance by url when it is the one that is down", () => {
    const down = instance({
      instance: "",
      url: "http://down:7072",
      reachable: false,
    });
    const gate = resolveInstanceGate(state([instance(), down], down.url));
    expect(gate).toHaveProperty("label", "http://down:7072");
  });

  describe("when nothing at all is reachable", () => {
    const allDown = [
      instance({ instance: "a", url: "http://a:7072", reachable: false }),
      instance({ instance: "b", url: "http://b:7072", reachable: false }),
    ];

    // No separate all-down state: a selection is always adopted, so this is
    // just the selected instance being down, said once about the instance the
    // operator is actually on.
    test("reports the selected instance, not the fleet", () => {
      const gate = resolveInstanceGate(state(allDown, "http://a:7072"));
      expect(gate.kind).toBe("selected-unreachable");
      expect(gate).toHaveProperty("label", "a");
    });

    test("waits quietly in the frame before one is adopted", () => {
      expect(resolveInstanceGate(state(allDown)).kind).toBe("pending");
    });
  });
});
