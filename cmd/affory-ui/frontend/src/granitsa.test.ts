// @vitest-environment node
//
// A filesystem test has no business in jsdom: there import.meta.url is not a
// file: URL and fileURLToPath refuses it. Node is the honest environment here.
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// Walk the directory by hand instead of import.meta.glob: glob only sees what
// Vite managed to bundle, and this test must see everything on disk.
function vseFayly(kat: string): string[] {
  const itog: string[] = [];
  // A missing directory is "nothing to check", which the first test below
  // reports as a red failure. A throw here would be an error, not a verdict.
  if (!existsSync(kat)) return itog;
  for (const imya of readdirSync(kat)) {
    const put = join(kat, imya);
    if (statSync(put).isDirectory()) itog.push(...vseFayly(put));
    else if (/\.tsx?$/.test(imya)) itog.push(put);
  }
  return itog;
}

describe("граница оболочки", () => {
  // fileURLToPath, not .pathname: on Windows .pathname yields "/D:/..." with a
  // leading slash that readdirSync rejects. Found the hard way, kept on purpose.
  const koren = fileURLToPath(new URL(".", import.meta.url));
  const ekrany = vseFayly(join(koren, "ekrany"));

  it("каталог экранов не пуст, иначе проверять нечего", () => {
    expect(ekrany.length).toBeGreaterThan(0);
  });

  it.each(ekrany)("%s не импортирует оболочку напрямую", (put) => {
    const tekst = readFileSync(put, "utf8");
    // Only most.ts is allowed to know Wails sits underneath. Swapping the shell
    // is then one module, not every screen.
    expect(tekst).not.toMatch(/from\s+["']@wailsio\/runtime["']/);
    expect(tekst).not.toMatch(/from\s+["'][./]*bindings\//);
  });
});
