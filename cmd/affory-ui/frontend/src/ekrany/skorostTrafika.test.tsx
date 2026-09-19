import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { Statistika } from "../protokol";
import { formatSkorosti, useSkorostTrafika } from "./skorostTrafika";

beforeEach(() => vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "performance"] }));
afterEach(() => { cleanup(); vi.useRealTimers(); });
const stat = (prinyato?: number, otdano?: number): Statistika => ({ adres_vyhoda: "", prinyato, otdano });
const props = (data: Statistika | null, up = true, session = "a") => ({ data, up, session });
const hook = ({ data, up, session }: ReturnType<typeof props>) => useSkorostTrafika(data, up, session);
const tick = (ms = 1000) => act(() => { vi.advanceTimersByTime(ms); });

it("вычисляет Мбит/с по времени между снимками, включая измеренный ноль", () => {
  const { result, rerender } = renderHook(hook, { initialProps: props(stat(100, 50)) });
  expect(result.current.priem).toBeUndefined();
  tick(2000);
  rerender(props(stat(3100100, 200050)));
  expect(formatSkorosti(result.current.priem)).toBe("12,4 Мбит/с");
  expect(formatSkorosti(result.current.otdacha)).toBe("0,8 Мбит/с");
  tick(); rerender(props(stat(3100100, 200050)));
  expect(formatSkorosti(result.current.priem)).toBe("0,0 Мбит/с");
});

it("не выдаёт скорость из отсутствующих, сброшенных и просроченных счётчиков", () => {
  const { result, rerender } = renderHook(hook, { initialProps: props(stat(100, 50)) });
  tick(); rerender(props(stat(10)));
  expect(result.current).toEqual({ priem: undefined, otdacha: undefined });
  tick(); rerender(props(stat(125010, 500)));
  expect(result.current.priem).toBe(1);
  expect(result.current.otdacha).toBeUndefined();
  tick(5000);
  expect(result.current).toEqual({});
  rerender(props(stat(10000000, 500000)));
  expect(result.current).toEqual({});
  rerender(props(null));
  expect(result.current).toEqual({});
});

it("не использует старый снимок после отключения или смены сессии", () => {
  const old = stat(100, 50);
  const { result, rerender } = renderHook(hook, { initialProps: props(old) });
  tick(); rerender(props(old, false));
  rerender(props(old, true, "b"));
  tick(); rerender(props(stat(1000100, 1000050), true, "b"));
  expect(result.current).toEqual({});
  tick(); rerender(props(stat(2000100, 2000050), true, "b"));
  expect(result.current.priem).toBe(8);
  const latest = stat(3000100, 3000050);
  tick(); rerender(props(latest, true, "b"));
  rerender(props(latest, true, "c"));
  expect(result.current).toEqual({});
});
