import { delay, HttpResponse, http } from "msw";
import {
  appendJobInstance,
  applyLiveJitter,
  buildConfigResponse,
  clearJobInstances,
  mockState,
  removeDeadDevices,
  removeDevice,
  setDeviceConnected,
} from "./state";

export const handlers = [
  http.get("/api/config", () => HttpResponse.json(buildConfigResponse())),

  http.get("/api/status", () => {
    applyLiveJitter();
    return HttpResponse.json({
      devices: mockState.devices,
      controllers: mockState.controllers,
    });
  }),

  http.get("/api/job", () => HttpResponse.json({ jobs: mockState.jobs })),

  http.get("/api/job-instance", () =>
    HttpResponse.json({ instances: mockState.instances }),
  ),

  http.put("/api/job/-/reload", async () => {
    await delay(300);
    return HttpResponse.json({ ok: true });
  }),

  http.put("/api/job-instance/-/clear", async () => {
    await delay(150);
    clearJobInstances();
    return HttpResponse.json({ ok: true });
  }),

  http.put("/api/job/:jobId/run", async ({ params }) => {
    await delay(200);
    const instance = appendJobInstance(String(params["jobId"]));
    return HttpResponse.json({ ok: true, instance });
  }),

  http.put("/api/device/_/action/delete", async () => {
    await delay(200);
    const devicesRemoved = removeDeadDevices();
    return HttpResponse.json({
      status: "ok",
      message: "Removed dead connections",
      devices_count: devicesRemoved,
    });
  }),

  http.put("/api/device/:deviceId/action/:action", async ({ params }) => {
    await delay(150);
    const id = String(params["deviceId"]);
    const action = String(params["action"]);
    switch (action) {
      case "delete":
        removeDevice(id);
        break;
      case "disconnect":
        setDeviceConnected(id, false);
        break;
      // reboot / restart / logcat are no-op success.
    }
    return HttpResponse.json({ ok: true });
  }),
];
