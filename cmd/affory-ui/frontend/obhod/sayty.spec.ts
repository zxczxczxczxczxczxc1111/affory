import { expect, test } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Ввод сайтов в настоящем браузере (C4, 22.09.2026).
//
// Зачем поверх проверок рядом с кодом. Кириллицу в punycode превращает не наш
// код, а конструктор URL, и таблицы IDNA у jsdom (whatwg-url) свои, а у
// браузера свои, из ICU. Совпадение результата на «пример.рф» проверяется
// здесь, потому что в правило уедет именно то, что посчитает окно.

test("адрес из адресной строки и кириллица превращаются в имя для правила", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=pravila");

  await page.getByRole("tab", { name: /^Сайты/ }).click();
  await page.getByRole("button", { name: "Добавить" }).click();
  await page.getByLabel("Домен сайта").fill("https://Пример.РФ/страница?a=1, news.proba-c4.example\nnews.proba-c4.example 10.0.0.1");

  const razbor = page.getByTestId("razbor-domenov");
  await expect(razbor).toContainText("xn--e1afmkfd.xn--p1ai");
  await expect(razbor).toContainText("news.proba-c4.example");
  // Повтор внутри ввода схлопнут, адрес назван адресом.
  await expect(razbor).toContainText("Добавится 2 правила");
  await expect(razbor).toContainText("10.0.0.1: это адрес, а не имя сайта");

  await page.getByRole("button", { name: "Добавить в черновик" }).click();
  await expect(page.getByRole("button", { name: "Удалить правило xn--e1afmkfd.xn--p1ai" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Удалить правило news.proba-c4.example" })).toBeVisible();

  // Черновик отменяется: подставному мосту эти правила применять незачем.
  await page.getByRole("button", { name: "Отменить изменения" }).click();
  await expect(page.getByRole("button", { name: /proba-c4/ })).toHaveCount(0);

  await nichegoNeSlomalos(page, oshibki);
});
