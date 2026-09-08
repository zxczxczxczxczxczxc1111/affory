import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Udalenie } from "./Udalenie";

afterEach(cleanup);

// Uninstall asks about the keys EXPLICITLY. The secrets blob survives
// `sc delete` (measured on the stand 02.09.2026), which is right for a
// reinstall and wrong for someone leaving for good, so the choice cannot be
// implied. "Keep" is the default in the sense of being the safe answer and
// being labelled so; it is never sent on the human's behalf.
describe("удаление программы", () => {
  it("подтверждение неактивно, пока выбор про ключи не сделан", () => {
    render(<Udalenie naUdalenie={() => undefined} naOtmenu={() => undefined} />);
    expect(screen.getByTestId("podtverdit")).toBeDisabled();
  });

  it("«оставить» подписано как безопасное умолчание", () => {
    render(<Udalenie naUdalenie={() => undefined} naOtmenu={() => undefined} />);
    expect(screen.getByLabelText(/оставить/i)).toBeTruthy();
    expect(screen.getByTestId("umolchanie")).toHaveTextContent(/оставить/i);
  });

  it("выбор «оставить» отдаёт steret=false", () => {
    const na = vi.fn();
    render(<Udalenie naUdalenie={na} naOtmenu={() => undefined} />);
    fireEvent.click(screen.getByLabelText(/оставить/i));
    fireEvent.click(screen.getByTestId("podtverdit"));
    expect(na).toHaveBeenCalledWith(false);
  });

  it("выбор «стереть» отдаёт steret=true", () => {
    const na = vi.fn();
    render(<Udalenie naUdalenie={na} naOtmenu={() => undefined} />);
    fireEvent.click(screen.getByLabelText(/стереть/i));
    fireEvent.click(screen.getByTestId("podtverdit"));
    expect(na).toHaveBeenCalledWith(true);
  });

  it("отмена доступна всегда и ничего не удаляет", () => {
    const na = vi.fn();
    const ot = vi.fn();
    render(<Udalenie naUdalenie={na} naOtmenu={ot} />);
    fireEvent.click(screen.getByTestId("otmena"));
    expect(ot).toHaveBeenCalledOnce();
    expect(na).not.toHaveBeenCalled();
  });
});
