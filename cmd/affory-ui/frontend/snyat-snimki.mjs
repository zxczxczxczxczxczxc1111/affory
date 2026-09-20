// Снимки README: те же экраны, что уезжают в выпуск, но с показательными
// данными из заглушки стенда (vite.stend.config.ts).
//
// Запуск: сначала `npx vite --config vite.stend.config.ts`, затем
// `node snyat-snimki.mjs`. Своего браузера Playwright не тянет, берётся
// пользовательский Chrome через channel.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

const BAZA = "http://127.0.0.1:9245";
const KUDA = fileURLToPath(new URL("../../../assets/screenshots/", import.meta.url));

// 1180x860 при масштабе 1.5 даёт 1770x1290: размер прежних снимков README, и
// менять его нельзя без пересъёмки всех сразу, иначе в таблице поедет вёрстка.
const OKNO = { width: 1180, height: 860 };
const MASHTAB = 1.5;

// Вкладка «Приложения» выбирается нажатием, а не адресом: подпись под снимком
// в README обещает правила приложений, и кадр обязан показывать именно их.
// Прежний снимок показывал вкладку сервисов при подписи «Правила приложений».
const KADRY = [
  { imya: "connection.jpg", ekran: "podklyuchenie" },
  { imya: "applications.jpg", ekran: "pravila", nazhat: "Приложения" },
  { imya: "settings.jpg", ekran: "nastroyki" },
];

mkdirSync(KUDA, { recursive: true });

const browser = await chromium.launch({ channel: "chrome" });
const context = await browser.newContext({ viewport: OKNO, deviceScaleFactor: MASHTAB });
const page = await context.newPage();

for (const kadr of KADRY) {
  await page.goto(`${BAZA}/?ekran=${kadr.ekran}`, { waitUntil: "networkidle" });
  if (kadr.nazhat) {
    // По роли, а не по тексту: подпись вкладки лежит в одном узле со счётчиком
    // правил, и точный поиск текста её не находит вовсе.
    await page.getByRole("tab", { name: kadr.nazhat }).click();
    await page.waitForTimeout(400);
  }
  // Экран настроек длиннее окна и после перехода приезжает ПРОКРУЧЕННЫМ:
  // снимок начинается с середины, шапки нет. Одного window.scrollTo мало,
  // прокручен внутренний узел, поэтому гасятся все сразу.
  await page.evaluate(() => {
    window.scrollTo(0, 0);
    document.querySelectorAll("*").forEach((el) => {
      if (el.scrollTop) el.scrollTop = 0;
    });
  });
  // Сфера на главном экране анимирована, а замеры приезжают отдельным кадром.
  await page.waitForTimeout(1200);
  await page.screenshot({ path: KUDA + kadr.imya, type: "jpeg", quality: 92 });
  console.log(`снят ${kadr.imya} (${kadr.ekran})`);
}

await browser.close();
