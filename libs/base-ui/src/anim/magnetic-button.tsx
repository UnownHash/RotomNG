import type { ComponentProps, ReactNode } from "react";
import { Button } from "../components/ui/button";
import { cn } from "../lib/utils";

type MagneticButtonProps = ComponentProps<typeof Button> & {
  children?: ReactNode;
};

/**
 * Slight lift on hover. Originally a 3D-tilt magnetic effect — toned down to
 * a plain shadcn Button with the `lift` utility for a subtle hover transform.
 * Kept as a separate export so callers can opt back in later.
 */
export const MagneticButton = ({
  children,
  className,
  ...props
}: MagneticButtonProps) => (
  <Button className={cn("lift bg-sky-300/80", className)} {...props}>
    {children}
  </Button>
);
