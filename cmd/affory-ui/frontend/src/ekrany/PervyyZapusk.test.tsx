import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PervyyZapusk } from "./PervyyZapusk";

afterEach(cleanup);

// Spec §9.2: before the service is installed exactly ONE control is active.
// The test counts enabled controls rather than looking for the button: the
// button is present even when three other controls are active next to it.
function aktivnye(): HTMLElement[] {
  // querySelectorAll, not getAllByRole: the latter throws on zero matches,
  // and "zero active controls" is a state this test must be able to assert.
  const vse = document.querySelectorAll<HTMLElement>(
    "button, input, select, textarea, a[href], [role=tab], [role=switch], [role=checkbox]",
  );
  return [...vse].filter(
    (el) => !(el as HTMLButtonElement).disabled && el.getAttribute("aria-disabled") !== "true",
  );
}

describe("первый запуск", () => {
  it("без службы активен ровно один элемент, и это «установить службу»", () => {
    render(<PervyyZapusk sostoyanie="net-sluzhby" naUstanovku={() => undefined} />);
    const a = aktivnye();
    expect(a).toHaveLength(1);
    expect(a[0]).toHaveTextContent(/установить службу/i);
  });

  it("кнопка отдаёт установку наверх, а не зовёт оболочку сама", () => {
    const na = vi.fn();
    render(<PervyyZapusk sostoyanie="net-sluzhby" naUstanovku={na} />);
    fireEvent.click(screen.getByTestId("ustanovit"));
    expect(na).toHaveBeenCalledOnce();
  });

  it("пока установка идёт, активных элементов ноль", () => {
    // UAC is up or the service is starting: a second click would queue a
    // second UAC prompt behind the first one.
    render(<PervyyZapusk sostoyanie="ustanavlivaetsya" naUstanovku={() => undefined} />);
    expect(aktivnye()).toHaveLength(0);
    expect(screen.getByTestId("hod")).toBeTruthy();
  });

  it("отказ установки показывает причину и снова даёт одну кнопку", () => {
    render(<PervyyZapusk sostoyanie="otkaz" prichina="UAC отклонён" naUstanovku={() => undefined} />);
    expect(screen.getByTestId("prichina")).toHaveTextContent("UAC отклонён");
    expect(aktivnye()).toHaveLength(1);
  });
});
