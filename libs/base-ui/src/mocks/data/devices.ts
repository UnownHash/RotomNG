import type { MockDevice, MockWorker } from "../types";
import { generateWorkers } from "./workers";

const randInt = (min: number, max: number) =>
  Math.floor(Math.random() * (max - min + 1)) + min;
const randomTimestamp = (maxAgeMs = 10 * 60_000) =>
  Date.now() - randInt(1000, maxAgeMs);

export interface DeviceGenOpts {
  workersTotal: number;
}

export const generateDevices = (
  count: number,
  { workersTotal }: DeviceGenOpts,
): MockDevice[] => {
  const perDevice = count > 0 ? Math.ceil(workersTotal / count) : 0;
  let workerOffset = 0;
  let remaining = workersTotal;

  return Array.from({ length: count }, (_, i) => {
    const workerCount = Math.min(perDevice, remaining);
    remaining -= workerCount;
    const id = `device-${i}`;
    const workers: MockWorker[] = generateWorkers(workerCount, {
      deviceId: id,
      indexOffset: workerOffset,
    });
    workerOffset += workerCount;
    const lastSeen = randomTimestamp();
    const connectedAt = lastSeen - randInt(60_000, 3_600_000);
    const inUse = workers.filter((w) => w.is_in_use);
    const maxWeight = workers.reduce((sum, w) => sum + (w.weight ?? 0), 0);
    const inUseWeight = inUse.reduce((sum, w) => sum + (w.weight ?? 0), 0);

    return {
      id,
      origin: `mock-origin-${i}`,
      version: `1.2.${i % 5}`,
      public_ip: `10.0.${Math.floor(i / 256)}.${i % 256}`,
      worker_count: workers.length,
      worker_in_use_count: inUse.length,
      worker_in_use_percent:
        workers.length === 0 ? 0 : (inUse.length / workers.length) * 100,
      worker_in_use_weight: inUseWeight,
      worker_in_use_weight_percent:
        maxWeight === 0 ? 0 : (inUseWeight / maxWeight) * 100,
      worker_max_weight: maxWeight,
      last_connected_at_ms: connectedAt,
      last_seen_at_ms: lastSeen,
      enabled: Math.random() > 0.1,
      is_connected: Math.random() > 0.1,
      is_in_use: Math.random() > 0.4,
      can_be_used: Math.random() > 0.15,
      last_memory: {
        free: randInt(100_000, 2_000_000),
        mitm: randInt(50_000, 500_000),
        start: randInt(10_000, 200_000),
      },
      workers,
      session: {
        connected_at_ms: connectedAt,
        message_last_received_at_ms: lastSeen,
        messages_received: randInt(1000, 500_000),
        bytes_received: randInt(100_000, 50_000_000),
        message_last_sent_at_ms: lastSeen - randInt(0, 60_000),
        messages_sent: randInt(1000, 500_000),
        bytes_sent: randInt(100_000, 50_000_000),
        last_seen_at_ms: lastSeen,
      },
      message_last_received_at_ms: lastSeen,
      messages_received: randInt(1000, 1_000_000),
      bytes_received: randInt(100_000, 100_000_000),
      message_last_sent_at_ms: lastSeen - randInt(0, 60_000),
      messages_sent: randInt(1000, 1_000_000),
      bytes_sent: randInt(100_000, 100_000_000),
    };
  });
};
