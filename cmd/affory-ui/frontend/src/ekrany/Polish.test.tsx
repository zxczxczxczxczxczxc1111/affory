import { useState } from "react";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Vybor } from "./Vybor";
import { Glavnyy } from "./Glavnyy";
import { Marshruty } from "./Marshruty";
import type { PravilaTrafika } from "../trafik";

afterEach(cleanup);
const options = [{ value: "vpn", label: "Через VPN" }, { value: "direct", label: "Напрямую" }, { value: "auto", label: "Автоматически" }];
function Picker({ change, disabled = false }: { change: (value: string) => void; disabled?: boolean }) {
  const [value, setValue] = useState("vpn");
  return <Vybor label="Маршрут" options={options} value={value} disabled={disabled} onChange={next => { setValue(next); change(next); }} />;
}
it("keyboard exploration does not save until Enter; Escape preserves the selection", () => {
  // A wandering arrow key is not consent to reconfigure the network.
  const change = vi.fn();
  render(<Picker change={change} />);
  const trigger = screen.getByRole("combobox");
  trigger.focus();
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  expect(change).not.toHaveBeenCalled();
  fireEvent.keyDown(trigger, { key: "Escape" });
  expect(trigger).toHaveTextContent("Через VPN");
  expect(screen.queryByRole("listbox")).toBeNull();
  fireEvent.keyDown(trigger, { key: "Enter" });
  fireEvent.keyDown(trigger, { key: "ArrowDown" });
  fireEvent.keyDown(trigger, { key: "Enter" });
  expect(change).toHaveBeenCalledExactlyOnceWith("direct");
  expect(trigger).toHaveTextContent("Напрямую");
  expect(trigger).toHaveFocus();
});
it("Home, End and Russian typeahead find options; Tab closes without committing", () => {
  const change = vi.fn();
  render(<Picker change={change} />);
  const trigger = screen.getByRole("combobox");
  fireEvent.keyDown(trigger, { key: "End" });
  const last = screen.getByRole("option", { name: "Автоматически" });
  expect(trigger.getAttribute("aria-activedescendant")).toBe(last.id);
  fireEvent.keyDown(trigger, { key: "Home" });
  expect(trigger.getAttribute("aria-activedescendant")).toBe(screen.getByRole("option", { name: "Через VPN" }).id);
  fireEvent.keyDown(trigger, { key: "н" });
  expect(trigger.getAttribute("aria-activedescendant")).toBe(screen.getByRole("option", { name: "Напрямую" }).id);
  fireEvent.keyDown(trigger, { key: "Tab" });
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(change).not.toHaveBeenCalled();
});
it("pointer selection works in the portal and an outside press cancels", () => {
  const change = vi.fn();
  render(<Picker change={change} />);
  const trigger = screen.getByRole("combobox");
  fireEvent.click(trigger);
  fireEvent.click(screen.getByRole("option", { name: "Напрямую" }));
  expect(change).toHaveBeenCalledExactlyOnceWith("direct");
  fireEvent.click(trigger);
  fireEvent.pointerDown(document.body);
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(change).toHaveBeenCalledTimes(1);
});
it("a pending operation closes the menu and disables further selections", () => {
  const change = vi.fn();
  const { rerender } = render(<Picker change={change} />);
  fireEvent.click(screen.getByRole("combobox"));
  rerender(<Picker change={change} disabled />);
  expect(screen.queryByRole("listbox")).toBeNull();
  expect(screen.getByRole("combobox")).toBeDisabled();
  expect(change).not.toHaveBeenCalled();
});
it("loading servers is distinct from an empty configured list", () => {
  const { rerender } = render(<Glavnyy status={{ sostoyanie: "vyklyuchen" }} />);
  expect(screen.getByText("Читаю список серверов")).toBeInTheDocument();
  expect(screen.queryByText("Серверов пока нет")).toBeNull();
  rerender(<Glavnyy status={{ sostoyanie: "vyklyuchen" }} spisok={{ servery: [], vybran: "", podpiska_zadana: false, podpiska_uzel: "" }} />);
  expect(screen.getByText("Серверов пока нет")).toBeInTheDocument();
  expect(screen.queryByText("Сведения о сервере")).toBeNull();
});
it("Russian site policy is visible above site rules and preserves explicit routes", async () => {
  const send = vi.fn();
  const trafik: PravilaTrafika = { po_umolchaniyu: "vpn", domeny: [{ domen: "example.org", marshrut: "vpn" }], prilozheniya: [], servisy: [] };
  render(<Marshruty otlozheno={{}} trafik={trafik} status={{ sostoyanie: "vyklyuchen" }} pravila={{ protsessy: [], domeny: [], trafik, bez_ru_spiska: false }} naKomandu={send} />);
  fireEvent.click(screen.getByRole("tab", { name: /Сайты/ }));
  const toggle = screen.getByRole("switch", { name: "Российские сайты напрямую" });
  expect(toggle.closest("details")).toBeNull();
  expect(toggle.compareDocumentPosition(screen.getByText("Правила сайтов")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  fireEvent.click(toggle);
  expect(send).not.toHaveBeenCalled();
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Применить изменения" })); });
  expect(send).toHaveBeenCalledWith("setRules", { trafik, bez_ru_spiska: true });
});
it("application rules show names without letter avatars and still route the process family", async () => {
  const send = vi.fn();
  const trafik: PravilaTrafika = { po_umolchaniyu: "vpn", prilozheniya: [{ put: "C:\\Games\\Steam\\steam.exe", imya: "steam.exe", potomki: true, marshrut: "direct" }], domeny: [], servisy: [] };
  const { container } = render(<Marshruty otlozheno={{}} trafik={trafik} status={{ sostoyanie: "vyklyuchen" }} pravila={{ protsessy: [], domeny: [], trafik }} naKomandu={send} />);
  fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
  expect(screen.getByText("steam.exe")).toBeInTheDocument();
    // Значков в правилах приложений нет по решению владельца: строка это имя
  // файла и путь под ним, а путь целиком лежит в подсказке.
  expect(container.querySelector("img, [style*='mask-image']")).toBeNull();
  expect(screen.getByTitle(String.raw`C:\Games\Steam\steam.exe`)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("combobox", { name: "Маршрут steam.exe" }));
  fireEvent.click(screen.getByRole("option", { name: "Через VPN" }));
  expect(send).not.toHaveBeenCalled();
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Применить изменения" })); });
  expect(send).toHaveBeenCalledWith("setRules", expect.objectContaining({ trafik: expect.objectContaining({ prilozheniya: [{ ...trafik.prilozheniya[0], marshrut: "vpn" }] }) }));
});

it("server checks remain available without VPN and never confuse node latency with VPN latency", () => {
  // A fast TCP handshake does not get to impersonate the entire tunnel.
  const check = vi.fn();
  render(<Glavnyy status={{ sostoyanie: "vyklyuchen" }} naProverit={check}
    servery={[{ id: "s", imya: "Test server", host: "example.org", port: 443, transport: "hy2", iz_podpiski: false }]}
    zaderzhki={[{ id: "s", tcping_ms: 4, realping_ms: 148 }]} />);
  fireEvent.click(screen.getByRole("button", { name: "Проверить серверы" }));
  expect(check).toHaveBeenCalledOnce();
  // В компактной строке только VPN; проверка узла остаётся в деталях.
  const zaderzhka = screen.getByText("148 мс");
  expect(zaderzhka).toHaveAttribute("title", "VPN 148 мс · узел 4 мс");
  expect(screen.queryByText("4 мс")).toBeNull();
});
