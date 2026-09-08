import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Karkas } from "./Karkas";
import { VKLADKI, nazvanieVkladki } from "./vkladki";

afterEach(cleanup);

const nichego = () => undefined;

describe("каркас окна", () => {
  it("вкладок ровно четыре и ровно те, что в §8.3", () => {
    expect([...VKLADKI]).toEqual(["podklyuchenie", "pravila", "servery", "nastroyki"]);
    expect(VKLADKI.map(nazvanieVkladki)).toEqual(["Подключение", "Правила", "Серверы", "Настройки"]);
  });

  it("активная вкладка отмечена чернилами, не заливкой", () => {
    // §8.2: fill paints things that carry a label; state is shown in ink.
    // A tab marked by fill alone has no visible edge on black.
    render(
      <Karkas vkladka="servery" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>,
    );
    const akt = screen.getByRole("tab", { selected: true });
    expect(akt).toHaveTextContent("Подключение");
    expect(akt.className).toMatch(/accent-ink/);
    expect(akt.className).not.toMatch(/bg-accent(?!-)/);
  });

  it("щелчок по вкладке отдаёт её имя наверх", () => {
    const na = vi.fn();
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={na} naSvernut={nichego} naZakryt={nichego}>x</Karkas>,
    );
    fireEvent.click(screen.getByRole("tab", { name: "Правила" }));
    expect(na).toHaveBeenCalledWith("pravila");
  });

  it("полоса перетаскивается, кнопки окна нет", () => {
    // The whole reason this task exists: without a drag region a frameless
    // window cannot be moved, and without no-drag the close button drags.
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego}>x</Karkas>,
    );
    expect(screen.getByTestId("polosa").style.getPropertyValue("--wails-draggable")).toBe("drag");
    expect(screen.getByTestId("zakryt").style.getPropertyValue("--wails-draggable")).toBe("no-drag");
  });

  it("свернуть и закрыть зовут наверх, а не оболочку напрямую", () => {
    const sv = vi.fn();
    const za = vi.fn();
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={sv} naZakryt={za}>x</Karkas>,
    );
    fireEvent.click(screen.getByTestId("svernut"));
    fireEvent.click(screen.getByTestId("zakryt"));
    expect(sv).toHaveBeenCalledOnce();
    expect(za).toHaveBeenCalledOnce();
  });
});

// First run (§9.2): until the service exists nothing else is active, tabs
// included. The window controls stay live: the human can still close it.
describe("каркас: вкладки на первом запуске", () => {
  it("с zablokirovany вкладки неактивны, свернуть и закрыть работают", () => {
    const na = vi.fn();
    render(
      <Karkas vkladka="podklyuchenie" naVkladku={na} naSvernut={nichego} naZakryt={nichego} zablokirovany>x</Karkas>,
    );
    const tab = screen.getByRole("tab", { name: "Подключение" }) as HTMLButtonElement;
    expect(tab.disabled).toBe(true);
    fireEvent.click(tab);
    expect(na).not.toHaveBeenCalled();
    expect((screen.getByTestId("zakryt") as HTMLButtonElement).disabled).toBe(false);
  });
});

it("GitHub opens through the shell without making the title bar draggable", () => {
  // A repository link should open a browser, not tow the whole window away.
  const open = vi.fn();
  render(<Karkas vkladka="podklyuchenie" naVkladku={nichego} naSvernut={nichego} naZakryt={nichego} naGitHub={open}>x</Karkas>);
  const button = screen.getByRole("button", {name: "Открыть GitHub Affory"});
  fireEvent.click(button);
  expect(open).toHaveBeenCalledOnce();
  expect(button.style.getPropertyValue("--wails-draggable")).toBe("no-drag");
});
