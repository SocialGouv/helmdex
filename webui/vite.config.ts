import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev: the Go API runs separately (`task ui:serve` or `helmdex ui`) and the
// Vite dev server proxies /api to it. Build: assets are copied into
// internal/server/static and embedded in the binary.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5197,
    proxy: {
      "/api": {
        target: process.env.HELMDEX_API ?? "http://127.0.0.1:8117",
        changeOrigin: true,
      },
    },
  },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
  },
});
