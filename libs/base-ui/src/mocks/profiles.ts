import type { MockProfileCounts } from "./types";

export type ProfileName = "empty" | "small" | "medium" | "large";

export const PROFILE_SIZES: Record<ProfileName, MockProfileCounts> = {
  empty: {
    devices: 0,
    controllers: 0,
    workers: 0,
    jobs: 0,
    instances: 0,
  },
  small: {
    devices: 3,
    controllers: 5,
    workers: 10,
    jobs: 2,
    instances: 4,
  },
  medium: {
    devices: 15,
    controllers: 30,
    workers: 100,
    jobs: 6,
    instances: 20,
  },
  large: {
    devices: 50,
    controllers: 100,
    workers: 500,
    jobs: 20,
    instances: 80,
  },
};

export interface MockOptions {
  profile: ProfileName;
  live: boolean;
}

/** Parse `?mock=<csv>` once at module load; modifying the URL requires a reload. */
export const readMockOptions = (): MockOptions => {
  if (typeof window === "undefined") {
    return { profile: "medium", live: false };
  }
  const raw = new URLSearchParams(window.location.search).get("mock") ?? "";
  const parts = raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  let profile: ProfileName = "medium";
  let live = false;

  for (const part of parts) {
    if (part === "live") live = true;
    else if (part in PROFILE_SIZES) profile = part as ProfileName;
  }

  return { profile, live };
};
