import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Pravila } from "./Pravila";
import type { StatusOtvet } from "../protokol";
import type { PravilaTrafika } from "../trafik";

afterEach(cleanup);

// Экран правил живёт в двух видах, и оба судятся здесь.
//
// Рабочий вид рисуется, когда служба прислала набор маршрутов. Второй вид
// показывается, когда набора НЕТ: служба молчит, hello ещё не пришёл,
// listRules отложена или отказала. Редактировать в нём нечего, и требование к
// нему одно: сказать, чего он ждёт, не выдумывая ни чисел, ни положений
// переключателей.

const VYKL: StatusOtvet = { sostoyanie: "vyklyuchen", kill_switch: false, zhurnal: false };
const OTLOZHENO = { listRules: 5, setRules: 5 };
const TRAFIK: PravilaTrafika = { po_umolchaniyu: "vpn", prilozheniya: [], domeny: [], servisy: [] };

function risovat(pere: {
  status?: StatusOtvet;
  otlozheno?: Record<string, number> | null;
  pravila?: {
    protsessy: string[];
    domeny: string[];
    trafik?: PravilaTrafika;
    bez_ru_spiska?: boolean;
  } | null;
  naKomandu?: (komanda: string, telo: unknown) => void;
  pravilaOtkaz?: { kod: string; tekst?: string } | null;
  obnovitPravila?: () => void;
} = {}) {
  const naKomandu = pere.naKomandu ?? vi.fn<(komanda: string, telo: unknown) => void>();
  render(
    <Pravila
      status={pere.status ?? VYKL}
      otlozheno={pere.otlozheno === undefined ? OTLOZHENO : pere.otlozheno}
      pravila={pere.pravila ?? null}
      pravilaOtkaz={pere.pravilaOtkaz ?? null}
      obnovitPravila={pere.obnovitPravila}
      naKomandu={naKomandu}
    />,
  );
  return naKomandu;
}

/** Набор маршрутов, каким его отдаёт служба: рабочий вид экрана. */
function sTrafikom(trafik: Partial<PravilaTrafika> = {}, prochee: { bez_ru_spiska?: boolean } = {}) {
  return { protsessy: [], domeny: [], trafik: { ...TRAFIK, ...trafik }, ...prochee };
}

describe("правила без данных: экран говорит, чего ждёт", () => {
  it("пока hello не ответил, экран говорит «жду», а не выдумывает волну", () => {
    risovat({ otlozheno: null });
    expect(screen.getByTestId("zagruzka")).toBeTruthy();
    expect(screen.queryByTestId("pravila-otlozheny")).toBeNull();
  });

  it("номер волны берётся из ответа службы, а не из своей строки", () => {
    risovat({ otlozheno: { listRules: 7, setRules: 7 } });
    expect(screen.getByTestId("pravila-otlozheny")).toHaveTextContent(/волне 7/);
    expect(screen.queryByText(/волне 5/)).toBeNull();
  });

  it("на экране нет «0 правил»: ноль это измеренное значение, а мы ничего не мерили", () => {
    risovat();
    expect(screen.queryByTestId("svodka-pravil")).toBeNull();
    expect(document.body.textContent).not.toMatch(/\b0\b/);
  });

  it("молчащая служба названа причиной, и это не выдаётся за отсутствие правил", () => {
    risovat({ status: { ...VYKL, sostoyanie: "sluzhba-molchit" }, otlozheno: {} });
    expect(screen.getByTestId("pravka-nedostupna")).toHaveTextContent(/не отвеча/i);
    expect(screen.queryByText(/правил нет/i)).toBeNull();
  });

  it("отказ: неудачное чтение не выдаётся за пустой список, и повтор зовёт службу", () => {
    const obnovit = vi.fn();
    risovat({ otlozheno: {}, pravilaOtkaz: { kod: "secrets-unreadable" }, obnovitPravila: obnovit });
    expect(screen.getByTestId("otkaz-pravil")).toBeTruthy();
    expect(screen.queryByTestId("net-pravil")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /повтор/i }));
    expect(obnovit).toHaveBeenCalledTimes(1);
  });

  it("раскладка та же, что у рабочего вида: переход между ними ничего не двигает", () => {
    risovat({ otlozheno: {} });
    expect(screen.getByRole("heading", { name: "Куда идёт трафик" })).toBeTruthy();
    expect(screen.getByLabelText("Правила")).toBeTruthy();
  });

  it("журнал работает, пока служба отвечает: он не про набор правил", () => {
    const naKomandu = risovat({ otlozheno: {} });
    fireEvent.click(screen.getByTestId("razdel-zhurnal"));
    fireEvent.click(screen.getByTestId("diagnostika"));
    expect(naKomandu).toHaveBeenCalledWith("setDiagnostics", { vkl: true });
    fireEvent.click(screen.getByTestId("ochistit-zhurnal"));
    expect(naKomandu).toHaveBeenCalledWith("clearJournal", {});
  });

  it("при молчащей службе тумблеры журнала неактивны, а не врут положением", () => {
    risovat({ status: { ...VYKL, sostoyanie: "sluzhba-molchit" }, otlozheno: {} });
    fireEvent.click(screen.getByTestId("razdel-zhurnal"));
    expect((screen.getByTestId("zhurnal") as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByTestId("diagnostika") as HTMLInputElement).disabled).toBe(true);
  });
});

describe("правила: четыре состояния списка", () => {
  it("пусто: отдельных правил нет, и это сказано как факт, а не как отказ", () => {
    risovat({ otlozheno: {}, pravila: sTrafikom() });
    fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
    expect(screen.getByText(/нет отдельных правил/i)).toBeTruthy();
    expect(screen.queryByTestId("otkaz-pravil")).toBeNull();
  });

  it("много: пятьсот правил прокручиваются, а не растут без края", () => {
    const prilozheniya = Array.from({ length: 500 }, (_, i) => ({
      put: `D:\\Games\\game${i + 1}.exe`,
      imya: `game${i + 1}.exe`,
      potomki: true,
      marshrut: "direct" as const,
    }));
    risovat({ otlozheno: {}, pravila: sTrafikom({ prilozheniya }) });
    fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
    // Строки считаются по разметке, а не через getAllByRole: доступное имя
    // каждой кнопки на пятистах строках считается две с половиной минуты, и
    // это цена ЗАПРОСА, а не экрана (рендер тех же строк занимает 90 мс).
    expect(document.querySelectorAll("li[class*='grid']")).toHaveLength(500);
    // Прокручивается область вкладок, а не окно: иначе шапка с вкладками
    // уезжает вверх и вернуться к ним можно только колесом.
    const oblast = screen.getByRole("tablist").parentElement as HTMLElement;
    expect(oblast.className).toMatch(/overflow-y-auto/);
  });

  it("длинное: домен в две сотни знаков не выезжает за колонку и не пропадает", () => {
    const dlinnyy = "sub." + "o".repeat(200) + ".example.org";
    risovat({
      otlozheno: {},
      pravila: sTrafikom({ domeny: [{ domen: dlinnyy, marshrut: "direct" }] }),
    });
    fireEvent.click(screen.getByRole("tab", { name: /Сайты/ }));
    const yacheyka = screen.getByTitle(dlinnyy);
    expect(yacheyka.className).toMatch(/truncate/);
    expect(yacheyka.textContent).toBe(dlinnyy);
  });

  it("счёт правил считает все три вида, а не только открытую вкладку", () => {
    risovat({
      otlozheno: {},
      pravila: sTrafikom({
        prilozheniya: [{ put: "C:\\igra.exe", imya: "igra.exe", potomki: false, marshrut: "direct" }],
        domeny: [{ domen: "example.org", marshrut: "direct" }],
        servisy: [{ id: "youtube", marshrut: "vpn" }],
      }),
    });
    expect(screen.getByTestId("svodka-pravil")).toHaveTextContent(/3\s*правила/);
  });
});

describe("российский список", () => {
  // Решено 08.09.2026: список российских доменов работал всегда, а в окне про
  // него не было ни строки. Человек видел, что банк открывается напрямую, и не
  // мог ни подтвердить это, ни отменить.
  it("тумблер шлёт setRules ВМЕСТЕ с набором: служба заменяет его целиком", () => {
    const naKomandu = risovat({ otlozheno: {}, pravila: sTrafikom({}, { bez_ru_spiska: false }) });
    fireEvent.click(screen.getByRole("tab", { name: /Сайты/ }));
    const tumbler = screen.getByTestId("ru-spisok") as HTMLInputElement;
    expect(tumbler.checked).toBe(true);
    fireEvent.click(tumbler);
    expect(naKomandu).toHaveBeenCalledWith("setRules", { trafik: TRAFIK, bez_ru_spiska: true });
  });

  it("выключенный список рисуется выключенным", () => {
    risovat({ otlozheno: {}, pravila: sTrafikom({}, { bez_ru_spiska: true }) });
    fireEvent.click(screen.getByRole("tab", { name: /Сайты/ }));
    expect((screen.getByTestId("ru-spisok") as HTMLInputElement).checked).toBe(false);
  });

  it("служба молчит: тумблер неактивен, а не врёт положением", () => {
    risovat({
      status: { ...VYKL, sostoyanie: "sluzhba-molchit" },
      otlozheno: {},
      pravila: sTrafikom({}, { bez_ru_spiska: false }),
    });
    fireEvent.click(screen.getByRole("tab", { name: /Сайты/ }));
    expect((screen.getByTestId("ru-spisok") as HTMLInputElement).disabled).toBe(true);
  });
});
