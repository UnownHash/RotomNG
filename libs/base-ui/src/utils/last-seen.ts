/**
 * Calculate the most recent timestamp from message_last_received_at_ms and message_last_sent_at_ms
 * @param lastReceivedMs - The timestamp when the last message was received
 * @param lastSentMs - The timestamp when the last message was sent
 * @returns The most recent timestamp, or 0 if both are falsy
 */
export function getLastSeenTimestamp(
  lastReceivedMs?: number,
  lastSentMs?: number,
): number {
  // Handle cases where timestamps might be undefined, null, or 0
  const receivedTime = lastReceivedMs || 0;
  const sentTime = lastSentMs || 0;

  // Return the most recent timestamp
  return Math.max(receivedTime, sentTime);
}
