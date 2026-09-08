import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Skorost } from "./Skorost";
import { readSpeed, type SpeedSnapshot } from "../skorost";

const base: SpeedSnapshot = { id: 1, phase: "upload", path: "vpn", provider: "librespeed", name: "LibreSpeed", attempt: 1 };
afterEach(cleanup);
describe("managed speed test", () => {
  it("starts the selected provider without inventing a result", () => {
    const start = vi.fn();
    render(<Skorost snapshot={null} error="" pending={false} disabled={false} start={start} cancel={vi.fn()} />);
    fireEvent.click(screen.getByText("Сервис замера"));
    fireEvent.click(screen.getByLabelText("Первый сервис замера"));
    fireEvent.click(screen.getByRole("option", { name: "Cloudflare" }));
    fireEvent.click(screen.getByRole("button", { name: "Проверить" }));
    expect(start).toHaveBeenCalledWith("cloudflare");
    expect(screen.queryByText("Мбит/с")).toBeNull();
  });
  it("keeps cancellation available when the VPN is busy", () => {
    const cancel = vi.fn();
    render(<Skorost snapshot={base} error="" pending={false} disabled start={vi.fn()} cancel={cancel} />);
    fireEvent.click(screen.getByRole("button", { name: "Остановить" }));
    expect(cancel).toHaveBeenCalledOnce();
    expect(screen.getByLabelText("Первый сервис замера")).toBeDisabled();
  });
  it("rejects partial and non-finite completion frames", () => {
    // A single number is still one direction, even if the bridge looks optimistic.
    expect(readSpeed({ ...base, phase: "complete", result: { download_mbps: 71 } })?.phase).toBe("error");
    expect(readSpeed({ ...base, phase: "complete", result: { download_mbps: 71, upload_mbps: Infinity } })?.phase).toBe("error");
  });
  it("does not render stale numbers after cancellation", () => {
    render(<Skorost snapshot={{ ...base, phase: "cancelled", reason: "Сервер изменился", result: { name: "LibreSpeed", download_mbps: 999, upload_mbps: 999, attempts: [] } }} error="" pending={false} disabled={false} start={vi.fn()} cancel={vi.fn()} />);
    expect(screen.getByText("Сервер изменился")).toBeInTheDocument();
    expect(screen.queryByText(/999/)).toBeNull();
  });
});
