import { expect, it } from "vitest";
import { aktualnyeZamery } from "./zamery";
import type { SpisokServerov } from "./ekrany/Servery";

const spisok: SpisokServerov = { servery: ["a", "b", "manual"].map(id => ({ id, imya: id, host: "example.org", port: 443, transport: "trojan", iz_podpiski: id !== "manual" })), vybran: "", podpiska_zadana: true, podpiska_uzel: "panel", versii: { a: "unchanged", b: "new", manual: "manual-v1" } };

it("сохраняет замеры неизменённых и ручных, убирает изменённые и удалённые", () => {
  const a = { id: "a", versiya: "unchanged", realping_ms: 123 };
  const manual = { id: "manual", versiya: "manual-v1", realping_ms: 45 };
  expect(aktualnyeZamery([a, { id: "b", versiya: "old", realping_ms: 89 }, manual, { id: "deleted", versiya: "old", realping_ms: 99 }], spisok)).toEqual([a, manual]);
});

it("запоздавший результат старого замера не возвращает задержку после смены ключей", () => {
  expect(aktualnyeZamery([{ id: "b", versiya: "old", realping_ms: 89 }], spisok)).toEqual([]);
});

it("повторный список и обновление другой подписки не очищают замеры", () => {
  const measured = [{ id: "a", versiya: "unchanged", realping_ms: 123 }];
  expect(aktualnyeZamery(measured, { ...spisok, podpiska_obnovlena: new Date().toISOString() })).toEqual(measured);
});
