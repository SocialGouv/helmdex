import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClientProvider } from "@tanstack/react-query";
import App from "./App";
import DesktopShell from "./components/DesktopShell";
import { isDesktop } from "./lib/desktop";
import { makeQueryClient } from "./lib/queryClient";
import { initTheme } from "./lib/theme";
import "./lib/monaco";
import "./index.css";

initTheme();

// Desktop: the shell owns per-workspace query clients and the folder rail.
// Browser (`helmdex ui`): a single workspace, a single client.
createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {isDesktop() ? (
      <DesktopShell />
    ) : (
      <QueryClientProvider client={makeQueryClient()}>
        <App />
      </QueryClientProvider>
    )}
  </StrictMode>,
);
