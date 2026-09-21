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

// Вкладки правил выбираются нажатием, а не адресом: подпись под снимком в
// README обещает конкретную вкладку, и кадр обязан показывать именно её.
// Прежний снимок показывал вкладку сервисов при подписи «Правила приложений».
//
// Снимков стало семь (21.09.2026, по просьбе владельца «скрины всех экранов»):
// до этого README показывал три из шести экранов, и «Управлять» - то место,
// куда человек идёт первым делом, - не был виден вовсе.
const KADRY = [
  { imya: "connection.jpg", ekran: "podklyuchenie" },
  // Замеры нажимаются: без них список стоит без задержек, а это половина
  // смысла экрана.
  { imya: "servers.jpg", ekran: "servery", otkryt: "zamerit-zaderzhki", vysota: 640 },
  { imya: "services.jpg", ekran: "pravila", nazhat: "Сервисы" },
  { imya: "applications.jpg", ekran: "pravila", nazhat: "Приложения" },
  { imya: "sites.jpg", ekran: "pravila", nazhat: "Сайты", vysota: 640 },
  { imya: "settings.jpg", ekran: "nastroyki" },
  // Справка это окно поверх экрана подключения, и открывается оно значком
  // вопроса у заголовка «Серверы».
  { imya: "protocols.jpg", ekran: "podklyuchenie", otkryt: "spravka-protokolov-otkryt" },
];

mkdirSync(KUDA, { recursive: true });

const browser = await chromium.launch({ channel: "chrome" });
const context = await browser.newContext({ viewport: OKNO, deviceScaleFactor: MASHTAB });
const page = await context.newPage();

for (const kadr of KADRY) {
  // Своя высота у кадров, чей экран короче окна: у списка серверов и правил
  // сайтов нижняя треть иначе уходит пустым полем, и снимок читается как
  // незагрузившийся.
  await page.setViewportSize({ width: OKNO.width, height: kadr.vysota ?? OKNO.height });
  await page.goto(`${BAZA}/?ekran=${kadr.ekran}`, { waitUntil: "networkidle" });
  if (kadr.nazhat) {
    // По роли, а не по тексту: подпись вкладки лежит в одном узле со счётчиком
    // правил, и точный поиск текста её не находит вовсе.
    await page.getByRole("tab", { name: kadr.nazhat }).click();
    await page.waitForTimeout(400);
  }
  if (kadr.otkryt) {
    await page.getByTestId(kadr.otkryt).click();
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
