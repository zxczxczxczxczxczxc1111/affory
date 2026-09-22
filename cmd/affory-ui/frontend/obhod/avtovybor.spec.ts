import { expect, test } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Область автовыбора правой кнопкой (A5, 22.09.2026).
//
// Зачем поверх проверок рядом с кодом. jsdom не знает ни настоящего
// контекстного меню браузера, ни того, что оно открывается вместо нашего:
// `fireEvent.contextMenu` это просто событие, а здесь проверяется, что
// preventDefault сработал и меню на экране одно - наше.

test("правая кнопка по серверу убирает его из автовыбора и возвращает обратно", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=servery");

  const oblast = page.getByTestId("oblast-avto");
  await expect(oblast).toContainText("Автовыбор перебирает все 6");

  const stroka = page.getByRole("option", { name: /Германия/ });
  await stroka.click({ button: "right" });

  const menyu = page.getByTestId("menyu-servera");
  await expect(menyu).toBeVisible();
  await menyu.getByRole("menuitem", { name: "Убрать из автовыбора" }).click();

  // Список пришёл заново: пометка на строке и новое число в области.
  await expect(page.getByTestId("vne-avto-de")).toHaveText("не в автовыборе");
  await expect(oblast).toContainText("5 из 6");

  await stroka.click({ button: "right" });
  await page.getByTestId("menyu-servera").getByRole("menuitem", { name: "Вернуть в автовыбор" }).click();
  await expect(page.getByTestId("vne-avto-de")).toHaveCount(0);
  await expect(oblast).toContainText("все 6");

  await nichegoNeSlomalos(page, oshibki);
});

test("Escape закрывает меню и ничего не меняет", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?ekran=servery");

  await page.getByRole("option", { name: /Финляндия/ }).click({ button: "right" });
  await expect(page.getByTestId("menyu-servera")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("menyu-servera")).toHaveCount(0);
  await expect(page.getByTestId("oblast-avto")).toContainText("все 6");

  await nichegoNeSlomalos(page, oshibki);
});
