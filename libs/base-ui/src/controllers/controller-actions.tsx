import {
  Unlink as LinkOffIcon,
  MoreVertical as MoreVertIcon,
  RefreshCw as RefreshIcon,
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

interface ControllerActionsProps {
  controllerId: string;
  controllerUuid: string;
  onAction: (
    controllerUuid: string,
    action: "disconnect" | "reconnect",
  ) => void;
}

const ControllerActionsComponent: React.FC<ControllerActionsProps> = ({
  controllerId,
  controllerUuid,
  onAction,
}) => {
  const [confirmationDialog, setConfirmationDialog] = useState<{
    open: boolean;
    action: "disconnect" | "reconnect" | null;
    title: string;
    message: string;
  }>({
    open: false,
    action: null,
    title: "",
    message: "",
  });

  const handleAction = useCallback(
    (action: "disconnect" | "reconnect") => {
      const messages = {
        disconnect: `Disconnect controller '${controllerId}'?`,
        reconnect: `Tell controller '${controllerId}' to reconnect?`,
      };

      setConfirmationDialog({
        open: true,
        action,
        title: "Confirm Action",
        message: messages[action],
      });
    },
    [controllerId],
  );

  const handleConfirmAction = useCallback(() => {
    if (confirmationDialog.action) {
      onAction(controllerUuid, confirmationDialog.action);
    }
    setConfirmationDialog({
      open: false,
      action: null,
      title: "",
      message: "",
    });
  }, [confirmationDialog.action, controllerUuid, onAction]);

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
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="controller actions"
          >
            <MoreVertIcon className="size-4" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuItem onClick={() => handleAction("reconnect")}>
            <RefreshIcon className="size-4" />
            Reconnect
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => handleAction("disconnect")}>
            <LinkOffIcon className="size-4" />
            Disconnect
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
export const ControllerActions = memo(ControllerActionsComponent);
