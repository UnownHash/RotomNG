import type { Controller, Device } from "./connections";

// New status structure matching the server API
export interface Status {
  devices: Device[];
  controllers: Controller[];
}
