import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

// Конфиг ТОЛЬКО для снимков README. Отличие от рабочего одно: вместо рантайма
// Wails подставляется заглушка с показательными данными, поэтому окно
// рисуется тем же кодом, что уезжает в выпуск.

// Номер версии берётся из файла VERSIYA, а не пишется в заглушке числом.
// Прибитый номер прожил ровно до следующего выпуска: снимки README, сделанные
// 16.09.2026, показывали 1.2.0 ещё в 1.3.2, и заметить это можно было только
// глазами на картинке. Снимок, который врёт о версии, хуже отсутствующего.
const versiya = readFileSync(
  fileURLToPath(new URL("../../../VERSIYA", import.meta.url)), "utf8").trim();

export default defineConfig({
  server: { host: "127.0.0.1", port: 9245, strictPort: true },
  plugins: [react(), tailwindcss()],
  define: { __VERSIYA_STENDA__: JSON.stringify(versiya) },
  resolve: {
    alias: {
      "@wailsio/runtime": fileURLToPath(new URL("./src/stend/wails-zaglushka.ts", import.meta.url)),
    },
  },
});
