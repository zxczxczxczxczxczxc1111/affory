import { describe, expect, it } from "vitest";
import { CSP, metaCsp } from "./csp";

// О7 аудита 1.6.1. Политики безопасности у страницы не было вовсе, и чужой
// скрипт, попавший в неё (через имя сервера, текст панели), мог бы звать мост
// окна как свой.
describe("политика безопасности страницы", () => {
  const pravila = new Map(CSP.split(";").map((p) => {
    const [imya, ...znacheniya] = p.trim().split(/\s+/);
    return [imya, znacheniya] as const;
  }));

  it("скрипты только свои, без eval и встроенных", () => {
    expect(pravila.get("script-src")).toEqual(["'self'"]);
    expect(CSP).not.toContain("unsafe-eval");
    expect(pravila.get("default-src")).toEqual(["'none'"]);
  });

  it("мост Wails ходит только на свой адрес", () => {
    // @wailsio/runtime beta.16: fetch на location.origin + /wails/runtime.
    expect(pravila.get("connect-src")).toEqual(["'self'"]);
  });

  it("картинки data: разрешены: из них QR и значки сервисов", () => {
    expect(pravila.get("img-src")).toEqual(["'self'", "data:"]);
  });

  it("встаёт первым тегом head", () => {
    const m = metaCsp();
    expect(m.tag).toBe("meta");
    expect(m.attrs["http-equiv"]).toBe("Content-Security-Policy");
    expect(m.attrs.content).toBe(CSP);
    expect(m.injectTo).toBe("head-prepend");
  });
});
