import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Nastroyki } from "./Nastroyki";
import type { StatusOtvet } from "../protokol";

afterEach(cleanup);

// Task 4.8: two switches for two decisions (§9.2). Flipping one must send
// exactly its own command and never touch the other: a combined switch would
// promise a tunnel the human did not ask for.
function status(pere: Partial<StatusOtvet> = {}): StatusOtvet {
  return { sostoyanie: "vyklyuchen", avtozapusk: true, podklyuchat_pri_starte: false, ...pere };
}

describe("настройки: автозапуск и подключение при старте", () => {
  it("показывает умолчания §9.2: автозапуск включён, подключение при старте выключено", () => {
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    expect((screen.getByTestId("avtozapusk") as HTMLInputElement).checked).toBe(true);
    expect((screen.getByTestId("pri-starte") as HTMLInputElement).checked).toBe(false);
  });

  it("переключение «подключаться при старте» шлёт только setConnectOnStart", () => {
    const naKomandu = vi.fn();
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={naKomandu} naUdalenie={vi.fn()} />);
    fireEvent.click(screen.getByTestId("pri-starte"));
    expect(naKomandu).toHaveBeenCalledTimes(1);
    expect(naKomandu).toHaveBeenCalledWith("setConnectOnStart", { vkl: true });
  });

  it("переключение автозапуска шлёт только setAutostart с обратным значением", () => {
    const naKomandu = vi.fn();
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={naKomandu} naUdalenie={vi.fn()} />);
    fireEvent.click(screen.getByTestId("avtozapusk"));
    expect(naKomandu).toHaveBeenCalledTimes(1);
    expect(naKomandu).toHaveBeenCalledWith("setAutostart", { vkl: false });
  });

  it("пока служба молчит, переключатели неактивны, а не врут умолчанием", () => {
    render(<Nastroyki status={{ sostoyanie: "sluzhba-molchit" }} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    expect((screen.getByTestId("avtozapusk") as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByTestId("pri-starte") as HTMLInputElement).disabled).toBe(true);
  });
});

// Task 4.11. The §9.2 defaults are SHOWN, not implied; switching the
// "all traffic" mode re-raises the tunnel and must warn before sending;
// deferred commands (leak check, exit address, update) take their wave from
// hello and stay disabled with the reason beside them; the local proxy port
// is visible, because a busy port silently drops the proxy today.
const OTLOZHENO = { checkLeaks: 6, checkExitIp: 6, installUpdate: 6, setJournal: 6, clearJournal: 6 };

function polnyy(pere: Partial<StatusOtvet> = {}) {
  const naKomandu = vi.fn<(komanda: string, telo: unknown) => void>();
  render(
    <Nastroyki
      status={{ sostoyanie: "podnyat", avtozapusk: true, podklyuchat_pri_starte: false, kill_switch: false, port_proksi: 10809, ...pere }}
      otlozheno={OTLOZHENO}
      naKomandu={naKomandu}
      naUdalenie={vi.fn()}
    />,
  );
  return naKomandu;
}

describe("настройки: полоса канала", () => {
  // 05.09.2026. Полоса это ЕДИНСТВЕННЫЙ способ включить Brutal у hysteria2:
  // отдельного флага у протокола нет, пустые поля означают BBR. При этом
  // завышенное число хуже отсутствующего: замер дал восемь провалов и p95
  // 3214 мс там, где честное объявление держало ноль и 92 мс.
  // Текст переписан 05.09.2026, когда разбор научился брать полосу ИЗ ССЫЛКИ.
  // Прежнее «ядро считает полосу само» стало неправдой: у сервера, чья ссылка
  // несёт upmbps и downmbps, Brutal включится и с пустыми полями. Вход
  // hy2-brutal нашей же подписки ровно такой.
  it("без настройки сказано, что число берётся из ссылки, а не показан ноль", () => {
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    const r = screen.getByTestId("polosa");
    expect(r).toHaveTextContent(/из ссылки/);
    expect(r).not.toHaveTextContent(/ядро считает полосу само/);
    expect(r).not.toHaveTextContent(/\b0\b/);
  });

  it("объявленная полоса видна числами", () => {
    render(<Nastroyki status={status({ polosa_vverh: 250, polosa_vniz: 440 })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    const r = screen.getByTestId("polosa");
    expect(r).toHaveTextContent("250");
    expect(r).toHaveTextContent("440");
  });

  // Часть В задачи 8. Число для объявления hysteria2 обязано браться замером,
  // а не полем ввода: Brutal шлёт ровно с объявленной скоростью, и цену
  // завышения платит канал человека, который своей полосы не знает.
  it("кнопка замера шлёт measureBandwidth обеими мишенями", () => {
    // Мишени две, потому что направления живут по разным адресам: у
    // speed.cloudflare.com это __down и __up. Одно поле на оба означало бы,
    // что отдача меряется туда же, куда приём, и получает 405.
    const na = vi.fn();
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={na} naUdalenie={vi.fn()} />);
    fireEvent.change(screen.getByTestId("polosa-mishen"), {
      target: { value: "https://example.org/big.bin" },
    });
    fireEvent.change(screen.getByTestId("polosa-mishen-vverh"), {
      target: { value: "https://example.org/__up" },
    });
    fireEvent.click(screen.getByTestId("izmerit-polosu"));
    expect(na).toHaveBeenCalledWith(
      "measureBandwidth",
      expect.objectContaining({
        adres: "https://example.org/big.bin",
        adres_vverh: "https://example.org/__up",
      }),
    );
  });

  // Кнопка, не меняющая ничего на экране, читается как сломанная (решено
  // 03.09.2026). Замер тратит настоящий трафик, поэтому молчащий результат
  // здесь дороже обычного: человек нажмёт ещё раз.
  it("измеренное показывается числами и советом", () => {
    render(
      <Nastroyki
        status={status()}
        otlozheno={{}}
        naKomandu={vi.fn()}
        naUdalenie={vi.fn()}
        zamerPolosy={{
          mbitVniz: 87.4, sovetVniz: 78,
          mbitVverh: 27.1, sovetVverh: 24,
          otkazVverh: "", cherezTunnel: true, vremya: "21:48",
        }}
      />,
    );
    const r = screen.getByTestId("zamer-polosy");
    expect(r).toHaveTextContent("87");
    expect(r).toHaveTextContent("27");
    expect(r).toHaveTextContent("78");
    expect(r).toHaveTextContent("24");
    expect(r).toHaveTextContent("21:48");
  });

  it("кнопка объявления шлёт setBandwidth ровно советами, а не замером", () => {
    // Запас вниз и есть весь смысл совета: Brutal шлёт ровно с объявленной
    // скоростью, и объявленные 87 вместо 78 стоили бы 10% полосы и 30%
    // задержки (замерено на стенде).
    const na = vi.fn();
    render(
      <Nastroyki
        status={status()}
        otlozheno={{}}
        naKomandu={na}
        naUdalenie={vi.fn()}
        zamerPolosy={{
          mbitVniz: 87.4, sovetVniz: 78,
          mbitVverh: 27.1, sovetVverh: 24,
          otkazVverh: "", cherezTunnel: true, vremya: "21:48",
        }}
      />,
    );
    fireEvent.click(screen.getByTestId("obyavit-polosu"));
    expect(na).toHaveBeenCalledWith("setBandwidth", { vverh: 24, vniz: 78 });
  });

  it("без измеренной отдачи объявлять нечем, и причина названа", () => {
    // setBandwidth требует ОБЕ стороны, и подставить приём вместо отдачи
    // нельзя: у домашнего канала отдача обычно втрое ниже, а завышение любой
    // стороны ломает Brutal одинаково.
    render(
      <Nastroyki
        status={status()}
        otlozheno={{}}
        naKomandu={vi.fn()}
        naUdalenie={vi.fn()}
        zamerPolosy={{
          mbitVniz: 87.4, sovetVniz: 78,
          mbitVverh: null, sovetVverh: null,
          otkazVverh: "мишень не принимает заливку, ответив 405",
          cherezTunnel: false, vremya: "21:48",
        }}
      />,
    );
    expect((screen.getByTestId("obyavit-polosu") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("zamer-polosy")).toHaveTextContent("405");
  });

  it("без мишени замер не уходит вовсе", () => {
    // Мишень задаётся явно: клиент не ходит самовольно на чужой хост и не
    // тратит трафик мобильного тарифа без спроса.
    const na = vi.fn();
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={na} naUdalenie={vi.fn()} />);
    fireEvent.click(screen.getByTestId("izmerit-polosu"));
    expect(na).not.toHaveBeenCalled();
  });

  it("сохранение шлёт setBandwidth парой чисел", () => {
    const na = vi.fn();
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={na} naUdalenie={vi.fn()} />);
    fireEvent.change(screen.getByTestId("polosa-vverh"), { target: { value: "250" } });
    fireEvent.change(screen.getByTestId("polosa-vniz"), { target: { value: "440" } });
    fireEvent.click(screen.getByTestId("sohranit-polosu"));
    expect(na).toHaveBeenCalledWith("setBandwidth", { vverh: 250, vniz: 440 });
  });

  it("половина пары не отправляется: одно число означало бы нулевое второе", () => {
    const na = vi.fn();
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={na} naUdalenie={vi.fn()} />);
    fireEvent.change(screen.getByTestId("polosa-vniz"), { target: { value: "440" } });
    expect(screen.getByTestId("sohranit-polosu")).toBeDisabled();
    fireEvent.click(screen.getByTestId("sohranit-polosu"));
    expect(na).not.toHaveBeenCalled();
  });

  it("снятие шлёт пару нулей, и только когда полоса объявлена", () => {
    const na = vi.fn();
    render(<Nastroyki status={status({ polosa_vverh: 250, polosa_vniz: 440 })} otlozheno={{}} naKomandu={na} naUdalenie={vi.fn()} />);
    fireEvent.click(screen.getByTestId("snyat-polosu"));
    expect(na).toHaveBeenCalledWith("setBandwidth", { vverh: 0, vniz: 0 });
  });

  it("человека предупреждают, что завышенное число вредит", () => {
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    expect(screen.getByTestId("polosa")).toHaveTextContent(/завышенн/i);
  });
});

describe("настройки: защита", () => {
  it("умолчание §9.2 видно: kill-switch выключен", () => {
    polnyy();
    expect((screen.getByTestId("ves-trafik") as HTMLInputElement).checked).toBe(false);
  });

  it("смена режима «весь трафик» не уходит в службу без подтверждения про разрыв соединений", () => {
    const na = polnyy();
    fireEvent.click(screen.getByTestId("ves-trafik"));
    expect(na).not.toHaveBeenCalled();
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveTextContent(/соединения (рвутся|разорвутся)/);
    fireEvent.click(screen.getByTestId("podtverdit-rezhim"));
    expect(na).toHaveBeenCalledTimes(1);
    expect(na).toHaveBeenCalledWith("setKillSwitch", { vkl: true });
  });

  it("отмена в подтверждении ничего не шлёт и закрывает вопрос", () => {
    const na = polnyy();
    fireEvent.click(screen.getByTestId("ves-trafik"));
    fireEvent.click(screen.getByTestId("otmena-rezhima"));
    expect(na).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("защита сохраняется при выключенном VPN без переподключения", () => {
    const send = polnyy({ sostoyanie: "vyklyuchen" });
    expect(screen.getByTestId("ves-trafik")).toBeEnabled();
    fireEvent.click(screen.getByTestId("ves-trafik"));
    expect(send).toHaveBeenCalledWith("setKillSwitch", { vkl: true });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("объясняет, что отключение сферы освобождает сеть", () => {
    polnyy({ sostoyanie: "podnyat" });
    expect(screen.getByTestId("ves-trafik-ryad")).toHaveTextContent("После отключения сферы сеть освобождается");
  });
  it("порт локального прокси показан, а без порта сказано, что прокси не поднят", () => {
    polnyy({ port_proksi: 10809 });
    expect(screen.getByTestId("proksi")).toHaveTextContent("127.0.0.1:10809");
    cleanup();
    polnyy({ port_proksi: undefined });
    expect(screen.getByTestId("proksi")).toHaveTextContent(/не поднят/);
    expect(screen.getByTestId("proksi")).not.toHaveTextContent(/\b0\b/);
  });
});

describe("настройки: отложенные команды", () => {
  it("проверка утечек, адрес выхода и обновление неактивны с номером волны из hello", () => {
    polnyy();
    for (const id of ["proverit-utechki", "proverit-adres", "proverit-obnovlenie"]) {
      expect((screen.getByTestId(id) as HTMLButtonElement).disabled).toBe(true);
    }
    expect(screen.getByTestId("proverka")).toHaveTextContent(/волне 6/);
  });

  it("номер волны не свой: другая карта, другое число", () => {
    render(
      <Nastroyki
        status={{ sostoyanie: "podnyat" }}
        otlozheno={{ checkLeaks: 8, checkExitIp: 8, installUpdate: 8 }}
        naKomandu={vi.fn()}
        naUdalenie={vi.fn()}
      />,
    );
    expect(screen.getByTestId("proverka")).toHaveTextContent(/волне 8/);
    expect(screen.queryByText(/волне 6/)).toBeNull();
  });

  it("ответ checkExitIp показывается в строке: адрес, путь и время, а не молчание", () => {
    // Owner, 03.09.2026: pressing "Проверить" did nothing visible, because
    // the answer only refreshed a number that was already on the main screen.
    const { rerender } = render(
      <Nastroyki status={{ sostoyanie: "podnyat" }} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        adresVyhoda={{ adres: "203.0.113.5", cherez: "tunnel", vremya: "12:03" }} />,
    );
    expect(screen.getByTestId("adres-vyhoda")).toHaveTextContent(/203\.0\.113\.5/);
    expect(screen.getByTestId("adres-vyhoda")).toHaveTextContent(/через туннель/);
    expect(screen.getByTestId("adres-vyhoda")).toHaveTextContent(/12:03/);
    rerender(
      <Nastroyki status={{ sostoyanie: "vyklyuchen" }} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        adresVyhoda={{ adres: "198.51.100.7", cherez: "napryamuyu", vremya: "12:04" }} />,
    );
    expect(screen.getByTestId("adres-vyhoda")).toHaveTextContent(/напрямую/);
    expect(screen.getByTestId("adres-vyhoda")).toHaveTextContent(/198\.51\.100\.7/);
  });

  it("когда checkExitIp реализована, кнопка активна и шлёт команду по нажатию, не по таймеру", () => {
    const na = vi.fn<(komanda: string, telo: unknown) => void>();
    render(
      <Nastroyki status={{ sostoyanie: "podnyat" }} otlozheno={{}} naKomandu={na} naUdalenie={vi.fn()} />,
    );
    expect(na).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("proverit-adres"));
    expect(na).toHaveBeenCalledWith("checkExitIp", {});
  });
});

// Task 6.3: the leak check names what it cannot see, and the screen shows
// every line, not a single green light.
describe("настройки: результат проверки утечек", () => {
  it("без ответа списка нет", () => {
    render(<Nastroyki status={status()} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    expect(screen.queryByTestId("punkty")).toBeNull();
  });

  it("рисует каждый пункт с итогом, включая «не проверяется»", () => {
    const proverka = {
      punkty: [
        { imya: "адрес выхода", itog: "ok" as const, tekst: "интернет видит 192.0.2.10" },
        { imya: "DoH браузера", itog: "ne_vidim" as const, tekst: "браузер резолвит сам" },
        { imya: "IPv6", itog: "utechka" as const, tekst: "правила нет" },
      ],
    };
    render(<Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} proverka={proverka} />);
    const punkty = screen.getByTestId("punkty");
    expect(punkty.querySelectorAll("li").length).toBe(3);
    expect(punkty.textContent).toContain("не проверяется");
    expect(punkty.textContent).toContain("утечка");
    expect(punkty.querySelector('[data-itog="utechka"]')).not.toBeNull();
  });
});

describe("настройки: обновление", () => {
  it("когда installUpdate реализована, кнопка зовёт выбор архива, а не пустую команду", () => {
    // Wave 6.5: the path comes from a native file dialog on the Go side.
    // An empty installUpdate would only earn an update-archive-invalid refusal.
    const naKomandu = vi.fn();
    const naObnovlenie = vi.fn();
    render(<Nastroyki status={{ sostoyanie: "vyklyuchen" }} otlozheno={{}} naKomandu={naKomandu} naUdalenie={vi.fn()} naObnovlenie={naObnovlenie} />);
    const knopka = screen.getByTestId("proverit-obnovlenie") as HTMLButtonElement;
    expect(knopka.disabled).toBe(false);
    fireEvent.click(knopka);
    expect(naObnovlenie).toHaveBeenCalledTimes(1);
    expect(naKomandu).not.toHaveBeenCalledWith("installUpdate", expect.anything());
  });
});

// План «шесть удобств» §5: служба проверяет сервер обновлений сама раз в
// сутки, строка показывает версию и итог проверки, кнопка ставит найденное.
describe("настройки: обновление с сервера", () => {
  it("без находки: версия программы, отметка проверки и кнопка «Проверить» зовёт checkUpdate", () => {
    const naKomandu = polnyy({ versiya_programmy: "0.6.2", obnovlenie_provereno: "2026-09-03T10:00:00Z" });
    const ryad = screen.getByTestId("obnovlenie");
    expect(ryad).toHaveTextContent(/0\.6\.2/);
    expect(ryad).toHaveTextContent(/проверено/);
    fireEvent.click(screen.getByTestId("proverit-versiyu"));
    expect(naKomandu).toHaveBeenCalledWith("checkUpdate", {});
  });

  it("с находкой: «есть 0.6.3» с размером и кнопка «Установить» зовёт downloadUpdate", () => {
    const naKomandu = polnyy({ versiya_programmy: "0.6.2", obnovlenie: { versiya: "0.6.3", razmer: 9244901, provereno: "2026-09-03T10:00:00Z" } });
    const ryad = screen.getByTestId("obnovlenie");
    expect(ryad).toHaveTextContent(/есть 0\.6\.3/);
    expect(ryad).toHaveTextContent(/8,8 МБ/);
    fireEvent.click(screen.getByTestId("ustanovit-obnovlenie"));
    expect(naKomandu).toHaveBeenCalledWith("downloadUpdate", {});
  });

  it("ни разу не проверялось: так и написано, кнопка проверки есть", () => {
    polnyy({ versiya_programmy: "0.6.2" });
    expect(screen.getByTestId("obnovlenie")).toHaveTextContent(/ещё не проверялось/);
    expect(screen.getByTestId("proverit-versiyu")).toBeTruthy();
  });
});

// Правило полосы Е: четыре состояния у каждого экрана. Обход интерфейса
// 03.09.2026: кнопки «Проверить», «Проверить версию» и «Выбрать архив»
// сереют БЕЗ единого слова о причине, а неудачная checkLeaks оставляет на
// экране ПРОШЛЫЙ успешный результат.
const REZULTAT = {
  punkty: [
    { imya: "адрес выхода", itog: "ok" as const, tekst: "интернет видит 203.0.113.10" },
    { imya: "IPv6", itog: "utechka" as const, tekst: "правила нет" },
  ],
};

describe("настройки: четыре состояния", () => {
  it("пусто: проверок не было, и экран не рисует ни результата, ни нулей", () => {
    render(<Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    expect(screen.queryByTestId("punkty")).toBeNull();
    expect(screen.getByTestId("adres-vyhoda")).toHaveTextContent(/по кнопке/);
    expect(screen.getByTestId("proverka").textContent).not.toMatch(/\b0\b/);
  });

  it("отказ: причина названа, и прошлый успешный результат не выдаётся за текущий", () => {
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        proverka={REZULTAT} proverkaOtkaz={{ kod: "exit-ip-unmeasured" }} />,
    );
    expect(screen.getByTestId("otkaz-proverki")).toHaveTextContent(/адрес выхода не измерен/);
    expect(screen.queryByTestId("punkty")).toBeNull();
    expect(screen.queryByText(/интернет видит/)).toBeNull();
  });

  it("много: сорок пунктов проверки остаются внутри прокручиваемого списка", () => {
    const punkty = Array.from({ length: 40 }, (_, i) => ({ imya: `пункт ${i + 1}`, itog: "ok" as const, tekst: "в порядке" }));
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        proverka={{ punkty }} />,
    );
    const spisokEl = screen.getByTestId("punkty");
    expect(spisokEl.className).toMatch(/overflow-y-auto/);
    expect(spisokEl.querySelectorAll("li").length).toBe(40);
  });

  it("длинное: адрес выхода без пробелов не выезжает за колонку", () => {
    const dlinnyy = "exit-" + "n".repeat(200) + ".example.net";
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        adresVyhoda={{ adres: dlinnyy, cherez: "tunnel", vremya: "12:03" }} />,
    );
    expect(screen.getByText(new RegExp(dlinnyy)).className).toMatch(/break-words/);
  });
});

describe("настройки: серая кнопка называет причину", () => {
  it("пока службы нет, у каждой заблокированной кнопки сказано, почему она серая", () => {
    const povtorit = vi.fn();
    render(
      <Nastroyki status={{ sostoyanie: "sluzhba-molchit" }} svyaz="net" povtorit={povtorit}
        otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />,
    );
    for (const id of ["proverit-utechki", "proverit-adres", "proverit-versiyu", "proverit-obnovlenie"]) {
      expect(screen.getByTestId(id)).toBeDisabled();
    }
    for (const id of ["proverka", "adres-vyhoda", "obnovlenie", "arhiv-sborki"]) {
      expect(screen.getByTestId(id)).toHaveTextContent(/служба не отвечает/);
    }
    fireEvent.click(screen.getByTestId("povtorit-svyaz"));
    expect(povtorit).toHaveBeenCalledTimes(1);
  });

  it("пока hello не ответил, причина другая и волну никто не выдумывает", () => {
    render(<Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={null} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    expect(screen.getByTestId("proverit-utechki")).toBeDisabled();
    expect(screen.getByTestId("proverka")).toHaveTextContent(/служба ещё не ответила/);
    expect(screen.queryByText(/волне/)).toBeNull();
  });
});

describe("настройки: свежесть результата", () => {
  it("результат помечен временем проверки", () => {
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        proverka={REZULTAT} />,
    );
    expect(screen.getByTestId("proverka-snyata")).toHaveTextContent(/\d\d:\d\d/);
  });

  it("не показывает прошлый результат проверки как текущий: туннель сменился, результат погас", () => {
    const { rerender } = render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        proverka={REZULTAT} />,
    );
    expect(screen.getByTestId("punkty")).toBeTruthy();
    rerender(
      <Nastroyki status={status({ sostoyanie: "vyklyuchen" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        proverka={REZULTAT} />,
    );
    expect(screen.queryByTestId("punkty")).toBeNull();
    expect(screen.getByTestId("proverka-ustarela")).toHaveTextContent(/проверь заново/);
  });

  it("«через туннель» не висит после отключения", () => {
    render(
      <Nastroyki status={status({ sostoyanie: "vyklyuchen" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        adresVyhoda={{ adres: "203.0.113.5", cherez: "tunnel", vremya: "12:03" }} />,
    );
    expect(screen.getByTestId("adres-vyhoda")).not.toHaveTextContent(/через туннель/);
    expect(screen.getByTestId("adres-vyhoda")).toHaveTextContent(/туннель с тех пор опущен/);
  });
});

describe("настройки: профиль", () => {
  it("пароль вводится на экране и уходит обработчику, а не в App", () => {
    // The password field belongs next to the buttons it serves. It used to
    // live in App.tsx as a stand-in until this screen could take the props.
    const vyvesti = vi.fn();
    const vvesti = vi.fn();
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        vyvestiProfil={vyvesti} vvestiProfil={vvesti} />,
    );
    // Empty password sends nothing: the service would refuse it anyway, and a
    // refusal the screen could have predicted is a refusal it should not cause.
    expect(screen.getByTestId("vyvesti-profil")).toBeDisabled();
    expect(screen.getByTestId("vvesti-profil")).toBeDisabled();
    fireEvent.change(screen.getByTestId("parol-profilya"), { target: { value: "хорошийпароль" } });
    fireEvent.click(screen.getByTestId("vyvesti-profil"));
    fireEvent.click(screen.getByTestId("vvesti-profil"));
    expect(vyvesti).toHaveBeenCalledWith("хорошийпароль");
    expect(vvesti).toHaveBeenCalledWith("хорошийпароль");
  });

  it("поле пароля это поле пароля, а не открытый текст", () => {
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        vyvestiProfil={vi.fn()} vvestiProfil={vi.fn()} />,
    );
    expect(screen.getByTestId("parol-profilya")).toHaveAttribute("type", "password");
  });

  it("итог прошлой попытки виден рядом с кнопкой, а не только в баннере", () => {
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        vyvestiProfil={vi.fn()} vvestiProfil={vi.fn()} itogProfilya="профиль записан: D:\profil.affory" />,
    );
    expect(screen.getByTestId("itog-profilya")).toHaveTextContent(/профиль записан/);
  });

  it("пока hello не ответил, кнопки профиля серые, а не только объяснение под ними", () => {
    // otlozheno === null means hello has not answered, so the row already
    // SAYS the service is silent. A button that still clicks under that
    // sentence makes the sentence a lie, and the human trusts the button.
    render(
      <Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={null} naKomandu={vi.fn()} naUdalenie={vi.fn()}
        vyvestiProfil={vi.fn()} vvestiProfil={vi.fn()} />,
    );
    expect(screen.getByTestId("vyvesti-profil")).toBeDisabled();
    expect(screen.getByTestId("vvesti-profil")).toBeDisabled();
  });

  it("без обработчиков раздела профиля нет: пустая кнопка обещала бы то, чего нет", () => {
    render(<Nastroyki status={status({ sostoyanie: "podnyat" })} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} />);
    expect(screen.queryByTestId("vyvesti-profil")).toBeNull();
  });
});
