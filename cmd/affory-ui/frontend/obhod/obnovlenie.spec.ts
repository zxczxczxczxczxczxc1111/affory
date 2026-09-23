import { expect, test } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Путь от находки до установки (D3, 23.09.2026).
//
// До этого дня о новой версии знали трое, и все шёпотом: карточка в настройках,
// куда надо зайти самому; пункт трея, который говорит один раз и забывает;
// уведомление Windows, живущее пять секунд. Главный экран, который человек
// видит всегда, не говорил ничего.
//
// Проверяется именно СВЯЗКА, а её не видит ни один unit-тест: находка службы
// рисует плашку в подвале, нажатие на плашку переключает вкладку и подсвечивает
// карточку. Три куска в трёх файлах, и каждый по отдельности зелёный.

test("находка службы выводит новую версию в подвал главного, а плашка ведёт к установке", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?sluchay=podnyat");

  const plashka = page.getByTestId("est-obnovlenie");
  // Пока служба не нашла выпуск, плашки нет: подвал не место для подписи «всё
  // в порядке», там и так тесно.
  await expect(plashka).toHaveCount(0);

  await page.getByRole("tab", { name: "Настройки", exact: true }).click();
  await page.getByTestId("proverit-versiyu").click();
  // Карточка обязана назвать находку своими словами: «Есть X, столько-то МБ».
  await expect(page.getByTestId("obnovlenie")).toContainText(/Есть \d+\.\d+\.\d+/);
  await expect(page.getByTestId("ustanovit-obnovlenie")).toBeEnabled();

  await page.getByRole("tab", { name: "Подключение", exact: true }).click();
  await expect(plashka).toBeVisible();
  const nomer = (await plashka.textContent())?.match(/\d+\.\d+\.\d+/)?.[0];
  expect(nomer, "плашка обязана назвать номер версии, а не просто «есть обновление»").toBeTruthy();

  // Нажатие ведёт туда, где обновление ставят, и доводит до самой карточки:
  // пункт трея этим путём ходит с 13.09.2026, плашка идёт тем же.
  await plashka.click();
  const karta = page.getByTestId("obnovlenie");
  await expect(karta).toBeVisible();
  await expect(karta).toContainText(nomer as string);
  await expect(page.getByTestId("ustanovit-obnovlenie")).toBeEnabled();

  await nichegoNeSlomalos(page, oshibki);
});
