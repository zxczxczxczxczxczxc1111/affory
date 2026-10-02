import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NAZVANIYA, OPISANIYA, PORYADOK, SpravkaProtokolov, VIDY, ZAGOLOVOK_CHUZHIE } from "./SpravkaProtokolov";
import { Servery, type SpisokServerov } from "./Servery";
import { Glavnyy } from "./Glavnyy";
import type { Server, StatusOtvet } from "../protokol";

afterEach(cleanup);

// Справка нужна ровно тем, кто не знает слов из мира машин, поэтому главная
// проверка здесь не про разметку, а про ЯЗЫК: термин, просочившийся в текст,
// отменяет смысл экрана целиком.

const VYKL: StatusOtvet = { sostoyanie: "vyklyuchen", rezhim_marshruta: "ruchnoy" };

function server(n: number, pere: Partial<Server> = {}): Server {
  return {
    id: `id${n}`, imya: `Сервер ${n}`, transport: "anytls",
    host: `s${n}.example.net`, port: 443, iz_podpiski: true, ...pere,
  };
}

function spisok(servery: Server[]): SpisokServerov {
  return { servery, vybran: "", podpiska_zadana: true, podpiska_uzel: "panel.example.net" };
}

describe("Справка о протоколах", () => {
  it("открывается значком у заголовка и закрывается крестиком", () => {
    render(<Servery status={VYKL} spisok={spisok([server(1)])} naKomandu={vi.fn()} podpiski={[]} />);
    expect(screen.queryByTestId("spravka-protokolov")).toBeNull();

    fireEvent.click(screen.getByTestId("spravka-protokolov-otkryt"));
    expect(screen.getByTestId("spravka-protokolov")).toBeTruthy();

    fireEvent.click(screen.getByTestId("spravka-protokolov-zakryt"));
    expect(screen.queryByTestId("spravka-protokolov")).toBeNull();
  });

  it("открывается с ГЛАВНОГО экрана, а не только с «Управлять»", () => {
    // Первая редакция 19.09.2026 повесила значок на экран Servery, а список
    // серверов человек читает на главном: значка он не видел ни разу, и
    // тесты этого не ловили, потому что проверяли тот экран, куда значок и
    // поставлен.
    render(<Glavnyy status={VYKL} servery={[server(1, { transport: "hy2" })]} />);
    fireEvent.click(screen.getByTestId("spravka-protokolov-otkryt"));
    expect(screen.getByTestId("spravka-protokolov")).toBeTruthy();
  });

  it("заголовком идёт сам транспорт, слово в слово как в строке сервера", () => {
    // «hy2» в списке и «hysteria2» в справке это два разных слова для одного
    // ключа, и связать их человеку нечем.
    render(<SpravkaProtokolov zakryt={vi.fn()} />);
    const zagolovki = [...screen.getByTestId("spravka-protokolov").querySelectorAll("dt")]
      .map((dt) => (dt.textContent ?? "").split(" (")[0].trim());
    expect(zagolovki.sort()).toEqual(Object.keys(OPISANIYA).sort());
  });

  it("свои протоколы идут выше чужих", () => {
    render(<SpravkaProtokolov zakryt={vi.fn()} svoi={["hy2", "trojan-ws"]} />);
    const okno = screen.getByTestId("spravka-protokolov");
    const mesto = (s: string) => okno.textContent?.indexOf(s) ?? -1;
    const granica = mesto(ZAGOLOVOK_CHUZHIE);
    expect(granica).toBeGreaterThan(0);
    // trojan-ws сведён к trojan: отдельного описания у него нет.
    expect(mesto("hy2")).toBeLessThan(granica);
    expect(mesto("trojan")).toBeLessThan(granica);
    expect(mesto("vmess")).toBeGreaterThan(granica);
  });

  it("описывает, а не советует", () => {
    // До 02.10.2026 справка звала «бери tuic» по замерам одной машины, а у
    // другого провайдера тот же tuic режут. Какой протокол быстрее, зависит
    // от сети человека, и справка этого знать не может (решение владельца).
    const sovety = ["бери", "пробуй", "лучш", "худш", "самый", "самая", "рекоменд", "совет"];
    const tekst = [
      ...Object.values(OPISANIYA).map((o) => `${o.kak} ${o.sledstvie}`),
      ...VIDY.map((v) => v.tekst),
    ].join(" ").toLowerCase();
    for (const slovo of sovety) {
      expect(tekst.includes(slovo), slovo).toBe(false);
    }
  });

  it("порядок идёт по виду, как в пояснении сверху", () => {
    // Сначала протоколы, передающие пакетами, затем потоком: человек читает
    // пояснение о двух видах и находит их ниже в том же порядке.
    const paketami = VIDY[0].kto.split(", ");
    const posledniyPaket = Math.max(...paketami.map((t) => PORYADOK.indexOf(t)));
    const pervyyPotok = Math.min(...PORYADOK.filter((t) => !paketami.includes(t)).map((t) => PORYADOK.indexOf(t)));
    expect(paketami.every((t) => PORYADOK.includes(t))).toBe(true);
    expect(posledniyPaket).toBeLessThan(pervyyPotok);
    render(<SpravkaProtokolov zakryt={vi.fn()} />);
    expect(screen.getByTestId("spravka-vidy")).toHaveTextContent(/Пакетами \(tuic, hy2\)/);
  });

  it("закрывается по Escape", () => {
    const zakryt = vi.fn();
    render(<SpravkaProtokolov zakryt={zakryt} />);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(zakryt).toHaveBeenCalledTimes(1);
  });

  it("описывает каждый транспорт, который может встретиться в списке", () => {
    // Транспорты живой подписки на 19.09.2026 и reality поверх grpc, которым
    // подписка заменила reality 26.09.2026. Ключ без описания оставляет
    // человека ровно с тем вопросом, ради которого эта справка и открыта.
    for (const t of ["anytls", "trojan", "reality-tcp", "reality-grpc", "hy2", "tuic", "httpupgrade"]) {
      expect(OPISANIYA[t], t).toBeTruthy();
    }
  });

  it("говорит человеческими словами, без терминов", () => {
    // Список закрыт нарочно: правило «пиши проще» без проверки живёт ровно до
    // первой правки. Те же слова запрещены в пояснениях окна (13.09.2026).
    const zapreshcheno = [
      "UDP", "TCP", "QUIC", "MTU", "DNS", "TLS", "шифров", "рукопожат",
      "датаграмм", "мультиплекс", "инкапсул", "обфускац", "джиттер", "трафик",
    ];
    // Названия самих ключей вычёркиваются до проверки: человек видит их в
    // списке серверов, и назвать их в справке можно. Иначе сторож ловит «tls»
    // внутри «anytls» и запрещает ссылаться на соседний ключ по имени.
    // Длинные имена вперёд, чтобы короткое не разрезало длинное пополам, и
    // замена на пробел, чтобы склейка не породила запрещённое слово.
    const imena = [...Object.keys(OPISANIYA), ...Object.keys(NAZVANIYA), ...Object.values(NAZVANIYA)]
      .sort((a, b) => b.length - a.length);
    let tekst = [...Object.values(OPISANIYA).map((o) => `${o.kak} ${o.sledstvie}`), ...VIDY.map((v) => v.tekst)]
      .join(" ").toLowerCase();
    for (const imya of imena) tekst = tekst.split(imya.toLowerCase()).join(" ");
    for (const slovo of zapreshcheno) {
      expect(tekst.includes(slovo.toLowerCase()), slovo).toBe(false);
    }
  });

  it("у каждого протокола названо устройство и что из него следует", () => {
    // Одно устройство без следствия человеку ничего не говорит, одно
    // следствие без устройства читается как обещание. Сторож стоит против
    // пустой строки и отписки, а не против краткости.
    for (const [t, o] of Object.entries(OPISANIYA)) {
      expect(o.kak.length, t).toBeGreaterThan(15);
      expect(o.sledstvie.length, t).toBeGreaterThan(15);
    }
  });
});
