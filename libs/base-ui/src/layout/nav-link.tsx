import type { FC, MouseEvent as ReactMouseEvent, ReactNode } from "react";
import { useHref, useLinkClickHandler, useLocation } from "react-router";
import { cn } from "@/lib/utils";

interface NavLinkProps {
  to: string;
  children?: ReactNode;
  onClick?: () => void;
  /** Block layout for mobile drawer; inline for top nav. */
  block?: boolean;
}

export const NavLink: FC<NavLinkProps> = ({ to, children, onClick, block }) => {
  const href = useHref(to);
  const handleClick = useLinkClickHandler(to);
  const location = useLocation();
  const isActive = location.pathname === to;

  return (
    <a
      href={href}
      onClick={(event) => {
        handleClick(event as unknown as ReactMouseEvent<HTMLAnchorElement>);
        onClick?.();
      }}
      className={cn(
        "relative rounded-md px-3 py-2 text-sm font-medium transition-all duration-200",
        "hover:text-foreground",
        block && "block w-full",
        isActive
          ? "text-foreground bg-muted shadow-[inset_0_-2px_0_var(--brand)]"
          : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
      )}
    >
      {children}
    </a>
  );
};
