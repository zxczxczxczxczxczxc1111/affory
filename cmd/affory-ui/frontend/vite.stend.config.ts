import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

// Конфиг ТОЛЬКО для снимков README. Отличие от рабочего одно: вместо рантайма
// Wails подставляется заглушка с показательными данными, поэтому окно
// рисуется тем же кодом, что уезжает в выпуск.
export default defineConfig({
  server: { host: "127.0.0.1", port: 9245, strictPort: true },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@wailsio/runtime": fileURLToPath(new URL("./src/stend/wails-zaglushka.ts", import.meta.url)),
    },
  },
});
