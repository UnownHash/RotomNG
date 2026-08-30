import { ServerCrash, ServerOff } from "lucide-react";
import type { FC, ReactNode } from "react";
import {
  type InstancesState,
  instanceLabel,
  useInstances,
} from "@/hooks/use-instances";

export interface InstanceGateProps {
  children: ReactNode;
}

/** What the gate should show. See resolveInstanceGate. */
export type InstanceGateState =
  | { kind: "ready" }
  | { kind: "pending" }
  | { kind: "none-configured" }
  | { kind: "selected-unreachable"; label: string };

/** The fields the decision depends on. */
type GateInput = Pick<
  InstancesState,
  "multiInstance" | "instances" | "selected" | "selectedUnreachable"
>;

/**
 * Decides what the gate shows, in priority order. Pure so the ordering is
 * testable -- it is the whole substance of this component, and getting it wrong
 * produces a message that is true but misleading rather than an obvious break.
 *
 * There is no separate "none of them are reachable" state. A selection is
 * always adopted when there is one to be had, so an all-down fleet is just the
 * selected instance being down, said once about the instance the operator is
 * actually on.
 */
export const resolveInstanceGate = (state: GateInput): InstanceGateState => {
  if (!state.multiInstance) {
    return { kind: "ready" };
  }
  if (state.instances.length === 0) {
    return { kind: "none-configured" };
  }
  if (state.selected === null) {
    // Something is reachable but nothing is chosen yet: the frame between the
    // config landing and the effect that adopts a selection. Transient, so it
    // gets no message -- one would only ever be seen as a flash.
    return { kind: "pending" };
  }
  if (state.selectedUnreachable) {
    return {
      kind: "selected-unreachable",
      label: instanceLabel(state.selected),
    };
  }
  return { kind: "ready" };
};

/**
 * Holds back the pages when the selected instance cannot answer for them.
 *
 * Every page polls `/api/status` and friends, which the admin service can only
 * answer by asking an instance. Without this the operator would get a wall of
 * error toasts from queries that never had a chance; a single sentence saying
 * what is wrong is both truer and quieter.
 *
 * Rendered inside `Layout`, so the instance picker stays reachable — an
 * operator whose instance went down can still switch to one that is up.
 *
 * Passes children straight through outside multi-instance mode.
 */
export const InstanceGate: FC<InstanceGateProps> = ({ children }) => {
  const gate = resolveInstanceGate(useInstances());

  switch (gate.kind) {
    case "ready":
      return <>{children}</>;
    case "pending":
      return null;
    case "none-configured":
      return (
        <GateMessage
          icon={<ServerOff className="size-8" />}
          title="No instances configured"
          detail="Add one or more [[instances]] entries to this service's config, then reload it."
        />
      );
    case "selected-unreachable":
      return (
        <GateMessage
          icon={<ServerCrash className="size-8" />}
          title="Current instance not reachable"
          detail={`${gate.label} is not responding. It will come back on its own once it does; you can also pick another instance from the header.`}
        />
      );
  }
};

interface GateMessageProps {
  icon: ReactNode;
  title: string;
  detail: string;
}

const GateMessage: FC<GateMessageProps> = ({ icon, title, detail }) => (
  <div className="flex min-h-[60vh] flex-col items-center justify-center gap-3 text-center">
    <div className="text-muted-foreground">{icon}</div>
    <h2 className="text-lg font-semibold text-foreground">{title}</h2>
    <p className="max-w-md text-sm text-muted-foreground">{detail}</p>
  </div>
);
