import { AnimatePresence, motion } from "motion/react";
import type { ReactNode } from "react";

interface CollapseProps {
  open: boolean;
  children: ReactNode;
}

/**
 * Smooth height-animated collapse. Wrap conditional content (e.g., expanded
 * `<CardContent>`) so toggling the open state slides in/out instead of
 * snapping. Children are unmounted when closed.
 */
export const Collapse = ({ open, children }: CollapseProps) => (
  <AnimatePresence initial={false}>
    {open && (
      <motion.div
        initial={{ height: 0, opacity: 0 }}
        animate={{ height: "auto", opacity: 1 }}
        exit={{ height: 0, opacity: 0 }}
        transition={{ duration: 0.25, ease: [0.16, 1, 0.3, 1] }}
        style={{ overflow: "hidden" }}
      >
        {children}
      </motion.div>
    )}
  </AnimatePresence>
);
