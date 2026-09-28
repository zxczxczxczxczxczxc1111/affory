import { expect, test } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Вкладка «Реклама» в настоящем браузере на минимальном окне (28.09.2026).
//
// Поверх проверок рядом с кодом здесь две вещи, которых jsdom не видит:
// вёрстка на 900x600, где на содержимое остаётся около 548 точек, и путь
// «черновик, применение, повторное открытие» через подставной мост, то есть
// тем же кодом, что уезжает в выпуск.

test("реклама включается, меняет уровень, берёт исключение и переживает применение", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.setViewportSize({ width: 900, height: 600 });
  await page.goto("/?ekran=pravila");

  const vkladka = page.getByRole("tab", { name: /^Реклама/ });
  await vkladka.click();
  await expect(page.getByTestId("reklama-sostoyanie")).toHaveCount(0);
  await expect(page.getByRole("radio", { name: "Расширенный" })).toBeDisabled();

  await page.getByTestId("reklama-vkl").click();
  await page.getByRole("radio", { name: "Расширенный" }).click();
  await page.getByLabel("Сайт-исключение").fill("https://Mc.Yandex.ru/x");
  await expect(page.getByTestId("razbor-isklyucheniy")).toContainText("mc.yandex.ru");
  await page.getByRole("button", { name: "Добавить в черновик" }).click();
  await expect(page.getByRole("button", { name: "Удалить исключение mc.yandex.ru" })).toBeVisible();

  await page.getByRole("button", { name: "Применить изменения" }).click();
  await expect(page.getByRole("button", { name: "Применить изменения" })).toHaveCount(0);

  // Повторное открытие: вкладка рисует то, что вернула служба, а не черновик.
  await page.getByRole("tab", { name: /^Сайты/ }).click();
  await vkladka.click();
  await expect(page.getByTestId("reklama-vkl")).toBeChecked();
  await expect(page.getByRole("radio", { name: "Расширенный" })).toHaveAttribute("aria-checked", "true");
  await expect(page.getByRole("button", { name: "Удалить исключение mc.yandex.ru" })).toBeVisible();
  await expect(page.getByTestId("vklyucheno-reklama")).toBeVisible();
  // Служба ещё держит «Базовый», и вкладка говорит об этом, а не врёт.
  await expect(page.getByTestId("reklama-sostoyanie")).toContainText("Готовится список «Расширенный»");

  // Ни страница, ни колонка содержимого не уезжают вбок.
  const perepolnenie = await page.evaluate(() => {
    const kolonka = document.querySelector('nav[aria-label="Вид правил"]')?.parentElement;
    const d = document.documentElement;
    return {
      stranica: d.scrollWidth - d.clientWidth,
      kolonka: kolonka ? kolonka.scrollWidth - kolonka.clientWidth : -1,
    };
  });
  expect(perepolnenie).toEqual({ stranica: 0, kolonka: 0 });

  await nichegoNeSlomalos(page, oshibki);
});

test("отказ обновления списка виден предупреждением", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=pravila&sluchay=reklama-otkaz");
  await page.getByRole("tab", { name: /^Реклама/ }).click();
  await expect(page.getByTestId("reklama-vazhnoe")).toHaveText("Последнее обновление не удалось: список не скачан: код ответа 503");
  await nichegoNeSlomalos(page, oshibki);
});
