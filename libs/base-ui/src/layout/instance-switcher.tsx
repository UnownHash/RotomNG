import { Check, Database, Server } from "lucide-react";
import type { FC } from "react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { instanceLabel, useInstances } from "@/hooks/use-instances";
import { cn } from "@/lib/utils";
import { Button } from "../components/ui/button";

export interface InstanceSwitcherProps {
  /** Renders full width with a visible label, for the mobile drawer. */
  block?: boolean;
  /** Called after a selection is made, so the drawer can close itself. */
  onSelect?: () => void;
}

/**
 * Picks which rotom-ng the UI is pointed at.
 *
 * Renders nothing outside multi-instance mode, so the header is unchanged when
 * the UI is served by a rotom-ng directly.
 *
 * Unreachable instances stay listed but are not selectable: hiding them would
 * make an instance silently vanish from the picker whenever it restarted, and
 * leaving them selectable would put the operator on a server that cannot
 * answer. Listed-but-disabled says "it exists, it's down" in one glance.
 */
export const InstanceSwitcher: FC<InstanceSwitcherProps> = ({
  block,
  onSelect,
}) => {
  const { multiInstance, instances, selected, select } = useInstances();

  if (!multiInstance) {
    return null;
  }

  const currentLabel = selected ? instanceLabel(selected) : "No instance";

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size={block ? "default" : "sm"}
          aria-label="Select instance"
          className={cn(
            "gap-2 text-muted-foreground hover:text-foreground",
            block && "w-full justify-start",
          )}
        >
          <Database className="size-4 shrink-0" />
          <span
            className="max-w-40 truncate text-sm font-medium"
            title={currentLabel}
          >
            {currentLabel}
          </span>
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56 max-w-80">
        <DropdownMenuLabel>Instances</DropdownMenuLabel>
        {instances.length === 0 ? (
          <div className="px-2 py-1.5 text-sm text-muted-foreground">
            None configured
          </div>
        ) : (
          instances.map((instance) => {
            const label = instanceLabel(instance);
            const isCurrent = instance.url === selected?.url;
            return (
              <DropdownMenuItem
                key={instance.url}
                disabled={!instance.reachable}
                onSelect={() => {
                  select(instance.url);
                  onSelect?.();
                }}
              >
                <Server
                  className={cn(
                    "size-4 shrink-0",
                    instance.reachable ? "text-(--brand)" : "text-destructive",
                  )}
                />
                <span className="min-w-0 flex-1 truncate" title={instance.url}>
                  {label}
                  {instance.reachable ? null : (
                    <span className="ml-2 text-xs text-muted-foreground">
                      unreachable
                    </span>
                  )}
                </span>
                <Check className={cn("ml-auto", !isCurrent && "hidden")} />
              </DropdownMenuItem>
            );
          })
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
};
