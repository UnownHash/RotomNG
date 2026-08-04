import "@rotom-ng/base-ui/styles/globals.css";

import { StrictMode } from "react";
import * as ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router";

import { App } from "./app/app";

async function enableMocks() {
  if (!import.meta.env.DEV) return;
  if (import.meta.env.VITE_MOCK !== "true") return;
  const { worker } = await import("@rotom-ng/base-ui/mocks");
  await worker.start({ onUnhandledRequest: "bypass" });
}

function renderApp() {
  const root = ReactDOM.createRoot(
    document.getElementById("root") as HTMLElement,
  );
  root.render(
    <StrictMode>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </StrictMode>,
  );
}

enableMocks().then(renderApp, (err) => {
  // Never let an MSW boot failure swallow the React mount — surface the
  // error in the console and render the app against the real backend.
  console.error("[mocks] failed to start MSW worker:", err);
  renderApp();
});
