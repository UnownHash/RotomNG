import type { Device } from "../types";

export interface DeviceMetrics {
  enabledDevices: number;
  connectedDevices: number;
  totalDevices: number;
  inUseDevices: number;
}

export const calculateDeviceMetrics = (devices: Device[]): DeviceMetrics => {
  const enabledDevices = devices.filter((device) => device.can_be_used).length;
  const connectedDevices = devices.filter(
    (device) => device.is_connected,
  ).length;
  const totalDevices = devices.length;
  const inUseDevices = devices.filter((device) => device.is_in_use).length;

  return {
    enabledDevices,
    connectedDevices,
    totalDevices,
    inUseDevices,
  };
};
