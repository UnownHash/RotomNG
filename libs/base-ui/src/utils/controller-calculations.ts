import type { Controller } from "../types";

export interface ControllerMetrics {
  totalControllers: number;
}

export const calculateControllerMetrics = (
  controllers: Controller[],
): ControllerMetrics => {
  const totalControllers = controllers.length;
  return { totalControllers };
};
