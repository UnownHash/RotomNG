import type { FC, HTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/utils";

interface BoxProps extends HTMLAttributes<HTMLDivElement> {
  children?: ReactNode;
}

export const Box: FC<BoxProps> = ({ className, children, ...rest }) => (
  <div className={cn("box-border relative max-w-full", className)} {...rest}>
    {children}
  </div>
);
