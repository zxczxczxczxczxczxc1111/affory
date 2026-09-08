import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Otkaz } from "./Otkaz";
import { tekstOtkaza, type Deystvie } from "./otkazy";

afterEach(cleanup);

// The list of codes comes from kody.go on disk, not from a copy here. A copy
// goes stale the day a code is added, and a stale copy is a green test about
// a screen that does not exist. The Go-side gate (internal/kachestvo) checks
// the actions against spec §9.1; this side checks that every code has text.
function kodyIzGo(): string[] {
  // process.cwd(), not import.meta.url: this file needs jsdom for render(),
  // and under jsdom import.meta.url is not a file: URL. vitest runs from the
  // frontend directory (vitest.config.ts lives there), so the path is fixed.
  const put = join(process.cwd(), "..", "..", "..", "internal", "protokol", "kody.go");
  const tekst = readFileSync(put, "utf8");
  const itog: string[] = [];
  for (const m of tekst.matchAll(/Kod[A-Za-z]+\s*=\s*"([a-z-]+)"/g)) itog.push(m[1]);
  return itog;
}

// not-implemented is the fallback contract's business (dash, not a refusal
// screen), and the Go gate excludes it by the same name.
const BEZ_EKRANA = new Set(["not-implemented"]);

const VSE_DEYSTVIYA: Deystvie[] = [
  "povtorit", "perepodklyuchitsya", "otkryt-servery", "obnovit-podpisku",
  "obnovit-programmu", "postavit-sluzhbu", "zaprosit-prava", "proverit-set",
  "pokazat-vinovnika", "nichego",
];

describe("экраны отказов", () => {
  const kody = kodyIzGo().filter((k) => !BEZ_EKRANA.has(k));

  it("коды прочитаны из kody.go, и их сорок", () => {
    // Pinning the count is deliberate: a new code must fail here until the
    // screen for it exists, and an empty read must not pass as "all covered".
    // Тридцать восьмой это internal-error, заведён 04.09.2026 полосой З:
    // паника службы отвечала кодом «разные версии», и человек шёл
    // переустанавливать исправную программу.
    // Тридцать девятый и сороковой заведены 05.09.2026 задачей 8:
    // bandwidth-unmeasured про число, которое пойдёт в объявление hysteria2, и
    // request-invalid про форму запроса. Оба отдельные потому, что чинятся
    // по-разному, а прежний общий код отправлял человека переустанавливать
    // исправную программу.
    expect(kody.length).toBe(40);
  });

  it.each(kody)("у кода %s есть свой текст и действие из словаря", (kod) => {
    const z = tekstOtkaza[kod];
    expect(z, `нет записи для ${kod}`).toBeTruthy();
    expect(z.tekst.trim().length).toBeGreaterThan(0);
    expect(VSE_DEYSTVIYA).toContain(z.deystvie);
  });

  it("тексты сухие: без «мы» и без точки в конце", () => {
    for (const kod of kody) {
      const t = tekstOtkaza[kod].tekst;
      expect(t, kod).not.toMatch(/\bмы\b/i);
      expect(t, kod).not.toMatch(/\.$/);
    }
  });

  it("pipe-squatted показывает имя виновника", () => {
    render(<Otkaz kod="pipe-squatted" vinovnik="chuzhoy.exe" naDeystvie={() => undefined} />);
    expect(screen.getByTestId("vinovnik")).toHaveTextContent("chuzhoy.exe");
    // And it is NOT the first-run screen: no install button on a squatted pipe.
    expect(screen.queryByText(/установить службу/i)).toBeNull();
  });

  it("subscription-expired показывает текст панели дословно, без нашего поверх", () => {
    render(<Otkaz kod="subscription-expired" tekst="Оплатите тариф до 05.09" naDeystvie={() => undefined} />);
    const el = screen.getByTestId("otkaz-tekst");
    expect(el).toHaveTextContent("Оплатите тариф до 05.09");
    expect(el.textContent).not.toContain(tekstOtkaza["subscription-expired"].tekst);
  });

  it("у nichego нет кнопки, у povtorit есть, и она отдаёт действие наверх", () => {
    const na = vi.fn();
    render(<Otkaz kod="firewall-disabled" naDeystvie={na} />);
    expect(screen.queryByTestId("otkaz-deystvie")).toBeNull();
    cleanup();
    render(<Otkaz kod="dns-resolve-failed" naDeystvie={na} />);
    fireEvent.click(screen.getByTestId("otkaz-deystvie"));
    expect(na).toHaveBeenCalledWith("povtorit");
  });
});
