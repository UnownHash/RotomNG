import type React from "react";
import { memo } from "react";
import { TimeWindowStats } from "../components/time-window-stats";
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
import { CustomRelativeTimeLabel } from "../time-label";
import type { Worker } from "../types";

interface WorkerDetailsProps {
  worker: Worker;
}

const WorkerDetailsComponent: React.FC<WorkerDetailsProps> = ({ worker }) => {
  const hasSession = worker.session;

  return (
    <div className="rounded-lg border border-border/40 bg-card/40 backdrop-blur-xl p-4 m-2">
      <div className="flex flex-col gap-6">
        {/* Worker Details Table */}
        <div>
          <p className="text-sm font-medium text-muted-foreground mb-2">
            Worker Details
          </p>
          <div className={TABLE_WRAPPER}>
            <Table>
              <TableHeader>
                <TableRow className={TABLE_HEADER_ROW}>
                  <TableHead>User Agent</TableHead>
                  <TableHead>Version Code</TableHead>
                  <TableHead>Version Name</TableHead>
                  <TableHead>Device ID</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                <TableRow className={TABLE_BODY_ROW}>
                  <TableCell>{worker.user_agent || "N/A"}</TableCell>
                  <TableCell>{worker.version_code || "N/A"}</TableCell>
                  <TableCell>{worker.version_name || "N/A"}</TableCell>
                  <TableCell>{worker.device_id || "N/A"}</TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </div>

        {/* Session Statistics Table */}
        {hasSession ? (
          <div>
            <p className="text-sm font-medium text-muted-foreground mb-2">
              Session Statistics
            </p>
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
                    <TableCell className="whitespace-nowrap">
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
                    <TableCell className="whitespace-nowrap">
                      <CustomRelativeTimeLabel
                        timestamp={hasSession.message_last_received_at_ms}
                      />
                    </TableCell>
                    <TableCell>
                      {hasSession.messages_sent.toLocaleString()}
                    </TableCell>
                    <TableCell>
                      {(hasSession.bytes_sent / 1024).toFixed(2)} KB
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      <CustomRelativeTimeLabel
                        timestamp={hasSession.message_last_sent_at_ms}
                      />
                    </TableCell>
                  </TableRow>
                </TableBody>
              </Table>
            </div>
          </div>
        ) : (
          <div>
            <p className="text-sm font-medium text-muted-foreground mb-2">
              Session Statistics
            </p>
            <p className="text-sm text-muted-foreground">No active session</p>
          </div>
        )}

        {/* Time Window Statistics */}
        {worker.time_windowed_stats ? (
          <TimeWindowStats stats={worker.time_windowed_stats} />
        ) : null}
      </div>
    </div>
  );
};

// Export memoized component
export const WorkerDetails = memo(WorkerDetailsComponent);
