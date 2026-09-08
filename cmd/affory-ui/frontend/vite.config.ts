import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// No @wailsio/runtime/plugins/vite here, and that is a decision, not an
// omission: the plugin exists to serve GENERATED bindings, and it fails the
// build outright when they are absent. Our bridge is one Call.ByName in
// most.ts, so there is nothing to generate and nothing for the plugin to do.
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), tailwindcss()],
});
