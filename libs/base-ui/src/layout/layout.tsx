import { LogOut, Menu } from "lucide-react";
import { motion } from "motion/react";
import { type FC, type ReactNode, useState } from "react";
import { useAuthOptional } from "@/auth/auth-context";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTrigger } from "@/components/ui/sheet";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { useInstanceLabel } from "@/hooks/use-instances";
import { cn } from "@/lib/utils";
import { InstanceSwitcher } from "./instance-switcher";
import { NavLink } from "./nav-link";

export interface NavItem {
  label: string;
  path: string;
}

export interface LayoutProps {
  children?: ReactNode;
  appName: string;
  appIcon: string;
  appVersion?: string;
  navItems: NavItem[];
}

export const Layout: FC<LayoutProps> = ({
  children,
  appName,
  appIcon,
  appVersion,
  navItems,
}) => {
  const [drawerOpen, setDrawerOpen] = useState(false);
  // Null when the app has not adopted AuthGate; false when no api secret is
  // configured. Either way there is no session to end, so no button.
  const auth = useAuthOptional();
  const showLogout = auth?.authRequired ?? false;
  const instanceName = useInstanceLabel();

  return (
    <div
      className={cn(
        "min-h-screen flex items-center flex-col bg-app text-foreground",
      )}
    >
      <motion.header
        initial={{ y: -20, opacity: 0 }}
        animate={{ y: 0, opacity: 1 }}
        transition={{ duration: 0.4, ease: [0.16, 1, 0.3, 1] }}
        className="sticky w-full top-0 z-40 flex h-14 items-center justify-between border-b border-border/60 bg-background/80 px-4 backdrop-blur"
      >
        <div className="flex min-w-0 items-center gap-2">
          <img src={appIcon} alt={appName} className="size-8 shrink-0" />
          <div className="flex min-w-0 flex-col leading-tight">
            <span className="text-base font-bold gradient-text">
              {appName}
              {appVersion ? ` v${appVersion}` : ""}
            </span>
            {instanceName ? (
              // Which server the page is actually about. Fronted by the admin
              // service this follows the selection, so it doubles as
              // confirmation that a switch took effect.
              <span
                className="max-w-48 truncate text-xs text-muted-foreground sm:max-w-xs"
                title={instanceName}
              >
                Instance: {instanceName}
              </span>
            ) : null}
          </div>
        </div>

        <nav className="hidden md:flex items-center gap-1">
          {navItems.map((item) => (
            <NavLink key={item.path} to={item.path}>
              {item.label}
            </NavLink>
          ))}
          {/* Renders nothing unless the UI is fronted by the admin service. */}
          <InstanceSwitcher />
          {showLogout ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label="Sign out"
                  disabled={auth?.loggingOut}
                  onClick={() => auth?.logout()}
                >
                  <LogOut className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Sign out</TooltipContent>
            </Tooltip>
          ) : null}
        </nav>

        <Sheet open={drawerOpen} onOpenChange={setDrawerOpen}>
          <SheetTrigger asChild>
            <Button variant="ghost" size="icon" className="md:hidden">
              <Menu className="size-5" />
            </Button>
          </SheetTrigger>
          <SheetContent
            side="right"
            className="w-64 bg-card/95 backdrop-blur-xl"
          >
            <nav className="flex flex-col gap-1 pt-8">
              {navItems.map((item) => (
                <NavLink
                  key={item.path}
                  to={item.path}
                  onClick={() => setDrawerOpen(false)}
                  block
                >
                  {item.label}
                </NavLink>
              ))}
              <InstanceSwitcher block onSelect={() => setDrawerOpen(false)} />
              {showLogout ? (
                <Button
                  variant="ghost"
                  className="justify-start gap-2"
                  disabled={auth?.loggingOut}
                  onClick={() => {
                    setDrawerOpen(false);
                    auth?.logout();
                  }}
                >
                  <LogOut className="size-4" />
                  Sign out
                </Button>
              ) : null}
            </nav>
          </SheetContent>
        </Sheet>
      </motion.header>

      <main className="flex-1 p-4 md:p-6 max-w-full w-full">{children}</main>
    </div>
  );
};
