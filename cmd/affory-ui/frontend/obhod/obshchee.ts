import { expect, type Page } from "@playwright/test";

// Общее для обхода и случаев: ловля ошибок страницы и учёт пробелов стенда.

/** Ошибки страницы за прогон. Необработанное исключение в React роняет
 *  поддерево молча: экран остаётся на месте, но пустой. */
export function lovitOshibki(page: Page): string[] {
  const oshibki: string[] = [];
  page.on("pageerror", (e) => oshibki.push(`исключение: ${e.message}`));
  page.on("console", (m) => {
    if (m.type() === "error") oshibki.push(`консоль: ${m.text()}`);
  });
  return oshibki;
}

/** Команды, на которые подставной мост не знает ответа.
 *
 *  Это НЕГОДЕН стенда, а не провал продукта, и различить их надо до разбора:
 *  окно, получившее отказ на незнакомую команду, ведёт себя честно, а вина
 *  лежит на заглушке. Проверяется отдельной строкой, чтобы вердикт назывался
 *  своим именем. */
export async function probelyStenda(page: Page): Promise<string[]> {
  return await page.evaluate(() => ((window as unknown as { __probelyStenda?: string[] }).__probelyStenda ?? []));
}

/** Ни одной ошибки страницы и ни одного пробела стенда. Зовётся последней
 *  строкой каждого теста. */
export async function nichegoNeSlomalos(page: Page, oshibki: string[]): Promise<void> {
  const probely = await probelyStenda(page);
  expect(probely, `НЕГОДЕН: подставной мост не знает команды ${probely.join(", ")} - чинить стенд, а не продукт`).toEqual([]);
  expect(oshibki, oshibki.join("\n")).toEqual([]);
}
