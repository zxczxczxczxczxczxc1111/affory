import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Pravila } from "./Pravila";
import type { StatusOtvet } from "../protokol";

afterEach(cleanup);

// Task 4.10. Both commands of this tab (listRules, setRules) answer
// not-implemented until wave 5, and that is a first-class state of the
// screen, not an error: the wave number comes from the SERVICE (hello names
// deferred commands), the controls are disabled with the reason next to them,
// and no zero is drawn as a measured value.

const VYKL: StatusOtvet = { sostoyanie: "vyklyuchen", kill_switch: false, zhurnal: false };
const OTLOZHENO = { listRules: 5, setRules: 5, setJournal: 6, clearJournal: 6 };

function risovat(pere: {
  status?: StatusOtvet;
  otlozheno?: Record<string, number> | null;
  pravila?: { protsessy: string[]; domeny: string[]; bez_ru_spiska?: boolean } | null;
  naKomandu?: (komanda: string, telo: unknown) => void;
  zhdutPodyoma?: boolean;
  zapushchennye?: { imya: string; put: string }[] | null;
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
      zhdutPodyoma={pere.zhdutPodyoma ?? false}
      zapushchennye={pere.zapushchennye ?? null}
      naKomandu={naKomandu}
    />,
  );
  return naKomandu;
}

describe("правила: состояние not-implemented", () => {
  it("номер волны берётся из ответа службы, а не из своей строки", () => {
    risovat({ otlozheno: { ...OTLOZHENO, listRules: 7, setRules: 7 } });
    expect(screen.getByTestId("pravila-otlozheny")).toHaveTextContent(/волне 7/);
    expect(screen.queryByText(/волне 5/)).toBeNull();
  });

  it("элементы управления неактивны и рядом сказано почему", () => {
    risovat();
    expect((screen.getByTestId("dobavit-pravilo") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("pravila-otlozheny")).toHaveTextContent(/listRules/);
  });

  it("на экране нет «0 правил»: ноль это измеренное значение, а мы ничего не мерили", () => {
    risovat();
    expect(document.body.textContent).not.toMatch(/\b0 правил/);
    expect(document.body.textContent).not.toMatch(/\b0\b/);
  });

  it("пока hello не ответил, экран говорит «загрузка», а не выдумывает волну", () => {
    risovat({ otlozheno: null });
    expect(screen.getByTestId("zagruzka")).toBeTruthy();
    expect(screen.queryByText(/волне/)).toBeNull();
  });
});

describe("правила: когда команда появится", () => {
  it("если listRules не отложена, список рисуется и «Добавить» активна", () => {
    risovat({ otlozheno: { setJournal: 6, clearJournal: 6 }, pravila: { protsessy: ["game.exe"], domeny: ["example.net"] } });
    expect(screen.getByText("game.exe")).toBeTruthy();
    expect((screen.getByTestId("dobavit-pravilo") as HTMLButtonElement).disabled).toBe(false);
    fireEvent.click(screen.getByRole("radio", { name: "домены" }));
    expect(screen.getByText("example.net")).toBeTruthy();
  });
});

describe("правила: режим «весь трафик» и журнал", () => {
  it("при kill-switch список сам говорит, что исключения не действуют", () => {
    risovat({ status: { ...VYKL, kill_switch: true } });
    expect(screen.getByTestId("ves-trafik")).toHaveTextContent(/не действуют/);
    cleanup();
    risovat({ status: { ...VYKL, kill_switch: false } });
    expect(screen.queryByTestId("ves-trafik")).toBeNull();
  });

  it("журнал: тумблер неактивен с номером волны setJournal, значение из статуса", () => {
    risovat({ status: { ...VYKL, zhurnal: false } });
    const t = screen.getByTestId("zhurnal") as HTMLInputElement;
    expect(t.disabled).toBe(true);
    expect(t.checked).toBe(false);
    expect(screen.getByTestId("zhurnal-ryad")).toHaveTextContent(/волне 6/);
    expect((screen.getByTestId("ochistit-zhurnal") as HTMLButtonElement).disabled).toBe(true);
  });

  it("когда setJournal реализована, тумблер шлёт setJournal и ничего больше", () => {
    const na = risovat({ otlozheno: { listRules: 5, setRules: 5 }, status: { ...VYKL, zhurnal: false } });
    fireEvent.click(screen.getByTestId("zhurnal"));
    expect(na).toHaveBeenCalledTimes(1);
    expect(na).toHaveBeenCalledWith("setJournal", { vkl: true });
  });

  // Подробный журнал это ОТДЕЛЬНЫЙ тумблер. Журнал соединений пишет, куда
  // ходили; этот пишет, почему встало. Включают их по разным поводам.
  it("подробный журнал: свой тумблер, значение из статуса", () => {
    risovat({ otlozheno: { listRules: 5, setRules: 5 }, status: { ...VYKL, diagnostika: true } });
    const t = screen.getByTestId("diagnostika") as HTMLInputElement;
    expect(t.checked).toBe(true);
  });

  it("подробный журнал шлёт setDiagnostics и не трогает журнал соединений", () => {
    const na = risovat({ otlozheno: { listRules: 5, setRules: 5 }, status: { ...VYKL, diagnostika: false } });
    fireEvent.click(screen.getByTestId("diagnostika"));
    expect(na).toHaveBeenCalledTimes(1);
    expect(na).toHaveBeenCalledWith("setDiagnostics", { vkl: true });
  });
});

describe("правила: домены и честное предупреждение", () => {
  it("в списке доменов есть строка про DoH браузера с причиной, а не приговором", () => {
    risovat({ otlozheno: {}, pravila: { protsessy: [], domeny: ["example.org"] } });
    fireEvent.click(screen.getByRole("radio", { name: "домены" }));
    const p = screen.getByTestId("doh-preduprezhdenie");
    // The cause: the browser resolves over DoH on 443, so the rule holds only
    // on the TLS handshake. And the action the human can take: switch DoH off.
    expect(p).toHaveTextContent(/DoH/);
    expect(p).toHaveTextContent(/рукопожати/);
    expect(p).toHaveTextContent(/выключи/);
    expect(p).not.toHaveTextContent(/голому адресу/);
  });

  it("предупреждение стоит и над пустым списком доменов", () => {
    risovat({ otlozheno: {}, pravila: { protsessy: [], domeny: [] } });
    fireEvent.click(screen.getByRole("radio", { name: "домены" }));
    expect(screen.getByTestId("doh-preduprezhdenie")).toBeInTheDocument();
  });

  it("в списке процессов предупреждения про DoH нет", () => {
    risovat({ otlozheno: {}, pravila: { protsessy: ["C:\Steam\steam.exe"], domeny: [] } });
    expect(screen.queryByTestId("doh-preduprezhdenie")).toBeNull();
  });
});

describe("правила: форма добавления и удаление", () => {
  const PRAVILA = { protsessy: ["C:\Steam\steam.exe"], domeny: ["example.org"] };

  it("«Добавить» открывает форму, процесс по пути уходит в setRules ПОЛНЫМ списком", () => {
    const naKomandu = risovat({ otlozheno: {}, pravila: PRAVILA });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    const forma = screen.getByTestId("forma");
    expect(forma).toBeInTheDocument();
    fireEvent.change(screen.getByTestId("put-protsessa"), { target: { value: "D:\Games\game.exe" } });
    fireEvent.click(screen.getByTestId("dobavit-zapis"));
    expect(naKomandu).toHaveBeenCalledWith("setRules", {
      protsessy: ["C:\Steam\steam.exe", "D:\Games\game.exe"],
      domeny: ["example.org"],
    });
  });

  // Adding a domain moved to the «форма спрашивает то, что выбрано вкладкой»
  // block below: the form's own type segment is gone, the tab decides.

  it("пустая строка не отправляется", () => {
    risovat({ otlozheno: {}, pravila: PRAVILA });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    expect(screen.getByTestId("dobavit-zapis")).toBeDisabled();
  });

  it("удаление в два щелчка шлёт setRules без этой строки", () => {
    const p = PRAVILA.protsessy[0];
    const naKomandu = risovat({ otlozheno: {}, pravila: PRAVILA });
    fireEvent.click(screen.getByTestId(`udalit-${p}`));
    expect(naKomandu).not.toHaveBeenCalled();
    expect(screen.getByTestId(`podtverdit-${p}`)).toHaveTextContent(/удалить\?/);
    fireEvent.click(screen.getByTestId(`podtverdit-${p}`));
    expect(naKomandu).toHaveBeenCalledWith("setRules", { protsessy: [], domeny: ["example.org"] });
  });

  it("удаляется ВЫБРАННАЯ строка, а не первая попавшаяся", () => {
    // Three rows, and the MIDDLE one goes. With a single row in the set,
    // "delete this one" and "delete whichever" are the same green test, and
    // that is exactly how a filter by index survived here (03.09.2026).
    const nabor = { protsessy: [], domeny: ["a.example.org", "b.example.org", "c.example.org"] };
    const naKomandu = risovat({ otlozheno: {}, pravila: nabor });
    fireEvent.click(screen.getByRole("radio", { name: "домены" }));
    fireEvent.click(screen.getByTestId("udalit-b.example.org"));
    fireEvent.click(screen.getByTestId("podtverdit-b.example.org"));
    expect(naKomandu).toHaveBeenCalledWith("setRules", {
      protsessy: [],
      domeny: ["a.example.org", "c.example.org"],
    });
  });

  it("пока команды отложены, формы нет и строк тоже нет", () => {
    risovat({ pravila: PRAVILA });
    expect(screen.getByTestId("dobavit-pravilo")).toBeDisabled();
    expect(screen.queryByTestId(`udalit-${PRAVILA.protsessy[0]}`)).toBeNull();
  });

  it("после setRules при поднятом туннеле список говорит, что применится со следующего подключения", () => {
    risovat({ otlozheno: {}, pravila: PRAVILA, zhdutPodyoma: true });
    expect(screen.getByTestId("zhdut-podyoma")).toHaveTextContent(/следующего подключения/);
  });
});

describe("правила: процесс из списка запущенных", () => {
  const PRAVILA = { protsessy: [], domeny: [] };
  const ZAPUSHCHENNYE = [
    { imya: "game.exe", put: "D:\Games\game.exe" },
    { imya: "steam.exe", put: "C:\Steam\steam.exe" },
  ];

  it("выбор из запущенных подставляет путь в поле, и он уходит в setRules", () => {
    const naKomandu = risovat({ otlozheno: {}, pravila: PRAVILA, zapushchennye: ZAPUSHCHENNYE });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    // Owner, 03.09.2026: the native <select> drew a white list with blank
    // rows over the dark window. The picker is ours: a button opens a listbox
    // in the window's own style, a row click fills the path.
    fireEvent.click(screen.getByTestId("zapushchennye"));
    expect(screen.getByRole("listbox", { name: "запущенные процессы" })).toBeInTheDocument();
    expect(screen.getAllByRole("option")).toHaveLength(2);
    fireEvent.click(screen.getByTestId("zapushchennyy-steam.exe"));
    expect(screen.queryByRole("listbox", { name: "запущенные процессы" })).toBeNull();
    expect(screen.getByTestId("put-protsessa")).toHaveValue("C:\Steam\steam.exe");
    fireEvent.click(screen.getByTestId("dobavit-zapis"));
    expect(naKomandu).toHaveBeenCalledWith("setRules", { protsessy: ["C:\Steam\steam.exe"], domeny: [] });
  });

  it("пока список запущенных не пришёл, поле пути работает само, без выбора", () => {
    risovat({ otlozheno: {}, pravila: PRAVILA, zapushchennye: null });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    expect(screen.queryByTestId("zapushchennye")).toBeNull();
    expect(screen.getByTestId("put-protsessa")).toBeInTheDocument();
  });
});

// Two defects the owner found on the screen, 04.09.2026.
describe("правила: форма спрашивает то, что выбрано вкладкой", () => {
  const PRAVILA = { protsessy: ["C:\\Steam\\steam.exe"], domeny: ["example.org"] };
  const ZAPUSHCHENNYE = [{ imya: "steam.exe", put: "C:\\Steam\\steam.exe" }];

  it("на вкладке «домены» форма спрашивает домен, и процессов в ней нет", () => {
    const naKomandu = risovat({ otlozheno: {}, pravila: PRAVILA, zapushchennye: ZAPUSHCHENNYE });
    fireEvent.click(screen.getByRole("radio", { name: "домены" }));
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    expect(screen.queryByTestId("put-protsessa")).toBeNull();
    expect(screen.queryByTestId("zapushchennye")).toBeNull();
    fireEvent.change(screen.getByTestId("domen"), { target: { value: "cdn.example.net" } });
    fireEvent.click(screen.getByTestId("dobavit-zapis"));
    expect(naKomandu).toHaveBeenCalledWith("setRules", {
      protsessy: ["C:\\Steam\\steam.exe"],
      domeny: ["example.org", "cdn.example.net"],
    });
  });

  it("на вкладке «процессы» доменного поля нет, и выбора типа тоже", () => {
    // The type used to live in its own state, always starting at "процесс".
    // So the domains tab opened a process form, and the processes tab could
    // add a domain that then vanished into the other view.
    risovat({ otlozheno: {}, pravila: PRAVILA, zapushchennye: ZAPUSHCHENNYE });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    expect(screen.getByTestId("put-protsessa")).toBeInTheDocument();
    expect(screen.queryByTestId("domen")).toBeNull();
    expect(screen.queryByRole("radio", { name: "домен" })).toBeNull();
  });

  it("смена вкладки закрывает открытую форму вместе с введённым", () => {
    risovat({ otlozheno: {}, pravila: PRAVILA });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    fireEvent.change(screen.getByTestId("put-protsessa"), { target: { value: "D:\\Games\\game.exe" } });
    fireEvent.click(screen.getByRole("radio", { name: "домены" }));
    expect(screen.queryByTestId("forma")).toBeNull();
    fireEvent.click(screen.getByRole("radio", { name: "процессы" }));
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    expect(screen.getByTestId("put-protsessa")).toHaveValue("");
  });
});

describe("правила: список запущенных виден целиком", () => {
  /** jsdom does not lay anything out, so the check is on the CAUSE found in
   *  the code 04.09.2026: the list hangs `absolute` inside a card that clips
   *  whatever leaves it, and the card ends right under the field, so one row
   *  survived on screen. Either answer is fine: the list in the flow (the
   *  container grows with it), or no clipping ancestor up to the form. */
  function obrezaetsya(spisok: HTMLElement, forma: HTMLElement): boolean {
    if (!/\b(absolute|fixed)\b/.test(spisok.className)) return false;
    for (let u: HTMLElement | null = spisok.parentElement; u; u = u.parentElement) {
      if (/\boverflow-hidden\b/.test(u.className)) return true;
      if (u === forma) return false;
    }
    return false;
  }

  it("карточка формы не режет выпавший список", () => {
    const zapushchennye = Array.from({ length: 12 }, (_, i) => ({ imya: `p${i}.exe`, put: `C:\\p${i}.exe` }));
    risovat({ otlozheno: {}, pravila: { protsessy: [], domeny: [] }, zapushchennye });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    fireEvent.click(screen.getByTestId("zapushchennye"));
    const spisok = screen.getByRole("listbox", { name: "запущенные процессы" });
    expect(obrezaetsya(spisok, screen.getByTestId("forma"))).toBe(false);
  });

  it("длинный список прокручивается сам, а не растёт без края", () => {
    const zapushchennye = Array.from({ length: 200 }, (_, i) => ({ imya: `p${i}.exe`, put: `C:\\p${i}.exe` }));
    risovat({ otlozheno: {}, pravila: { protsessy: [], domeny: [] }, zapushchennye });
    fireEvent.click(screen.getByTestId("dobavit-pravilo"));
    fireEvent.click(screen.getByTestId("zapushchennye"));
    const spisok = screen.getByRole("listbox", { name: "запущенные процессы" });
    expect(spisok.className).toMatch(/\bmax-h-/);
    expect(spisok.className).toMatch(/\boverflow-y-auto\b/);
  });
});

// Правило полосы Е: четыре состояния у каждого экрана. Разбор 03.09.2026:
// пустой массив и неудачная загрузка рисовались ОДНОЙ строкой «исключений
// нет, весь трафик идёт через туннель», и это прямая ложь, когда правила
// просто не прочитались.
describe("правила: четыре состояния", () => {
  const mnogo = (n: number) => Array.from({ length: n }, (_, i) => `D:\Games\game${i + 1}.exe`);

  it("пусто: исключений нет, и это сказано как факт, а не как отказ", () => {
    risovat({ otlozheno: {}, pravila: { protsessy: [], domeny: [] } });
    expect(screen.getByText(/процессов в исключениях нет/i)).toBeTruthy();
    expect(screen.queryByTestId("otkaz-pravil")).toBeNull();
  });

  it("отказ: неудачная загрузка не выдаётся за отсутствие исключений", () => {
    const obnovit = vi.fn();
    risovat({ otlozheno: {}, pravila: null, pravilaOtkaz: { kod: "secrets-unreadable" }, obnovitPravila: obnovit });
    expect(screen.queryByText(/исключениях нет/i)).toBeNull();
    expect(screen.getByText(/не прочит/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /повтор/i }));
    expect(obnovit).toHaveBeenCalledTimes(1);
  });

  it("много: пятьсот правил не превращают окно в бесконечную страницу", () => {
    risovat({ otlozheno: {}, pravila: { protsessy: mnogo(500), domeny: [] } });
    const spisokEl = screen.getByTestId("spisok-pravil");
    expect(spisokEl.className).toMatch(/overflow-y-auto/);
    expect(spisokEl.className).toMatch(/max-h-\[60vh\]/);
    expect(spisokEl.querySelectorAll("[data-testid^='pravilo-']").length).toBe(500);
  });

  it("длинное: домен без пробелов ломается по символам, а не выезжает за колонку", () => {
    const dlinnyy = "sub." + "o".repeat(200) + ".example.org";
    risovat({ otlozheno: {}, pravila: { protsessy: [], domeny: [dlinnyy] } });
    fireEvent.click(screen.getByRole("radio", { name: "домены" }));
    expect(screen.getByText(dlinnyy).className).toMatch(/break-all/);
  });
});

// Разбор 03.09.2026: подтверждение удаления хранилось как `${vid}-${i}`, то
// есть по НОМЕРУ строки. Служба порядок не меняет, но УКОРАЧИВАЕТ список
// дедупом (komandy_pravil.go), и номера едут на соседа.
describe("правила: удаление опознаётся по правилу, а не по номеру", () => {
  it("подтверждение не переезжает на соседнюю строку, когда список укоротился", () => {
    const naKomandu = vi.fn<(komanda: string, telo: unknown) => void>();
    const { rerender } = render(
      <Pravila status={VYKL} otlozheno={{}} pravila={{ protsessy: ["a.exe", "b.exe", "c.exe"], domeny: [] }}
        pravilaOtkaz={null} zhdutPodyoma={false} zapushchennye={null} naKomandu={naKomandu} />,
    );
    fireEvent.click(screen.getByTestId("udalit-b.exe"));
    expect(screen.getByTestId("podtverdit-b.exe")).toBeTruthy();
    rerender(
      <Pravila status={VYKL} otlozheno={{}} pravila={{ protsessy: ["b.exe", "c.exe"], domeny: [] }}
        pravilaOtkaz={null} zhdutPodyoma={false} zapushchennye={null} naKomandu={naKomandu} />,
    );
    expect(screen.queryByTestId("podtverdit-c.exe")).toBeNull();
    // Любая смена списка гасит вопрос целиком: чужой ответ не должен
    // оставлять взведённое удаление ни на ком.
    expect(screen.queryByTestId("podtverdit-b.exe")).toBeNull();
    expect(naKomandu).not.toHaveBeenCalled();
  });

  it("при молчащей службе корзина на месте и рядом сказано, почему нельзя", () => {
    risovat({ status: { ...VYKL, sostoyanie: "sluzhba-molchit" }, otlozheno: {}, pravila: { protsessy: ["a.exe"], domeny: [] } });
    expect(screen.getByTestId("udalit-a.exe")).toBeDisabled();
    expect(screen.getByTestId("pravka-nedostupna")).toHaveTextContent(/служба не отвечает/);
  });
});

describe("российский список", () => {
  // Решено 08.09.2026: список российских доменов работал всегда, а
  // в окне про него не было ни строки. Человек видел, что банк открывается
  // напрямую, и не мог ни подтвердить это, ни отменить.
  const PRAVILA = { protsessy: ["C:\igra.exe"], domeny: ["example.org"] };

  it("тумблер шлёт setRules ВМЕСТЕ со списками: служба заменяет их целиком", () => {
    const naKomandu = risovat({ otlozheno: {}, pravila: { ...PRAVILA, bez_ru_spiska: false } });
    const tumbler = screen.getByTestId("ru-spisok") as HTMLInputElement;
    expect(tumbler.checked).toBe(true);
    fireEvent.click(tumbler);
    expect(naKomandu).toHaveBeenCalledWith("setRules", { ...PRAVILA, bez_ru_spiska: true });
  });

  it("выключенный список рисуется выключенным", () => {
    risovat({ otlozheno: {}, pravila: { ...PRAVILA, bez_ru_spiska: true } });
    expect((screen.getByTestId("ru-spisok") as HTMLInputElement).checked).toBe(false);
    const nazad = screen.getByTestId("ru-spisok") as HTMLInputElement;
    fireEvent.click(nazad);
  });

  it("служба молчит: тумблер неактивен, а не врёт положением", () => {
    risovat({ status: { ...VYKL, sostoyanie: "sluzhba-molchit" }, otlozheno: {}, pravila: { ...PRAVILA, bez_ru_spiska: false } });
    expect((screen.getByTestId("ru-spisok") as HTMLInputElement).disabled).toBe(true);
  });
});
