import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { KatalogServerov, perenestiIdServerov } from "./KatalogServerov";
import type { Server } from "../protokol";

const server = (id: string, podpiska = true): Server => ({ id, imya: id, host: `${id}.example`, port: 443, transport: "trojan", iz_podpiski: podpiska });
const props = {
  servery: [server("a"), server("manual", false), { ...server("old"), uderzhan: true }],
  podpiski: [{ id: "A", uzel: "A.example", aktivnaya: true }, { id: "B", uzel: "B.example", aktivnaya: false, servery: [server("b")] }],
  zapros: "", zaderzhki: [], podnyat: true, nesushchiy: "a", disabled: false,
};
beforeEach(() => localStorage.clear());
afterEach(cleanup);

it("миграция конфликтующего ID сохраняет закрепление своего источника", () => {
  localStorage.setItem("affory.server-view.v1", JSON.stringify({zakrepleny:["B:b","ruchnye:b"],skryty:["A"],svernuty:["B"]}));
  const view=render(<KatalogServerov {...props}/>);
  act(()=>perenestiIdServerov("B",{b:"new-b"}));
  view.rerender(<KatalogServerov {...props} podpiski={[props.podpiski[0],{...props.podpiski[1],servery:[{...server("b"),id:"new-b"}]}]}/>);
  expect(screen.getByRole("button",{name:"Открепить b"})).toBeTruthy();
  expect(JSON.parse(localStorage.getItem("affory.server-view.v1")!)).toEqual({zakrepleny:["B:new-b","ruchnye:b"],skryty:["A"],svernuty:["B"]});
});

it("ручные отдельно, старые не видны, закрепление не дублирует строку и переживает перезапуск", () => {
  const first = render(<KatalogServerov {...props} />);
  expect(within(screen.getByRole("region", { name: "Добавлены вручную" })).getByText("manual")).toBeTruthy();
  expect(screen.queryByText("old")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Закрепить manual" }));
  expect(screen.getAllByText("manual")).toHaveLength(1);
  expect(within(screen.getByRole("region", { name: "Закреплённые" })).getByText("manual")).toBeTruthy();
  first.unmount();
  render(<KatalogServerov {...props} />);
  expect(screen.getByRole("button", { name: "Открепить manual" })).toBeTruthy();
});

it("скрытие не вызывает команд подключения, закреплённый остаётся доступен", () => {
  const choose = vi.fn();
  render(<KatalogServerov {...props} naVybor={choose} />);
  fireEvent.click(screen.getByRole("button", { name: "Закрепить a" }));
  fireEvent.click(screen.getByRole("button", { name: "Скрыть подписку A.example" }));
  expect(screen.queryByRole("region", { name: "A.example" })).toBeNull();
  expect(screen.getByRole("button", { name: "Подключиться к a" })).toBeTruthy();
  expect(choose).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Скрытые подписки · 1" }));
  fireEvent.click(screen.getByRole("button", { name: "Показать A.example" }));
  expect(screen.getByRole("region", { name: "A.example" })).toBeTruthy();
});

it("поиск раскрывает свёрнутую группу, выбор запасного передаёт источник", () => {
  const choose = vi.fn();
  const { rerender } = render(<KatalogServerov {...props} naVybor={choose} />);
  fireEvent.click(screen.getByRole("button", { name: "B.example · 1 сервер" }));
  expect(screen.queryByRole("button", { name: "Подключиться к b" })).toBeNull();
  rerender(<KatalogServerov {...props} zapros="b.example" naVybor={choose} />);
  fireEvent.click(screen.getByRole("button", { name: "Подключиться к b" }));
  expect(choose).toHaveBeenCalledWith("b", "B");
});

it("одинаковые ID из разных источников имеют независимые закрепления и замеры", () => {
  render(<KatalogServerov {...props} podpiski={[props.podpiski[0], { ...props.podpiski[1], servery: [server("a")] }]} zaderzhki={[{ id: "a", realping_ms: 19 }]} />);
  const group = within(screen.getByRole("region", { name: "B.example" }));
  expect(group.queryByText("19 мс")).toBeNull();
  fireEvent.click(group.getByRole("button", { name: "Закрепить a" }));
  expect(within(screen.getByRole("region", { name: "A.example" })).getByRole("button", { name: "Закрепить a" })).toBeTruthy();
});

it("ошибка и пустая подписка доступны, сбой хранения объясняется", () => {
  localStorage.setItem("affory.server-view.v1", "invalid");
  render(<KatalogServerov {...props} podpiski={[...props.podpiski, { id: "empty", uzel: "empty.example", aktivnaya: false, otkaz: "панель недоступна", servery: [] }]} />);
  expect(screen.getByText(/Настройки списка|Не удалось прочитать/)).toBeTruthy();
  expect(screen.getByText(/Не обновлена: панель недоступна/)).toBeTruthy();
  expect(screen.getByText("Серверов пока нет. Обнови подписку.")).toBeTruthy();
});

it("поиск по большому ручному списку не теряет последний сервер", () => {
  const many = Array.from({ length: 150 }, (_, n) => server(`manual-${n}`, false));
  render(<KatalogServerov {...props} servery={many} podpiski={[]} zapros="manual-149" />);
  expect(screen.getAllByRole("button", { name: /^Подключиться к/ })).toHaveLength(1);
  expect(screen.getByRole("button", { name: "Подключиться к manual-149" })).toBeTruthy();
});
