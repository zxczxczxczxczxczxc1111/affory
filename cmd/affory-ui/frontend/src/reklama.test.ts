import { describe, expect, it } from "vitest";
import { normReklama, strokiSostoyaniya, type ReklamaPravila, type StrokaSostoyaniya } from "./reklama";
import type { ReklamaSostoyanie } from "./protokol";

const VKL: ReklamaPravila = { vkl: true, uroven: "light", razresheno: [] };
const ST: ReklamaSostoyanie = {
  uroven: "light",
  pravil: 44757,
  versiya: "2026.0928.0846.00",
  sobran: "2026-09-28T08:46:00Z",
  proveren: "2026-09-28T12:00:00Z",
  deystvuet: true,
};
// Разделитель тысяч у ru-RU неразрывный, и дата в поясе машины: ожидание
// считается тем же способом, что и строка, а не пишется руками.
const PRAVIL = `${new Intl.NumberFormat("ru-RU").format(44757)} правил`;
const DATA = new Date(ST.sobran as string).toLocaleDateString("ru-RU");
const teksty = (s: StrokaSostoyaniya[]) => s.map((x) => x.tekst);
const vazhnye = (s: StrokaSostoyaniya[]) => s.filter((x) => x.vazhnoe).map((x) => x.tekst);

describe("строки состояния блокировки рекламы", () => {
  it("выключенная говорит одну строку и молчит о прошлом отказе", () => {
    const s = strokiSostoyaniya({ ...VKL, vkl: false }, { ...ST, deystvuet: false, otkaz: "ядро не приняло файл списка" }, "podnyat");
    expect(s).toEqual([{ tekst: "Реклама и трекеры не блокируются. Список не скачивается", vazhnoe: false }]);
  });

  it("действующий список называет уровень, дату и число правил", () => {
    expect(strokiSostoyaniya(VKL, ST, "podnyat")).toEqual([
      { tekst: `Список «Базовый» от ${DATA} · ${PRAVIL}. Обновляется раз в сутки`, vazhnoe: false },
    ]);
  });

  it("встроенный список зовёт себя встроенным", () => {
    expect(teksty(strokiSostoyaniya(VKL, { ...ST, vstroennyy: true }, "podnyat"))).toEqual([
      `Встроенный список от ${DATA} · ${PRAVIL}. Свежий скачается при первой возможности`,
    ]);
  });

  it("без даты сборки строка не теряет смысла", () => {
    expect(teksty(strokiSostoyaniya(VKL, { ...ST, sobran: undefined }, "podnyat"))).toEqual([
      `Список «Базовый»: ${PRAVIL}. Обновляется раз в сутки`,
    ]);
  });

  it("число правил склоняется", () => {
    expect(teksty(strokiSostoyaniya(VKL, { ...ST, pravil: 1 }, "podnyat"))[0]).toContain("· 1 правило.");
    expect(teksty(strokiSostoyaniya(VKL, { ...ST, pravil: 3 }, "podnyat"))[0]).toContain("· 3 правила.");
  });

  it("без подключения говорит, когда блокировка работает, и не пугает отказом подъёма", () => {
    const s = strokiSostoyaniya(VKL, { ...ST, deystvuet: false }, "vyklyuchen");
    expect(teksty(s)[0]).toBe("Блокировка работает, пока VPN подключён");
    expect(teksty(s)[1]).toContain("Список «Базовый»");
    expect(vazhnye(s)).toEqual([]);
  });

  it("списка нет, в том числе когда служба знает только отказ", () => {
    expect(teksty(strokiSostoyaniya(VKL, undefined, "podnyat"))).toEqual(["Список готовится"]);
    const s = strokiSostoyaniya(VKL, { uroven: "", pravil: 0, deystvuet: true, otkaz: "список не скачан: код ответа 503" }, "podnyat");
    expect(teksty(s)).toEqual(["Список готовится", "Последнее обновление не удалось: список не скачан: код ответа 503"]);
  });

  it("новый уровень готовится, пока действует прежний", () => {
    expect(teksty(strokiSostoyaniya({ ...VKL, uroven: "multi" }, ST, "podnyat"))).toEqual([
      "Готовится список «Расширенный». Пока действует «Базовый»",
    ]);
  });

  it("подключение без блока предупреждает и зовёт переподключиться", () => {
    expect(vazhnye(strokiSostoyaniya(VKL, { ...ST, deystvuet: false, otkaz: "ядро не приняло файл списка" }, "podnyat"))).toEqual([
      "На этом подключении блокировка не действует: ядро не приняло файл списка. Переподключись, чтобы попробовать снова",
    ]);
    expect(vazhnye(strokiSostoyaniya(VKL, { ...ST, deystvuet: false }, "podnyat"))).toEqual([
      "На этом подключении блокировка не действует. Переподключись, чтобы попробовать снова",
    ]);
  });

  it("отказ обновления при действующем блоке это одно предупреждение", () => {
    expect(vazhnye(strokiSostoyaniya(VKL, { ...ST, otkaz: "список не скачан: код ответа 503" }, "podnyat"))).toEqual([
      "Последнее обновление не удалось: список не скачан: код ответа 503",
    ]);
  });
});

it("порядок ключей не зависит от службы", () => {
  const sluzhba: ReklamaPravila = { razresheno: ["mc.yandex.ru"], uroven: "multi", vkl: true };
  expect(JSON.stringify(normReklama(sluzhba))).toBe('{"vkl":true,"uroven":"multi","razresheno":["mc.yandex.ru"]}');
});
