import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Servery, type SpisokServerov } from "./Servery";
import type { Server, StatusOtvet } from "../protokol";

afterEach(cleanup);

// Область автовыбора (A5). Автомат перебирал все серверы набора, и из чего
// именно он выбирает, не говорил ни один экран. Теперь область названа вслух, а
// состав её человек меняет правой кнопкой по строке: переключателем в каждой
// строке этот вопрос не стоит того места, которое занял бы на главном списке.

function server(n: number, pere: Partial<Server> = {}): Server {
  return {
    id: `id${n}`, imya: `Сервер ${n}`, transport: "reality-tcp",
    host: `s${n}.example.net`, port: 443, iz_podpiski: true, ...pere,
  };
}

function spisok(servery: Server[]): SpisokServerov {
  return { servery, vybran: "", podpiska_zadana: true, podpiska_uzel: "panel.example.net" };
}

const VYKL: StatusOtvet = { sostoyanie: "vyklyuchen", rezhim_marshruta: "avto" };

function risovat(servery: Server[]) {
  const naKomandu = vi.fn();
  render(<Servery status={VYKL} spisok={spisok(servery)} naKomandu={naKomandu} />);
  return naKomandu;
}

function menyuPoPravoy(nomer: number) {
  fireEvent.contextMenu(screen.getAllByRole("option")[nomer]);
  return screen.getByTestId("menyu-servera");
}

describe("серверы: область автовыбора", () => {
  it("называет область вслух: сколько серверов и откуда", () => {
    risovat([server(1), server(2), server(3)]);
    const stroka = screen.getByTestId("oblast-avto");
    expect(stroka).toHaveTextContent(/все 3/);
    expect(stroka).toHaveTextContent(/активной подписки и добавленные вручную/);
    // Главное недоразумение, которое строка закрывает: запасная подписка лежит
    // в программе, а в автомат не входит, и понять это было неоткуда.
    expect(stroka).toHaveTextContent(/запасных подписок в него не входят/);
  });

  it("правая кнопка по строке убирает сервер из автовыбора одной командой", () => {
    const na = risovat([server(1), server(2)]);
    const menyu = menyuPoPravoy(1);
    fireEvent.click(within(menyu).getByRole("menuitem", { name: "Убрать из автовыбора" }));
    expect(na).toHaveBeenCalledTimes(1);
    expect(na).toHaveBeenCalledWith("setAutoMember", { id: "id2", uchastvuet: false });
  });

  it("убранный помечен в списке и возвращается тем же меню", () => {
    const na = risovat([server(1), server(2, { vne_avto: true })]);
    // Состояние, заданное человеком, видно без открытия меню: иначе «почему
    // автомат его не берёт» остаётся вопросом без ответа на экране.
    expect(screen.getByTestId("vne-avto-id2")).toHaveTextContent("не в автовыборе");
    expect(screen.getByTestId("oblast-avto")).toHaveTextContent(/1 из 2/);

    const menyu = menyuPoPravoy(1);
    fireEvent.click(within(menyu).getByRole("menuitem", { name: "Вернуть в автовыбор" }));
    expect(na).toHaveBeenCalledWith("setAutoMember", { id: "id2", uchastvuet: true });
  });

  it("последнего участника убрать нечем: пункт погашен", () => {
    // Пустой автомат это конфиг, который ядро отвергает целиком. Служба такое
    // и не примет, но давать нажать ради отказа незачем.
    risovat([server(1), server(2, { vne_avto: true })]);
    const punkt = within(menyuPoPravoy(0)).getByRole("menuitem", { name: "Убрать из автовыбора" });
    expect(punkt).toBeDisabled();
  });

  it("поиск прячет строки и не трогает область", () => {
    // Скрытие в списке это про экран, а не про политику: убранный поиском
    // сервер автомат по-прежнему перебирает, и число обязано это говорить.
    risovat([server(1), server(2), server(3)]);
    fireEvent.change(screen.getByTestId("poisk"), { target: { value: "Сервер 1" } });
    expect(screen.getAllByRole("option")).toHaveLength(1);
    expect(screen.getByTestId("oblast-avto")).toHaveTextContent(/все 3/);
  });

  it("меню открывается с клавиатуры и закрывается Escape", () => {
    const na = risovat([server(1), server(2)]);
    const stroka = screen.getAllByRole("option")[1];
    fireEvent.keyDown(stroka, { key: "F10", shiftKey: true });
    const menyu = screen.getByTestId("menyu-servera");
    // Фокус уезжает в меню сразу: иначе пройти его с клавиатуры нечем.
    expect(within(menyu).getByRole("menuitem", { name: "Убрать из автовыбора" })).toHaveFocus();
    fireEvent.keyDown(document, { key: "Escape" });
    expect(screen.queryByTestId("menyu-servera")).toBeNull();
    // Escape закрывает и ничего не делает: закрытие это не отмена и не
    // согласие.
    expect(na).not.toHaveBeenCalled();
  });

  it("правая кнопка не выбирает сервер", () => {
    // Щелчок левой кнопкой по строке это setServer. Открытие меню не должно
    // заодно переключать VPN на чужой сервер.
    const na = risovat([server(1), server(2)]);
    menyuPoPravoy(1);
    expect(na).not.toHaveBeenCalled();
  });
});
