import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NAZVANIYA, OPISANIYA, SpravkaProtokolov } from "./SpravkaProtokolov";
import { Servery, type SpisokServerov } from "./Servery";
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

  it("закрывается по Escape", () => {
    const zakryt = vi.fn();
    render(<SpravkaProtokolov zakryt={zakryt} />);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(zakryt).toHaveBeenCalledTimes(1);
  });

  it("описывает каждый транспорт, который может встретиться в списке", () => {
    // Транспорты живой подписки на 19.09.2026. Ключ без описания оставляет
    // человека ровно с тем вопросом, ради которого эта справка и открыта.
    for (const t of ["anytls", "trojan", "reality-tcp", "hy2", "tuic", "httpupgrade"]) {
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
    const imena = [...Object.keys(NAZVANIYA), ...Object.values(NAZVANIYA)]
      .sort((a, b) => b.length - a.length);
    let tekst = Object.values(OPISANIYA).map((o) => `${o.horosho} ${o.ceny}`).join(" ").toLowerCase();
    for (const imya of imena) tekst = tekst.split(imya.toLowerCase()).join(" ");
    for (const slovo of zapreshcheno) {
      expect(tekst.includes(slovo.toLowerCase()), slovo).toBe(false);
    }
  });

  it("у каждого протокола названа и польза, и цена", () => {
    // Описание из одних достоинств это реклама, а не справка: выбрать по ней
    // нельзя, потому что выбор это всегда обмен одного на другое.
    for (const [t, o] of Object.entries(OPISANIYA)) {
      expect(o.horosho.length, t).toBeGreaterThan(20);
      expect(o.ceny.length, t).toBeGreaterThan(20);
    }
  });
});
