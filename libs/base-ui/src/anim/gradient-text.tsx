import type { ElementType, ReactNode } from "react";
import { cn } from "../lib/utils";

interface GradientTextProps {
  as?: ElementType;
  className?: string;
  children?: ReactNode;
}

export const GradientText = ({
  as: Tag = "span",
  className,
  children,
}: GradientTextProps) => (
  <Tag className={cn("gradient-text", className)}>{children}</Tag>
);
