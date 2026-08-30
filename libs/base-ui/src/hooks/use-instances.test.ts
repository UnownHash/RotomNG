/**
 * Rules for the instance name in the header, and for the name shown in the
 * instance picker. Both hinge on an instance name being allowed to be empty,
 * which is easy to regress and invisible until someone runs an unnamed server.
 *
 * Run with `bun test`.
 */

import { describe, expect, test } from "bun:test";
import type { AppConfig, InstanceInfo } from "../types";
import {
  instanceLabel,
  pickInstance,
  resolveInstanceLabel,
  UNNAMED_INSTANCE,
} from "./use-instances";

const instance = (over: Partial<InstanceInfo> = {}): InstanceInfo => ({
  instance: "east",
  url: "http://10.0.0.10:7072",
  reachable: true,
  ...over,
});

const rotomNgConfig = (over: Partial<AppConfig> = {}): AppConfig => ({
  version: "1.0.0",
  sha: "abc1234",
  ...over,
});

const adminConfig = (instances: InstanceInfo[]): AppConfig => ({
  version: "1.0.0",
  sha: "abc1234",
  instances,
});

describe("instanceLabel", () => {
  test("uses the instance's own name", () => {
    expect(instanceLabel(instance())).toBe("east");
  });

  test("falls back to the url when the name is empty", () => {
    // A rotom-ng with no `instance` set reports "", and the picker still has to
    // tell it apart from its siblings.
    expect(instanceLabel(instance({ instance: "" }))).toBe(
      "http://10.0.0.10:7072",
    );
  });
});

describe("resolveInstanceLabel", () => {
  test("is null until the config has loaded, so the header does not flicker", () => {
    expect(resolveInstanceLabel(undefined, null)).toBeNull();
  });

  describe("talking to a rotom-ng directly", () => {
    test("uses the configured instance name", () => {
      expect(
        resolveInstanceLabel(rotomNgConfig({ instance: "scanner-1" }), null),
      ).toBe("scanner-1");
    });

    test("shows <unnamed> for an empty name", () => {
      expect(resolveInstanceLabel(rotomNgConfig({ instance: "" }), null)).toBe(
        UNNAMED_INSTANCE,
      );
    });

    test("shows <unnamed> when no name is configured at all", () => {
      // rotom-ng omits the key entirely rather than sending "", so both the
      // absent and the empty case have to land here.
      expect(resolveInstanceLabel(rotomNgConfig(), null)).toBe(
        UNNAMED_INSTANCE,
      );
    });
  });

  describe("fronted by the admin service", () => {
    test("follows the selected instance", () => {
      const selected = instance({ instance: "west" });
      expect(resolveInstanceLabel(adminConfig([selected]), selected)).toBe(
        "west",
      );
    });

    test("shows the url when the selected instance has no name", () => {
      // Not "<unnamed>": here the name also has to identify which of several
      // servers is on screen.
      const selected = instance({ instance: "" });
      expect(resolveInstanceLabel(adminConfig([selected]), selected)).toBe(
        "http://10.0.0.10:7072",
      );
    });

    test("is null when nothing is selected", () => {
      expect(resolveInstanceLabel(adminConfig([instance()]), null)).toBeNull();
    });

    test("is null with no instances configured, not <unnamed>", () => {
      // An empty list still means admin mode, so the single-instance fallback
      // must not leak in and name a server that does not exist.
      expect(resolveInstanceLabel(adminConfig([]), null)).toBeNull();
    });

    test("still names an instance that is down while others are up", () => {
      // The gate names it too ("<x> is not responding"), so the header agrees
      // rather than going blank under a message about that very instance.
      const down = instance({ instance: "west", reachable: false });
      expect(resolveInstanceLabel(adminConfig([instance(), down]), down)).toBe(
        "west",
      );
    });

    test("names the selection even when the whole fleet is down", () => {
      // The gate says "a is not responding"; the header agrees rather than
      // going blank underneath a message about that very instance.
      const allDown = [
        instance({ instance: "a", url: "http://a:7072", reachable: false }),
        instance({ instance: "b", url: "http://b:7072", reachable: false }),
      ];
      expect(resolveInstanceLabel(adminConfig(allDown), allDown[0])).toBe("a");
    });
  });
});

describe("pickInstance", () => {
  test("prefers a reachable instance over an earlier unreachable one", () => {
    // A first visit should land somewhere that works rather than on an error.
    const down = instance({
      instance: "a",
      url: "http://a:7072",
      reachable: false,
    });
    const up = instance({ instance: "b", url: "http://b:7072" });
    expect(pickInstance([down, up])?.instance).toBe("b");
  });

  test("keeps config order among reachable instances", () => {
    const first = instance({ instance: "a", url: "http://a:7072" });
    const second = instance({ instance: "b", url: "http://b:7072" });
    expect(pickInstance([first, second])?.instance).toBe("a");
  });

  test("falls back to the first when none are reachable", () => {
    // An all-down fleet still gets a selection, so the UI is about a specific
    // instance and can say that one is down.
    const a = instance({
      instance: "a",
      url: "http://a:7072",
      reachable: false,
    });
    const b = instance({
      instance: "b",
      url: "http://b:7072",
      reachable: false,
    });
    expect(pickInstance([a, b])?.instance).toBe("a");
  });

  test("is null only when nothing is configured", () => {
    expect(pickInstance([])).toBeNull();
  });
});
