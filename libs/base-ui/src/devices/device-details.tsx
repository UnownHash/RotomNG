import type React from "react";
import { memo, useMemo } from "react";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import {
  TABLE_BODY_ROW,
  TABLE_HEADER_ROW,
  TABLE_WRAPPER,
} from "../lib/aesthetic";
import { formatMemory } from "../lib/format-memory";
import { CustomRelativeTimeLabel } from "../time-label";
import type { Device } from "../types";

interface DeviceDetailsProps {
  device: Device;
}

const DeviceDetailsComponent: React.FC<DeviceDetailsProps> = ({ device }) => {
  // Memoize the formatted memory values to avoid recalculation
  const memoryData = useMemo(() => {
    if (!device.last_memory) {
      return null;
    }

    const { free, mitm, start } = device.last_memory;
    return {
      free: formatMemory(free),
      mitm: formatMemory(mitm),
      start: formatMemory(start),
    };
  }, [device.last_memory]);

  // Calculate the most recent timestamp for "Last Seen"
  const lastSeenTimestamp = useMemo(() => {
    const lastRecv = device.message_last_received_at_ms || 0;
    const lastSent = device.message_last_sent_at_ms || 0;
    return Math.max(lastRecv, lastSent);
  }, [device.message_last_received_at_ms, device.message_last_sent_at_ms]);

  const hasSession = device.session;

  return (
    <div className="rounded-lg border border-border/40 bg-card/40 backdrop-blur-xl p-4 m-2">
      <div className="flex flex-col gap-6">
        {/* Memory Statistics Table */}
        <div>
          <p className="text-sm font-medium text-muted-foreground mb-2">
            Memory Statistics
          </p>
          {memoryData ? (
            <div className={TABLE_WRAPPER}>
              <Table>
                <TableHeader>
                  <TableRow className={TABLE_HEADER_ROW}>
                    <TableHead>Free Memory</TableHead>
                    <TableHead>MITM Memory</TableHead>
                    <TableHead>Start Memory</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow className={TABLE_BODY_ROW}>
                    <TableCell>{memoryData.free}</TableCell>
                    <TableCell>{memoryData.mitm}</TableCell>
                    <TableCell>{memoryData.start}</TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              No memory data available
            </p>
          )}
        </div>

        {/* Total Statistics Table */}
        <div>
          <p className="text-sm font-medium text-muted-foreground mb-2">
            Total Statistics
          </p>
          <div className={TABLE_WRAPPER}>
            <Table>
              <TableHeader>
                <TableRow className={TABLE_HEADER_ROW}>
                  <TableHead>Last Connected At</TableHead>
                  <TableHead>Msgs Recv</TableHead>
                  <TableHead>Bytes Recv</TableHead>
                  <TableHead>Msgs Sent</TableHead>
                  <TableHead>Bytes Sent</TableHead>
                  <TableHead>Last Seen</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow className={TABLE_BODY_ROW}>
                  <TableCell>
                    <CustomRelativeTimeLabel
                      timestamp={device.last_connected_at_ms}
                    />
                  </TableCell>
                  <TableCell>
                    {device.messages_received.toLocaleString()}
                  </TableCell>
                  <TableCell>
                    {(device.bytes_received / 1024).toFixed(2)} KB
                  </TableCell>
                  <TableCell>{device.messages_sent.toLocaleString()}</TableCell>
                  <TableCell>
                    {(device.bytes_sent / 1024).toFixed(2)} KB
                  </TableCell>
                  <TableCell>
                    <CustomRelativeTimeLabel timestamp={lastSeenTimestamp} />
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </div>

        {/* Session Statistics Table */}
        <div>
          <p className="text-sm font-medium text-muted-foreground mb-2">
            Session Statistics
          </p>
          {hasSession ? (
            <div className={TABLE_WRAPPER}>
              <Table>
                <TableHeader>
                  <TableRow className={TABLE_HEADER_ROW}>
                    <TableHead>Connected</TableHead>
                    <TableHead>Msgs Recv</TableHead>
                    <TableHead>Bytes Recv</TableHead>
                    <TableHead>Last Recv</TableHead>
                    <TableHead>Msgs Sent</TableHead>
                    <TableHead>Bytes Sent</TableHead>
                    <TableHead>Last Sent</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  <TableRow className={TABLE_BODY_ROW}>
                    <TableCell>
                      <CustomRelativeTimeLabel
                        timestamp={hasSession.connected_at_ms}
                      />
                    </TableCell>
                    <TableCell>
                      {hasSession.messages_received.toLocaleString()}
                    </TableCell>
                    <TableCell>
                      {(hasSession.bytes_received / 1024).toFixed(2)} KB
                    </TableCell>
                    <TableCell>
                      <CustomRelativeTimeLabel
                        timestamp={device.message_last_received_at_ms}
                      />
                    </TableCell>
                    <TableCell>
                      {hasSession.messages_sent.toLocaleString()}
                    </TableCell>
                    <TableCell>
                      {(hasSession.bytes_sent / 1024).toFixed(2)} KB
                    </TableCell>
                    <TableCell>
                      <CustomRelativeTimeLabel
                        timestamp={device.message_last_sent_at_ms}
                      />
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No active session</p>
          )}
        </div>
      </div>
    </div>
  );
};

// Export memoized component
export const DeviceDetails = memo(DeviceDetailsComponent);
