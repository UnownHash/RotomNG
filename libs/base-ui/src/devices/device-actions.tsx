import {
  Bug as BugReportIcon,
  Trash2 as DeleteIcon,
  Unlink as LinkOffIcon,
  MoreVertical as MoreVertIcon,
  RefreshCw as RefreshIcon,
  Power as RestartAltIcon,
} from "lucide-react";
import type React from "react";
import { memo, useCallback, useState } from "react";
import { ConfirmationDialog } from "../components/confirmation-dialog";
import { Button } from "../components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "../components/ui/dropdown-menu";

interface DeviceActionsProps {
  deviceId: string;
  isConnected: boolean;
  onAction: (
    deviceId: string,
    action: "reboot" | "restart" | "logcat" | "delete" | "disconnect",
  ) => void;
}

const DeviceActionsComponent: React.FC<DeviceActionsProps> = ({
  deviceId,
  isConnected,
  onAction,
}) => {
  const [confirmationDialog, setConfirmationDialog] = useState<{
    open: boolean;
    action: "reboot" | "restart" | "disconnect" | null;
    title: string;
    message: string;
  }>({
    open: false,
    action: null,
    title: "",
    message: "",
  });

  const handleAction = useCallback(
    (action: "reboot" | "restart" | "logcat" | "delete" | "disconnect") => {
      // Don't allow connection-dependent actions when device is not connected
      if (
        !isConnected &&
        (action === "restart" ||
          action === "reboot" ||
          action === "logcat" ||
          action === "disconnect")
      ) {
        return;
      }
      // Don't allow delete when device is connected
      if (isConnected && action === "delete") {
        return;
      }

      // Actions that need confirmation
      if (
        action === "reboot" ||
        action === "restart" ||
        action === "disconnect"
      ) {
        const actionMessages = {
          reboot: `Reboot device '${deviceId}'?`,
          restart: `Restart device '${deviceId}'?`,
          disconnect: `Disconnect device '${deviceId}'?`,
        };

        setConfirmationDialog({
          open: true,
          action,
          title: "Confirm Action",
          message: actionMessages[action],
        });
        return;
      }

      // Execute action directly for non-confirmation actions
      onAction(deviceId, action);
    },
    [deviceId, isConnected, onAction],
  );

  const handleConfirmAction = useCallback(() => {
    if (confirmationDialog.action) {
      onAction(deviceId, confirmationDialog.action);
    }
    setConfirmationDialog({
      open: false,
      action: null,
      title: "",
      message: "",
    });
  }, [confirmationDialog.action, deviceId, onAction]);

  const handleCancelAction = useCallback(() => {
    setConfirmationDialog({
      open: false,
      action: null,
      title: "",
      message: "",
    });
  }, []);

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon-sm" aria-label="device actions">
            <MoreVertIcon className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem
            onClick={() => handleAction("restart")}
            disabled={!isConnected}
          >
            <RefreshIcon className="size-4" />
            Restart
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => handleAction("reboot")}
            disabled={!isConnected}
          >
            <RestartAltIcon className="size-4" />
            Reboot
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => handleAction("logcat")}
            disabled={!isConnected}
          >
            <BugReportIcon className="size-4" />
            Logcat
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => handleAction("disconnect")}
            disabled={!isConnected}
          >
            <LinkOffIcon className="size-4" />
            Disconnect
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => handleAction("delete")}
            disabled={isConnected}
          >
            <DeleteIcon className="size-4" />
            Delete
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <ConfirmationDialog
        open={confirmationDialog.open}
        title={confirmationDialog.title}
        message={confirmationDialog.message}
        onConfirm={handleConfirmAction}
        onCancel={handleCancelAction}
      />
    </>
  );
};

// Export memoized component
export const DeviceActions = memo(DeviceActionsComponent);
