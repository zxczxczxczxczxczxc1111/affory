// @vitest-environment node
//
// Reads a file from disk, so node, not jsdom: under jsdom import.meta.url is
// not a file: URL and the read fails for a reason unrelated to colours.
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { kontrast } from "./kontrast";

// Values come from tokeny.css itself. Copying them here means fixing a colour
// in one place one day and getting a green test about the old one.
function tokeny(): Record<string, string> {
  const put = fileURLToPath(new URL("./tokeny.css", import.meta.url));
  const tekst = readFileSync(put, "utf8");
  const itog: Record<string, string> = {};
  for (const m of tekst.matchAll(/(--[a-z0-9-]+):\s*([^;]+);/g)) {
    itog[m[1]] = m[2].trim();
  }
  return itog;
}

describe("токены тёмной темы", () => {
  const tk = tokeny();

  it("файл вообще разобран", () => {
    // An empty map would turn every check below green without checking.
    expect(Object.keys(tk).length).toBeGreaterThan(20);
  });

  it("акцентная ЛИНИЯ держит контраст на фоне", () => {
    // 9.6:1 promised by spec §8.2. 4.5 is the AA floor: loose enough not to
    // fail on a one-step hue change, tight enough to catch a dark swap.
    expect(kontrast(tk["--color-accent-ink"], tk["--color-background"]))
      .toBeGreaterThanOrEqual(4.5);
  });

  it("вторичный текст держит контраст на поверхности", () => {
    expect(kontrast(tk["--color-fg-secondary"], tk["--color-surface"]))
      .toBeGreaterThanOrEqual(4.5);
  });

  it("акцентная ЗАЛИВКА текстовый контраст НЕ держит, и это правильно", () => {
    // Fill never sits under text. If this ever flips green the other way,
    // somebody swapped the fill for a light one and broke the plate.
    expect(kontrast(tk["--color-accent"], tk["--color-background"]))
      .toBeLessThan(4.5);
  });

  it("предупреждение и опасность различимы не только оттенком", () => {
    // Hue is the first channel to go under colour vision deficiency. The
    // pair must differ in lightness too.
    const w = kontrast(tk["--color-warn"], tk["--color-background"]);
    const d = kontrast(tk["--color-danger"], tk["--color-background"]);
    expect(Math.abs(w - d)).toBeGreaterThan(0.5);
  });
});
