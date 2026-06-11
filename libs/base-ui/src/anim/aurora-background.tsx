import type { FC, ReactNode } from "react";
import { cn } from "../lib/utils";

interface AuroraBackgroundProps {
  children?: ReactNode;
  className?: string;
}

export const AuroraBackground: FC<AuroraBackgroundProps> = ({
  children,
  className,
}) => <div className={cn("aurora-bg min-h-screen", className)}>{children}</div>;
