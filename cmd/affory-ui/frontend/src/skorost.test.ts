import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useSkorost } from "./skorost";
import type { Kadr } from "./most";

// Опрос состояния замера шёл раз в секунду ВСЕГДА, пока живо окно. Журнал
// команд живой машины за двое суток: 25 155 строк, из них 18 338 это
// speedTestStatus, при двадцати запусках замера за то же время.
//
// Диск от этого не страдает, ротация работает. Страдает диагностика: 73%
// журнала команд это пустой опрос, и настоящие команды уезжают за горизонт
// ротации вчетверо быстрее.

const otvet = (phase: string): Kadr => ({
  id: 1,
  imya: "speedTestStatus",
  telo: { phase, path: "vpn", provider: "", name: "", attempt: 0 },
} as unknown as Kadr);

describe("опрос замера скорости", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("в покое опрашивает редко, а не каждую секунду", async () => {
    const zvat = vi.fn(async () => otvet("idle"));
    renderHook(() => useSkorost(zvat, true));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    const posleStarta = zvat.mock.calls.length;

    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    const zaDesyatSekund = zvat.mock.calls.length - posleStarta;
    // Раз в секунду дало бы десять. Три с запасом это уже разреженный опрос.
    expect(zaDesyatSekund).toBeLessThanOrEqual(3);
  });

  it("пока замер идёт, опрашивает каждую секунду", async () => {
    const zvat = vi.fn(async () => otvet("download"));
    renderHook(() => useSkorost(zvat, true));
    await act(async () => { await vi.advanceTimersByTimeAsync(0); });
    const posleStarta = zvat.mock.calls.length;

    await act(async () => { await vi.advanceTimersByTimeAsync(5_000); });
    const zaPyatSekund = zvat.mock.calls.length - posleStarta;
    // Замер живёт минуты, и его состояние должно быть свежим: не реже четырёх
    // раз за пять секунд.
    expect(zaPyatSekund).toBeGreaterThanOrEqual(4);
  });

  // Замер, закончившийся сам, обязан вернуть опрос в редкий режим: иначе
  // экономия работает только до первого запуска.
  it("после конца замера опрос снова редкий", async () => {
    let phase = "download";
    const zvat = vi.fn(async () => otvet(phase));
    renderHook(() => useSkorost(zvat, true));
    await act(async () => { await vi.advanceTimersByTimeAsync(3_000); });

    phase = "complete";
    await act(async () => { await vi.advanceTimersByTimeAsync(2_000); });
    const posleKonca = zvat.mock.calls.length;

    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    const zaDesyatSekund = zvat.mock.calls.length - posleKonca;
    expect(zaDesyatSekund).toBeLessThanOrEqual(3);
  });

  it("без связи не опрашивает вовсе", async () => {
    const zvat = vi.fn(async () => otvet("idle"));
    renderHook(() => useSkorost(zvat, false));
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(zvat).not.toHaveBeenCalled();
  });
});
