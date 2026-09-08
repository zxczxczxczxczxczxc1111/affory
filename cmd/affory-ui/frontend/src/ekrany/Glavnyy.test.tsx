import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Glavnyy, obyom } from "./Glavnyy";
import { podpis } from "./podpisi";

afterEach(cleanup);

// Verbatim protokol.Sostoyanie. The list lives HERE, not imported from
// anywhere generated: this test must also catch the day generation lags.
const vseSostoyaniya = [
  "sluzhba-molchit",
  "vyklyuchen",
  "podnimaetsya",
  "podnyat",
  "ne-neset",
  "vosstanavlivaetsya",
  "otkaz",
] as const;

describe("главный экран", () => {
  it.each(vseSostoyaniya)("рисует %s своей подписью", (s) => {
    render(<Glavnyy status={{ sostoyanie: s }} />);
        const messages = { "sluzhba-molchit": "Нет связи со службой", vyklyuchen: "Интернет работает напрямую", podnimaetsya: "Устанавливаем соединение", podnyat: "Соединение через VPN установлено", "ne-neset": "Соединение не передаёт трафик", vosstanavlivaetsya: "Восстанавливаем соединение", otkaz: "Не удалось подключиться" };
    expect(screen.getByTestId("sostoyanie")).toHaveTextContent(messages[s]);
  });

  it("у семи состояний семь РАЗНЫХ подписей", () => {
    // A completeness check stays silent when all seven share one label:
    // each is non-empty, each matches itself, and the screen is one screen.
    const vse = vseSostoyaniya.map((s) => podpis[s]);
    expect(new Set(vse).size).toBe(vseSostoyaniya.length);
  });

  it("podnimaetsya рисуется, а не проглатывается", () => {
    // Skipping the transient state makes the app look hung: bringing the
    // tunnel up takes seconds, and the screen must say so the whole time.
    render(<Glavnyy status={{ sostoyanie: "podnimaetsya" }} />);
    expect(screen.getByTestId("sostoyanie")).toBeTruthy();
    expect(screen.getByTestId("glavnoe-deystvie")).toHaveAccessibleName("Отменить подключение");
  });

  it("цифры без данных это ПРОЧЕРК, а не ноль", () => {
    // Fallback contract: subscribeStats answers not-implemented until wave 6.
    // Zero is a measured value, and a person is entitled to read it as one.
    render(<Glavnyy status={{ sostoyanie: "podnyat" }} statistika={null} />);
    const s = screen.getByTestId("statistika").textContent ?? "";
    // The dash is a hyphen. The em dash is banned everywhere by the owner,
    // screens included, and the review caught it in this test before code.
    expect(s).toContain("-");
    expect(s).not.toMatch(/\b0\b/);
  });

  it("байты показываются в читаемых единицах, а не голым числом", () => {
    // Owner, 03.09.2026: "65094409 Б" on the strip is unreadable. Units grow
    // with the number, one decimal, comma as in Russian, no trailing zero noise.
    expect(obyom(undefined)).toBe("-");
    expect(obyom(0)).toBe("0 Б");
    expect(obyom(512)).toBe("512 Б");
    expect(obyom(65094409)).toBe("62,1 МБ");
    expect(obyom(471172330)).toBe("449,3 МБ");
    expect(obyom(5 * 1024 * 1024 * 1024)).toBe("5,0 ГБ");
    render(<Glavnyy status={{ sostoyanie: "podnyat" }} statistika={{ otdano: 65094409, prinyato: 471172330, adres_vyhoda: "203.0.113.5" }} />);
    const s = screen.getByTestId("statistika").textContent ?? "";
    expect(s).toContain("62,1 МБ");
    expect(s).toContain("449,3 МБ");
    expect(s).not.toContain("65094409");
  });

  it("несущего называет ядро, а не наш выбор", () => {
    // In auto mode vybran_id is empty by construction; that is not "no
    // server chosen".
    render(
      <Glavnyy status={{ sostoyanie: "podnyat", rezhim_marshruta: "avto",
                         vybran_id: "", nesushchiy_id: "abc123" }} />,
    );
    expect(screen.getByTestId("nesushchiy")).toHaveTextContent("abc123");
    expect(screen.queryByText(/сервер не выбран/i)).toBeNull();
  });
});

// Redraw 02.09.2026 on the accepted mockup: the main object is a card with
// the server NAME (an id on the screen was the first thing the owner saw
// and did not like), a link to change it, the route mode segment, and a
// fifth number, time online, from podnyat_s.
describe("главный экран: карточка", () => {
  const servery = [
    { id: "abc123", imya: "Amsterdam", transport: "xhttp", host: "nl.example.net", port: 443, iz_podpiski: true },
  ];

  it("называет несущего именем, а не идентификатором, когда список известен", () => {
    render(<Glavnyy status={{ sostoyanie: "podnyat", nesushchiy_id: "abc123" }} servery={servery} />);
    expect(screen.getByTestId("nesushchiy")).toHaveTextContent("Amsterdam");
    expect(screen.getByTestId("nesushchiy")).not.toHaveTextContent("abc123");
  });

  it("управление серверами открывается с главного экрана", () => {
    const na = vi.fn();
    render(<Glavnyy status={{ sostoyanie: "vyklyuchen" }} naServery={na} />);
    fireEvent.click(screen.getByRole("button", { name: "Управлять" }));
    expect(na).toHaveBeenCalledOnce();
  });

  it("сегмент режима шлёт setRouteMode со значением из протокола", () => {
    const na = vi.fn();
    render(<Glavnyy status={{ sostoyanie: "vyklyuchen", rezhim_marshruta: "avto" }} naRezhim={na} />);
    fireEvent.click(screen.getByRole("button", { name: "Вручную" }));
    expect(na).toHaveBeenCalledWith("ruchnoy");
  });

  it("время в сети считается от podnyat_s, без него прочерк", () => {
    const s = new Date(Date.now() - 65 * 60000).toISOString();
    render(<Glavnyy status={{ sostoyanie: "podnyat", podnyat_s: s }} />);
    expect(screen.getByTestId("v-seti")).toHaveTextContent(/1 ч/);
    cleanup();
    render(<Glavnyy status={{ sostoyanie: "vyklyuchen" }} />);
    expect(screen.getByTestId("v-seti")).toHaveTextContent("-");
  });
});

describe("главный экран: часы", () => {
  it("«в сети» тикает само, а не только при новом статусе", () => {
    vi.useFakeTimers();
    try {
      const s = new Date(Date.now() - 30000).toISOString();
      render(<Glavnyy status={{ sostoyanie: "podnyat", podnyat_s: s }} />);
      expect(screen.getByTestId("v-seti")).toHaveTextContent("0 мин");
      act(() => { vi.advanceTimersByTime(61000); });
      expect(screen.getByTestId("v-seti")).toHaveTextContent("1 мин");
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("главный экран: четыре состояния", () => {
  const PODNYAT = { sostoyanie: "podnyat" as const };

  it("пусто: ни списка, ни снимка, ни цифр, и ни одного выдуманного нуля", () => {
    render(<Glavnyy status={PODNYAT} spisok={null} statistika={null} />);
    expect(screen.getByTestId("statistika").textContent).not.toMatch(/\b0\b/);
    expect(screen.queryByText("Сведения о сервере")).toBeNull();
  });





  it("длинное: имя несущего без пробелов не выезжает за колонку", () => {
    const dlinnoe = "nl-" + "a".repeat(200);
    render(<Glavnyy status={{ ...PODNYAT, nesushchiy_id: "id1", nesushchiy_imya: dlinnoe }} />);
    expect(screen.getByTestId("nesushchiy")).toHaveTextContent(dlinnoe);
  });
});

describe("главный экран: имя несущего и четыре причины", () => {
  it("показывает имя несущего из кадра, а не голый идентификатор", () => {
    render(<Glavnyy status={{ sostoyanie: "podnyat", nesushchiy_id: "id-1", nesushchiy_imya: "Амстердам" }} spisok={null} />);
    expect(screen.getByTestId("nesushchiy")).toHaveTextContent("Амстердам");
    expect(screen.getByTestId("nesushchiy")).not.toHaveTextContent("id-1");
  });

  it("имя из кадра главнее списка: список мог устареть, кадр только что пришёл", () => {
    const servery = [{ id: "id-1", imya: "старое имя", transport: "xhttp", host: "nl.example.net", port: 443, iz_podpiski: true }];
    render(<Glavnyy status={{ sostoyanie: "podnyat", nesushchiy_id: "id-1", nesushchiy_imya: "Амстердам" }} servery={servery} />);
    expect(screen.getByTestId("nesushchiy")).toHaveTextContent("Амстердам");
  });



  it("текст ошибки не дублируется с баннером окна: карточка молчит", () => {
    render(<Glavnyy status={{ sostoyanie: "otkaz", oshibka: { kod: "dns-resolve-failed", tekst: "имя не разрешилось" } }} />);
    expect(screen.queryByTestId("oshibka")).toBeNull();
    expect(screen.queryByText("имя не разрешилось")).toBeNull();
  });
});

describe("подпись под переключателем режима", () => {
  // Задача И1 научила службу применять режим на ЖИВОМ ядре через clash_api:
  // переподъём больше не нужен. Подпись при этом продолжала обещать обратное,
  // и человек, переключивший режим на поднятом туннеле, ждал бы переподключения
  // ради уже случившегося. Подпись обязана зависеть от того, поднят ли туннель.
  it("на поднятом туннеле обещает применение сразу", () => {
    render(<Glavnyy status={{ sostoyanie: "podnyat", rezhim_marshruta: "ruchnoy" }} />);
    const ryad = screen.getByText("Выбор сервера").closest("div")!;
    expect(ryad.textContent).not.toContain("следующего подключения");
  });

  it("на опущенном туннеле честно говорит про следующее подключение", () => {
    // Зеркало: без него проверка выше зелена на подписи, которая молчит всегда.
    render(<Glavnyy status={{ sostoyanie: "vyklyuchen", rezhim_marshruta: "ruchnoy" }} />);
    expect(screen.getByText("Нажмите на сервер, чтобы сразу подключиться к нему.")).toBeInTheDocument();
  });
});
