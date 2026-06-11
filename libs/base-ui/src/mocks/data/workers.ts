import type { MockWorker } from "../types";

const VERSION_NAMES = ["1.0.5", "1.0.4", "1.0.3", "1.1.0"];
const PLATFORMS = ["android", "ios"];

const randInt = (min: number, max: number) =>
  Math.floor(Math.random() * (max - min + 1)) + min;

const randomTimestamp = (maxAgeMs = 10 * 60_000) =>
  Date.now() - randInt(1000, maxAgeMs);

export interface WorkerGenOpts {
  deviceId: string;
  indexOffset?: number;
}

export const generateWorkers = (
  count: number,
  { deviceId, indexOffset = 0 }: WorkerGenOpts,
): MockWorker[] =>
  Array.from({ length: count }, (_, i) => {
    const idx = indexOffset + i;
    const versionName =
      VERSION_NAMES[idx % VERSION_NAMES.length] ?? VERSION_NAMES[0];
    const versionCode = 100 + (idx % 4);
    const lastSeen = randomTimestamp();
    const connectedAt = lastSeen - randInt(60_000, 3_600_000);

    return {
      id: `worker-${idx}`,
      device_id: deviceId,
      origin: `worker-${idx}@${deviceId}`,
      version_code: versionCode,
      version_name: versionName,
      user_agent: `RotomMock/${versionName}`,
      last_connected_at_ms: connectedAt,
      last_seen_at_ms: lastSeen,
      is_connected: Math.random() > 0.1,
      is_in_use: Math.random() > 0.4,
      weight: randInt(1, 10),
      can_be_used: Math.random() > 0.15,
      platform: PLATFORMS[idx % PLATFORMS.length] ?? PLATFORMS[0],
      message_last_received_at_ms: lastSeen,
      messages_received: randInt(100, 100000),
      bytes_received: randInt(10_000, 10_000_000),
      message_last_sent_at_ms: lastSeen - randInt(0, 30_000),
      messages_sent: randInt(100, 100000),
      bytes_sent: randInt(10_000, 10_000_000),
      session: {
        connected_at_ms: connectedAt,
        message_last_received_at_ms: lastSeen,
        messages_received: randInt(100, 50000),
        bytes_received: randInt(10_000, 5_000_000),
        message_last_sent_at_ms: lastSeen - randInt(0, 30_000),
        messages_sent: randInt(100, 50000),
        bytes_sent: randInt(10_000, 5_000_000),
      },
      time_windowed_stats: {
        requests_rate_over_30_seconds: randInt(0, 50) + Math.random(),
        requests_rate_over_1_min: randInt(0, 50) + Math.random(),
        requests_rate_over_5_min: randInt(0, 50) + Math.random(),
        requests_rate_over_15_min: randInt(0, 50) + Math.random(),
        request_ms_avg_over_30_seconds: randInt(50, 500),
        request_ms_avg_over_1_min: randInt(50, 500),
        request_ms_avg_over_5_min: randInt(50, 500),
        request_ms_avg_over_15_min: randInt(50, 500),
      },
    };
  });
