import type React from "react";
import { memo } from "react";
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
import { TimestampWithRelative } from "../time-label";
import type { Controller } from "../types/connections";

interface ControllerDetailsProps {
  controller: Controller;
}

const ControllerDetailsComponent: React.FC<ControllerDetailsProps> = ({
  controller,
}) => {
  return (
    <div className="rounded-lg border border-border/40 bg-card/40 backdrop-blur-xl p-4 m-2">
      <div className="flex flex-col gap-6">
        {/* UUID Display */}
        <div>
          <p className="text-sm">UUID: {controller.uuid}</p>
          <p className="text-sm">
            Protocol: v{controller.proto_major_version}.
            {controller.proto_minor_version}
          </p>
          <p className="text-sm">
            Account: {controller.account_username} ({controller.account_source})
          </p>
        </div>

        {/* Session Statistics Table */}
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
                    <TimestampWithRelative
                      timestamp={controller.connected_at_ms}
                    />
                  </TableCell>
                  <TableCell>
                    {controller.messages_received.toLocaleString()}
                  </TableCell>
                  <TableCell>
                    {(controller.bytes_received / 1024).toFixed(2)} KB
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    <TimestampWithRelative
                      timestamp={controller.message_last_received_at_ms}
                    />
                  </TableCell>
                  <TableCell>
                    {controller.messages_sent.toLocaleString()}
                  </TableCell>
                  <TableCell>
                    {(controller.bytes_sent / 1024).toFixed(2)} KB
                  </TableCell>
                  <TableCell className="whitespace-nowrap">
                    <TimestampWithRelative
                      timestamp={controller.message_last_sent_at_ms}
                    />
                  </TableCell>
                </TableRow>
              </TableBody>
            </Table>
          </div>
        </div>
      </div>
    </div>
  );
};

// Export memoized component
export const ControllerDetails = memo(ControllerDetailsComponent);
