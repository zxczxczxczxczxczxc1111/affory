import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Servery, pohozheNaSsylku, type SpisokServerov, type ZamerZaderzhki } from "./Servery";
import type { Server, StatusOtvet } from "../protokol";

afterEach(cleanup);

// Task 4.9. The only tab whose commands all exist in the service, so it is
// accepted whole: list, subscription, add, remove, select. The "many data"
// state is measured, not imagined: five foreign subscriptions gave 59
// servers after folding (02.09.2026), and that is the floor, not the ceiling.

function server(n: number, pere: Partial<Server> = {}): Server {
  return {
    id: `id${n}`, imya: `Сервер ${n}`, transport: "reality-tcp",
    host: `s${n}.example.net`, port: 443, iz_podpiski: true, ...pere,
  };
}

function spisok(servery: Server[], pere: Partial<SpisokServerov> = {}): SpisokServerov {
  return { servery, vybran: "", podpiska_zadana: true, podpiska_uzel: "panel.example.net", ...pere };
}

const VYKL: StatusOtvet = { sostoyanie: "vyklyuchen", rezhim_marshruta: "ruchnoy" };

function risovat(s: SpisokServerov | null, status: StatusOtvet = VYKL, naKomandu = vi.fn()) {
  render(<Servery status={status} spisok={s} naKomandu={naKomandu} />);
  return naKomandu;
}

describe("серверы: список", () => {
  it("рисует 59 строк, каждая со своим именем, в прокручиваемом списке", () => {
    const mnogo = Array.from({ length: 59 }, (_, i) => server(i + 1));
    risovat(spisok(mnogo));
    const rows = screen.getAllByRole("option");
    expect(rows).toHaveLength(59);
    expect(rows[58]).toHaveTextContent("Сервер 59");
    // The list scrolls inside itself; the tab header stays put.
    expect(screen.getByRole("listbox").className).toMatch(/overflow-y-auto/);
  });

  it("выбранный, которого нет в свежем списке, помечен «нет в подписке» и не молчит", () => {
    risovat(spisok([server(1), server(2)], { vybran: "ushol" }), { ...VYKL, vybran_id: "ushol" });
    const pometka = screen.getByTestId("vybran-propal");
    expect(pometka).toHaveTextContent(/нет в подписке/);
    // A silent replacement by the first server is what this marker exists
    // against: the human must see that what runs is not what they chose.
    expect(pometka).toHaveTextContent(/первый по списку/);
  });

  it("в авто пустой выбор это не «сервер не выбран»", () => {
    risovat(spisok([server(1)]), { sostoyanie: "vyklyuchen", rezhim_marshruta: "avto", vybran_id: "" });
    expect(screen.queryByText(/не выбран/)).toBeNull();
    expect(screen.getByTestId("svodka")).toHaveTextContent(/автоматически/);
  });

  it("щелчок по строке выбирает сервер одной командой setServer", () => {
    const na = risovat(spisok([server(1), server(2)]));
    fireEvent.click(screen.getAllByRole("option")[1]);
    expect(na).toHaveBeenCalledTimes(1);
    expect(na).toHaveBeenCalledWith("setServer", { id: "id2" });
  });

  it("выбранная строка отмечена aria-selected, несущий помечен «активен»", () => {
    risovat(spisok([server(1), server(2)], { vybran: "id1" }), { ...VYKL, vybran_id: "id1", nesushchiy_id: "id2", sostoyanie: "podnyat" });
    const rows = screen.getAllByRole("option");
    expect(rows[0].getAttribute("aria-selected")).toBe("true");
    expect(rows[1].getAttribute("aria-selected")).toBe("false");
    expect(within(rows[1]).getByText("активен")).toBeTruthy();
  });

  it("пометки строки: добавлен вручную и принудительная проверка сертификата", () => {
    risovat(spisok([server(1, { iz_podpiski: false }), server(2, { nebezopasnyy_ignorirovan: true })]));
    const rows = screen.getAllByRole("option");
    expect(rows[0]).toHaveTextContent(/добавлен вручную/);
    expect(rows[1]).toHaveTextContent(/проверка сертификата/);
  });

  it("пометка строки: пин сертификата у hy2 с pinSHA256", () => {
    risovat(spisok([server(1, { transport: "hy2", s_pinom: true }), server(2, { transport: "hy2" })]));
    const rows = screen.getAllByRole("option");
    expect(rows[0]).toHaveTextContent(/пин сертификата/);
    expect(rows[1]).not.toHaveTextContent(/пин сертификата/);
  });

  it("поиск фильтрует по имени и по адресу", () => {
    risovat(spisok([server(1, { imya: "Amsterdam" }), server(2, { imya: "Paris", host: "fr.example.net" })]));
    fireEvent.change(screen.getByTestId("poisk"), { target: { value: "fr.exa" } });
    const rows = screen.getAllByRole("option");
    expect(rows).toHaveLength(1);
    expect(rows[0]).toHaveTextContent("Paris");
  });

  it("удаление в два щелчка: первый спрашивает, второй шлёт removeServer", () => {
    const na = risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("udalit-id1"));
    expect(na).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("udalit-id1"));
    expect(na).toHaveBeenCalledWith("removeServer", { id: "id1" });
  });
});

describe("серверы: подписка и добавление", () => {
  it("адрес подписки не рисуется никогда, только узел", () => {
    risovat(spisok([server(1)]));
    expect(screen.getByTestId("podpiska")).toHaveTextContent("panel.example.net");
    expect(document.body.textContent).not.toMatch(/https?:\/\//);
  });

  it("«Обновить» шлёт refreshSubscription; подписка задаётся через ту же «Добавить», выбором «подписка»", () => {
    // Owner's remark 02.09.2026: a server via "Добавить" and a subscription
    // via "изменить" were two buttons with two meanings for one act. One
    // button, one question: what exactly is being added.
    const na = risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("obnovit-podpisku"));
    expect(na).toHaveBeenCalledWith("refreshSubscription", {});
    expect(screen.queryByTestId("izmenit-podpisku")).toBeNull();
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.click(screen.getByRole("radio", { name: "подписка" }));
    fireEvent.change(screen.getByTestId("adres-podpiski"), { target: { value: "  https://p.example/sub  " } });
    fireEvent.click(screen.getByTestId("sohranit-podpisku"));
    expect(na).toHaveBeenCalledWith("setSubscription", { adres: "https://p.example/sub" });
  });

  it("непонятые строки подписки видны на экране, с номером и причиной", () => {
    // 05.09.2026. Служба причину знала с первого дня и отдавала полем otkazy,
    // а экран его не читал вовсе. Человек видел просто МЕНЬШЕ серверов, чем
    // прислала панель, без единого слова о том, почему. Ровно так и терялись
    // ссылки anytls, пока разбора для них не было.
    render(
      <Servery
        status={VYKL}
        spisok={spisok([server(1)])}
        naKomandu={vi.fn()}
        otkazyPodpiski={[
          { stroka: 3, prichina: "транспорт не поддерживается" },
          { stroka: 7, prichina: "ссылка не разобрана" },
        ]}
      />,
    );
    const k = screen.getByTestId("otkazy-podpiski");
    expect(k).toHaveTextContent("2");
    expect(k).toHaveTextContent("строка 3");
    expect(k).toHaveTextContent("транспорт не поддерживается");
    expect(k).toHaveTextContent("строка 7");
  });

  it("без непонятых строк карточки отказов нет вовсе", () => {
    // Пустой список это не «ноль отказов» на экране, а отсутствие разговора.
    risovat(spisok([server(1)]));
    expect(screen.queryByTestId("otkazy-podpiski")).toBeNull();
  });

  it("причина отказа рисуется, а сама ссылка никогда", () => {
    // В отказе служба присылает уже очищенный текст, но экран обязан не
    // подставить ссылку сам: в ней uuid и ключи.
    render(
      <Servery
        status={VYKL}
        spisok={spisok([server(1)])}
        naKomandu={vi.fn()}
        otkazyPodpiski={[{ stroka: 1, prichina: "транспорт не поддерживается: схема mieru" }]}
      />,
    );
    expect(screen.getByTestId("otkazy-podpiski")).toHaveTextContent("схема mieru");
    expect(document.body.textContent).not.toMatch(/https?:\/\/|vless:|anytls:/);
  });

  it("форма добавления с уже заданной подпиской предупреждает, что новая её заменит", () => {
    risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.click(screen.getByRole("radio", { name: "подписка" }));
    expect(screen.getByTestId("forma")).toHaveTextContent(/заменит/);
  });

  it("пустой список это первый запуск §9.2: поле для ссылки и одно действие, без «0 серверов»", () => {
    risovat(spisok([], { podpiska_zadana: false, podpiska_uzel: "" }));
    expect(screen.getByTestId("pusto")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/\b0 серверов/);
    expect(screen.queryByRole("option")).toBeNull();
    // The form is open by default with the choice visible: the first thing
    // the human does here is paste something, not look for a button.
    expect(screen.getByTestId("ssylka")).toBeTruthy();
    expect(screen.getByRole("radio", { name: "подписка" })).toBeTruthy();
  });

  it("«Добавить» раскрывает поле, ссылка уходит в addServer обрезанной", () => {
    const na = risovat(spisok([server(1)]));
    fireEvent.click(screen.getByTestId("dobavit"));
    fireEvent.change(screen.getByTestId("ssylka"), { target: { value: " vless://x@h:443?security=reality " } });
    fireEvent.click(screen.getByTestId("dobavit-ssylku"));
    expect(na).toHaveBeenCalledWith("addServer", { ssylka: "vless://x@h:443?security=reality" });
  });

  it("пока списка нет, экран говорит «загрузка», а не рисует пустоту с нулями", () => {
    risovat(null);
    expect(screen.getByTestId("zagruzka")).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/\b0\b/);
  });

  it("пока служба молчит, всё неактивно", () => {
    risovat(spisok([server(1)]), { sostoyanie: "sluzhba-molchit" });
    expect((screen.getByTestId("dobavit") as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByTestId("obnovit-podpisku") as HTMLButtonElement).disabled).toBe(true);
  });
});

// План «шесть удобств» §3: ссылка из буфера обмена и QR с экрана. Текст
// буфера на экран не попадает: либо ушёл в addServer, либо отвергнут словами.
describe("серверы: из буфера и с экрана", () => {
  it("похоже на ссылку: шесть схем, регистр и пробелы не мешают, прочее нет", () => {
    for (const s of ["vless://a", " HY2://b ", "hysteria2://c", "ss://d", "trojan://e", "vmess://f"]) expect(pohozheNaSsylku(s)).toBe(true);
    // tuic служба и правда не поддерживает: список схем окна обязан совпадать
    // с тем, что принимает разбор, иначе кнопка обещает больше, чем есть.
    for (const s of ["", "https://example.org", "tuic://x", "просто текст"]) expect(pohozheNaSsylku(s)).toBe(false);
  });

  it("кнопок нет, пока оболочка не дала способа читать буфер и экран", () => {
    risovat(spisok([]));
    expect(screen.queryByTestId("iz-bufera")).toBeNull();
    expect(screen.queryByTestId("qr-s-ekrana")).toBeNull();
  });

  it("ссылка из буфера уходит в addServer, поле остаётся пустым", async () => {
    const naKomandu = vi.fn();
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={naKomandu} chitatBufer={async () => " vless://x@h:443?security=reality "} />);
    fireEvent.click(screen.getByTestId("iz-bufera"));
    await vi.waitFor(() => expect(naKomandu).toHaveBeenCalledWith("addServer", { ssylka: "vless://x@h:443?security=reality" }));
    expect((screen.getByTestId("ssylka") as HTMLInputElement).value).toBe("");
  });

  it("не ссылка в буфере: отказ словами, команды нет, текст буфера не показан", async () => {
    const naKomandu = vi.fn();
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={naKomandu} chitatBufer={async () => "секретный текст"} />);
    fireEvent.click(screen.getByTestId("iz-bufera"));
    const ishod = await screen.findByTestId("ishod-vvoda");
    expect(ishod).toHaveTextContent(/не ссылка/);
    expect(ishod).not.toHaveTextContent(/секретный/);
    expect(naKomandu).not.toHaveBeenCalled();
  });

  it("QR с экрана: имя добавленного показано, отказ оболочки показан словами", async () => {
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} naQrSEkrana={async () => "vpn-pc-hy2"} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    expect(await screen.findByTestId("ishod-vvoda")).toHaveTextContent(/добавлен vpn-pc-hy2/);
  });

  // Разбор 03.09.2026: отказ службы на «QR с экрана» показывался серым мелким
  // шрифтом под полем, мимо §9.1 и без кнопки.
  it("отказ QR с экрана показывается экраном отказа, а не мелким серым текстом", async () => {
    const naQr = vi.fn(async () => { throw new Error("QR на экране не найден"); });
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} naQrSEkrana={naQr} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    const otkaz = await screen.findByTestId("otkaz-qr");
    expect(otkaz.getAttribute("role")).toBe("alert");
    expect(otkaz).toHaveTextContent(/не найден/);
    expect(screen.queryByTestId("ishod-vvoda")).toBeNull();
    fireEvent.click(screen.getByTestId("otkaz-qr-deystvie"));
    expect(naQr).toHaveBeenCalledTimes(2);
  });

  // Экран показывал этому отказу код `qr-s-ekrana`, которого нет ни в словаре
  // протокола, ни в §9.1, ни в otkazy.ts. Компонент неизвестный код переживает
  // молча, поэтому экран выглядел правильным. Чтение QR это работа окна, а не
  // службы: своя фраза и своё действие у отказа есть, а кода на проводе быть
  // не должно.
  it("отказ QR не показывает кода, которого нет в словаре", async () => {
    const naQr = vi.fn(async () => { throw new Error("QR на экране не найден"); });
    render(<Servery status={VYKL} spisok={spisok([])} naKomandu={vi.fn()} naQrSEkrana={naQr} />);
    fireEvent.click(screen.getByTestId("qr-s-ekrana"));
    const otkaz = await screen.findByTestId("otkaz-qr");
    expect(otkaz.getAttribute("data-kod")).toBeNull();
    expect(otkaz).toHaveTextContent(/QR с экрана не прочитался/);
  });
});

// Правило полосы Е: у каждого экрана тесты на четыре состояния. Обход
// интерфейса 03.09.2026 нашёл здесь тупик: при отказе listServers на вкладке
// не было ни списка, ни кнопки, ни формы, и уйти с неё было некуда.
describe("серверы: четыре состояния", () => {
  it("пусто: списка нет, а добавить можно сразу", () => {
    risovat(spisok([]));
    expect(screen.getByTestId("pusto")).toBeTruthy();
    expect(screen.getByTestId("ssylka")).toBeTruthy();
    expect(screen.queryByTestId("otkaz-spiska")).toBeNull();
  });

  it("отказ: причина названа словами и есть чем повторить", () => {
    const obnovit = vi.fn();
    render(
      <Servery
        status={VYKL}
        spisok={null}
        spisokOtkaz={{ kod: "secrets-unreadable" }}
        obnovitSpisok={obnovit}
        naKomandu={vi.fn()}
      />,
    );
    expect(screen.getByText(/прежние серверы не читаются/i)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: /повтор/i }));
    expect(obnovit).toHaveBeenCalledTimes(1);
    // Отказ это ответ, а не ожидание: строка «загружается» уходит.
    expect(screen.queryByTestId("zagruzka")).toBeNull();
  });

  it("отказ не запирает вкладку: сервер можно добавить и без прочитанного списка", () => {
    render(
      <Servery status={VYKL} spisok={null} spisokOtkaz={{ kod: "secrets-unreadable" }} naKomandu={vi.fn()} />,
    );
    fireEvent.click(screen.getByTestId("dobavit"));
    expect(screen.getByTestId("ssylka")).toBeTruthy();
  });

  it("много: пятьсот серверов остаются внутри прокручиваемого списка", () => {
    risovat(spisok(Array.from({ length: 500 }, (_, i) => server(i + 1))));
    expect(screen.getAllByRole("option")).toHaveLength(500);
    const spisokEl = screen.getByRole("listbox");
    expect(spisokEl.className).toMatch(/max-h-\[60vh\]/);
    expect(spisokEl.className).toMatch(/overflow-y-auto/);
  });

  it("длинное: имя без пробелов не выезжает за колонку", () => {
    const dlinnoe = "сервер-" + "ы".repeat(200);
    risovat(spisok([server(1, { imya: dlinnoe })]));
    const imya = screen.getByTestId("imya-servera");
    expect(imya.className).toMatch(/truncate/);
    // shrink-0 на имени и был причиной наезда на теги и корзину.
    expect(imya.className).not.toMatch(/shrink-0/);
  });
});

/** Свойства для замеров: один сервер с id s1, всё остальное по умолчанию. */
function svoystva(pere: { naKomandu?: () => void; zaderzhki?: ZamerZaderzhki[] }) {
  return {
    status: VYKL,
    spisok: spisok([server(1, { id: "s1" })]),
    naKomandu: pere.naKomandu ?? vi.fn(),
    ...(pere.zaderzhki ? { zaderzhki: pere.zaderzhki } : {}),
  };
}

describe("серверы: замер задержки", () => {
  // Задача 8, части А и Б. До неё клиент знал одно число, задержку последней
  // пробы urltest, одну на всё подключение. Выбирать сервер по нему нельзя:
  // оно про тот сервер, который УЖЕ выбран.
  //
  // Чисел два. tcping это дорога до узла, и туннель для него не нужен, значит
  // кнопка полезна ДО подключения, то есть тогда, когда и надо выбирать.
  // realping это весь путь через туннель вместе с рукопожатием. Узел,
  // отвечающий на TCP мгновенно и не несущий ни байта, по одной цифре
  // неотличим от далёкого, но исправного.
  it("кнопка шлёт measureDelays", () => {
    const na = vi.fn();
    render(<Servery {...svoystva({ naKomandu: na })} />);
    fireEvent.click(screen.getByTestId("zamerit-zaderzhki"));
    expect(na).toHaveBeenCalledWith("measureDelays", {});
  });

  it("две задержки видны в строке своими числами", () => {
    render(
      <Servery
        {...svoystva({
          zaderzhki: [{ id: "s1", tcping_ms: 31, realping_ms: 92 }],
        })}
      />,
    );
    const r = screen.getByTestId("zaderzhka-s1");
    expect(r).toHaveTextContent("31");
    expect(r).toHaveTextContent("92");
  });

  it("неизмеренный realping это слово, а не ноль", () => {
    // Ноль на экране читается как «мгновенно» и ставит узел первым по
    // задержке, то есть наверх списка. Ровно наоборот тому, что есть.
    render(
      <Servery
        {...svoystva({
          zaderzhki: [{ id: "s1", tcping_ms: 31, realping_otkaz: "туннель опущен" }],
        })}
      />,
    );
    const r = screen.getByTestId("zaderzhka-s1");
    expect(r).toHaveTextContent("31");
    expect(r).not.toHaveTextContent(/\b0\b/);
    expect(r).toHaveTextContent(/туннель опущен|не измерен/);
  });

  it("молчащий узел показан отказом, а не пустым местом", () => {
    render(
      <Servery
        {...svoystva({
          zaderzhki: [{ id: "s1", tcping_otkaz: "узел не отвечает" }],
        })}
      />,
    );
    expect(screen.getByTestId("zaderzhka-s1")).toHaveTextContent(/узел не отвечает/);
  });

  it("без замера строка задержки не появляется вовсе", () => {
    render(<Servery {...svoystva({})} />);
    expect(screen.queryByTestId("zaderzhka-s1")).toBeNull();
  });
});
