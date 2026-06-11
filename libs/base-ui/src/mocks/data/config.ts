import type { ConfigResponse } from "../../types";

export const generateConfig = (): ConfigResponse => ({
  status: "ok",
  config: {
    version: "1.0.5-mock",
    sha: "mockmockmockmock",
    instance: "mock-instance",
    tuning: { profiling: false },
  },
});
