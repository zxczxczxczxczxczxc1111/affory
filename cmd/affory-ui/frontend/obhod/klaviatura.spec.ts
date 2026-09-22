import { expect, test, type Page } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Клавиатурная приёмка правил (C9, 22.09.2026).
//
// Почему настоящий браузер, а не проверки рядом с кодом. Обе вещи, которые здесь
// проверяются, jsdom не воспроизводит вовсе:
//
//   - Enter в поле отправляет форму по правилам браузера (implicit
//     submission), а не по нашему обработчику: jsdom его не реализует, и
//     `fireEvent.submit` проверяет обработчик, а не клавишу;
//   - порядок Tab и фактический фокус после перерисовки: jsdom считает
//     tabindex, но ходить по нему не умеет.
//
// Мост подставной, службы нет: проверяется ровно интерфейс, как и в соседних
// файлах обхода.

/** Метка того, что сейчас в фокусе: имя для чтения с экрана или видимый текст. */
async function vFokuse(page: Page): Promise<string> {
  return await page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el || el === document.body) return "";
    return (el.getAttribute("aria-label") ?? el.innerText ?? "").trim();
  });
}

test("стрелки ходят по вкладкам правил, а Tab уводит с полосы к содержимому", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=pravila");

  const vkladka = (imya: string) => page.getByRole("tab", { name: new RegExp(`^${imya}`) });
  await vkladka("Сервисы").click();
  await expect(vkladka("Сервисы")).toHaveAttribute("aria-selected", "true");

  await page.keyboard.press("ArrowRight");
  await expect(vkladka("Приложения")).toHaveAttribute("aria-selected", "true");
  expect(await vFokuse(page)).toMatch(/^Приложения/);

  await page.keyboard.press("End");
  await expect(vkladka("Сайты")).toHaveAttribute("aria-selected", "true");
  // Круг: с последней вкладки шаг вправо возвращает к первой, а не упирается.
  await page.keyboard.press("ArrowRight");
  await expect(vkladka("Сервисы")).toHaveAttribute("aria-selected", "true");

  // Из полосы Tab уходит к содержимому: до 22.09.2026 в общий порядок
  // попадала каждая вкладка, и дойти до правил стоило трёх лишних нажатий.
  await page.keyboard.press("Tab");
  expect(await vFokuse(page)).not.toMatch(/^(Приложения|Сайты)/);

  await nichegoNeSlomalos(page, oshibki);
});

test("правило сайта заводится с клавиатуры: курсор в поле, Enter в черновик, фокус назад", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=pravila");

  await page.getByRole("tab", { name: /^Сайты/ }).click();
  await page.getByRole("button", { name: "Добавить" }).click();
  // Курсор сразу в поле: иначе после нажатия «Добавить» до него надо было
  // дойти табом через всю полосу вкладок.
  expect(await vFokuse(page)).toBe("Домен сайта");

  await page.keyboard.type("proba-klaviatury.example");
  await page.keyboard.press("Enter");

  await expect(page.getByLabel("Домен сайта")).toBeHidden();
  await expect(page.getByRole("button", { name: "Удалить правило proba-klaviatury.example" })).toBeVisible();
  // Форма закрылась вместе с полем, в котором стоял курсор; фокус вернулся на
  // кнопку, а не упал на body.
  expect(await vFokuse(page)).toBe("Добавить");

  // Та же строка убирается с клавиатуры, и фокус снова не теряется.
  await page.getByRole("button", { name: "Удалить правило proba-klaviatury.example" }).focus();
  await page.keyboard.press("Enter");
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: /proba-klaviatury/ })).toHaveCount(0);
  expect(await vFokuse(page)).toBe("Добавить");

  // Добавленное и тут же удалённое правило возвращает набор к исходному, и
  // полоса черновика уходит сама: стенд остался таким, каким был, применять
  // подставному мосту нечего.
  await expect(page.getByRole("button", { name: "Применить изменения" })).toHaveCount(0);
  await nichegoNeSlomalos(page, oshibki);
});

test("строки приложений называют своё имя в подписи флажка и удаления", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=pravila");

  await page.getByRole("tab", { name: /^Приложения/ }).click();
  // Подписи в столбцах одинаковы у всех строк, поэтому голосом строки
  // различаются только именем в доступном имени элемента.
  const flazhki = await page.getByRole("checkbox").all();
  const imena = await Promise.all(flazhki.map((f) => f.getAttribute("aria-label")));
  const svoi = imena.filter((i): i is string => !!i && i.startsWith("И программы, запущенные "));
  expect(svoi.length).toBeGreaterThan(1);
  expect(new Set(svoi).size).toBe(svoi.length);

  const udalit = await page.getByRole("button", { name: /^Удалить правило / }).all();
  const podpisi = await Promise.all(udalit.map((k) => k.getAttribute("aria-label")));
  expect(podpisi.length).toBeGreaterThan(1);
  expect(new Set(podpisi).size).toBe(podpisi.length);

  await nichegoNeSlomalos(page, oshibki);
});
