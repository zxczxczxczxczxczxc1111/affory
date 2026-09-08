import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Separate from vite.config.ts on purpose: the app build needs the Wails
// plugin, which only gets in the way here because bindings are mocked in
// tests, not served from disk.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    // jest-dom matchers (toHaveTextContent and friends). Without this line
    // the first screen test dies with "not a function" and blames the screen.
    setupFiles: ["src/setup-testov.ts"],
  },
});
