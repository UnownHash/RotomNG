import type { MockController } from "../types";

const randInt = (min: number, max: number) =>
  Math.floor(Math.random() * (max - min + 1)) + min;
const randomTimestamp = (maxAgeMs = 10 * 60_000) =>
  Date.now() - randInt(1000, maxAgeMs);

export const generateControllers = (count: number): MockController[] =>
  Array.from({ length: count }, (_, i) => {
    const connectedAt = randomTimestamp(60 * 60_000 * 24);
    const lastSeen = randomTimestamp();
    return {
      id: `ctrl-${i}`,
      uuid: `00000000-0000-4000-8000-${String(i).padStart(12, "0")}`,
      user_agent: `RotomController/1.0.${i % 5}`,
      weight: randInt(1, 10),
      proto_major_version: 1,
      proto_minor_version: 0,
      worker_id: `worker-${i % Math.max(1, count)}`,
      account_username: `mock_user_${i}`,
      account_source: i % 2 === 0 ? "ptc" : "google",
      connected_at_ms: connectedAt,
      message_last_received_at_ms: lastSeen,
      messages_received: randInt(100, 100_000),
      bytes_received: randInt(10_000, 10_000_000),
      message_last_sent_at_ms: lastSeen - randInt(0, 30_000),
      messages_sent: randInt(100, 100_000),
      bytes_sent: randInt(10_000, 10_000_000),
    };
  });
