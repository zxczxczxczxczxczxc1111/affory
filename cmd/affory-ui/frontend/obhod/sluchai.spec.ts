import { expect, test, type Page } from "@playwright/test";
import { lovitOshibki, nichegoNeSlomalos } from "./obshchee";

// Случаи, которых в госте нет.
//
// Гостевой прогон всегда идёт по одной дороге: служба жива, подписка есть,
// туннель поднимается. Экраны молчащей службы, отказа подъёма и пустой
// подписки за всё время не видел НИ ОДИН судья - чтобы их увидеть, продукт
// пришлось бы ломать нарочно на живой машине. Здесь они задаются параметром
// адреса (`?sluchay=`), и стоит это одну секунду на случай.
//
// Проверяется не «экран не упал», а то, ради чего эти экраны существуют:
// объяснение видно, действие предложено ровно одно и оно выполнимо.
//
// Доказано мутациями продукта 19.09.2026, по одной с откатом:
//
//   | мутация                                    | что покраснело            |
//   |--------------------------------------------|---------------------------|
//   | баннер отказа получает `absolute -top-24`  | «начинается выше кромки»  |
//   | из кнопок сервера снят `disabled={molchit}` | пять живых кнопок вместо 0|
//
// Первая мутация повторяет настоящий дефект 13.09.2026: баннер рисовался на
// Y -72 при окне от 12, человек не видел объяснения ни к одной админской
// команде, и ни один судья этого не поймал - дерево доступности узел
// показывало.

/** ЖИВЫЕ кнопки подключения к конкретному серверу: видимые и доступные.
 *
 *  Строки серверов со экрана не исчезают - человек должен видеть, что у него
 *  есть, - но нажать их, пока подключаться нечем, нельзя. Поэтому судим по
 *  доступности, а не по наличию: счёт по всему DOM полон и там, где окно
 *  ведёт себя правильно. */
function zhivyeKnopkiPodklyucheniya(page: Page) {
  return page.getByRole("button", { name: /^Подключиться к /, disabled: false }).filter({ visible: true });
}

test("выключенный туннель поднимается нажатием и проходит через «подключается»", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?sluchay=vyklyuchen");

  await expect(page.getByTestId("sostoyanie")).toHaveText("выключено", { ignoreCase: true });
  const deystvie = page.getByTestId("glavnoe-deystvie");
  await expect(deystvie).toHaveAttribute("aria-label", "Подключить");

  await deystvie.click();
  // Промежуточное состояние проверяется отдельной строкой: переход сразу в
  // «подключено» оставил бы эту ветку непройденной, а человек видит её на
  // каждом подъёме. Что во время перехода нет главной кнопки, держит
  // юнит-тест: окно 300 мс - слишком узкая щель для прибора.
  await expect(page.getByTestId("sostoyanie")).toHaveText("подключается", { ignoreCase: true });

  await expect(page.getByTestId("sostoyanie")).toHaveText("подключено", { ignoreCase: true });
  await expect(page.getByTestId("glavnoe-deystvie")).toHaveAttribute("aria-label", "Отключить");

  await nichegoNeSlomalos(page, oshibki);
});

test("молчащая служба объясняет себя и не предлагает того, чего не может", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?sluchay=molchit");

  await expect(page.getByTestId("sostoyanie")).toHaveText("служба не отвечает", { ignoreCase: true });
  // «Повторить», а не пустота: окно, потерявшее службу, однажды осталось
  // вообще без кнопок, и единственным выходом было закрыть его и запустить
  // заново (гостевой прогон 03.09.2026).
  await expect(page.getByTestId("glavnoe-deystvie")).toHaveAttribute("aria-label", "Повторить");

  // Список серверов при этом на экране остаётся и остаётся мёртвым: строки
  // видны, нажать нельзя. Отдельная надпись «список серверов недоступен»
  // появляется только когда список ПУСТ, и требовать её тут значит требовать
  // другого случая.
  await expect(zhivyeKnopkiPodklyucheniya(page)).toHaveCount(0);
  // Число нарочно не зашито: состав заглушки меняется вместе с проверками
  // протоколов, и точная пятёрка уже уронила ворота на добавленном сервере.
  // Проверяется здесь не состав, а то, что список остался на экране и мёртв
  // целиком.
  await expect(page.getByRole("button", { name: /^Подключиться к / }).first()).toBeVisible();

  await nichegoNeSlomalos(page, oshibki);
});

test("отказ подъёма объяснён целиком и виден внутри окна", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?sluchay=otkaz");

  const otkaz = page.getByTestId("otkaz");
  await expect(otkaz).toBeVisible();
  // Человеческая строка и подлинный текст службы рядом: первая объясняет,
  // вторая доказывает. Код server-auth-failed настоящий, см. заглушку.
  await expect(page.getByTestId("otkaz-tekst")).toHaveText("Сервер не принял ключ, проверь подписку", { ignoreCase: true });
  await expect(page.getByTestId("otkaz-prichina")).toContainText("не понёс трафик");
  await expect(page.getByTestId("otkaz-deystvie")).toHaveText("Обновить подписку", { ignoreCase: true });

  // Баннер ВНУТРИ окна, а не за его кромкой.
  //
  // 13.09.2026 отказ рисовался на Y -72 при окне, начинающемся с 12: экран
  // был прав по содержанию и невидим на деле, и человек не получал
  // объяснения ни к одной админской команде. Ни один судья этого не поймал,
  // потому что дерево доступности узел показывало.
  const korobka = await otkaz.boundingBox();
  expect(korobka, "у баннера отказа нет коробки: он не отрисован").not.toBeNull();
  const okno = page.viewportSize();
  expect(korobka!.y, `баннер отказа начинается выше верхней кромки окна (Y ${korobka!.y})`).toBeGreaterThanOrEqual(0);
  expect(korobka!.x, `баннер отказа уехал левее окна (X ${korobka!.x})`).toBeGreaterThanOrEqual(0);
  expect(korobka!.y + korobka!.height, "баннер отказа не помещается в окно по высоте").toBeLessThanOrEqual(okno!.height);

  await page.getByTestId("otkaz-zakryt").click();
  await expect(otkaz).toBeHidden();

  await nichegoNeSlomalos(page, oshibki);
});

test("без подписки окно зовёт завести сервер, а не показывает пустой список", async ({ page }) => {
  const oshibki = lovitOshibki(page);
  await page.goto("/?sluchay=pusto");

  await expect(page.getByRole("button", { name: "Добавить сервер", exact: true })).toBeVisible();
  await expect(zhivyeKnopkiPodklyucheniya(page)).toHaveCount(0);
  // Состояние при этом обычное «выключено», а не отказ: серверов нет, но
  // сломанного тоже ничего нет.
  await expect(page.getByTestId("sostoyanie")).toHaveText("выключено", { ignoreCase: true });

  await nichegoNeSlomalos(page, oshibki);
});
