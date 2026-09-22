import type { SpisokServerov, ZamerZaderzhki } from "./ekrany/Servery";

export function aktualnyeZamery(zamery: ZamerZaderzhki[], spisok: SpisokServerov | null): ZamerZaderzhki[] {
  if (!spisok) return [];
  const ids = new Set(spisok.servery.map(s => s.id));
  return zamery.filter(z => ids.has(z.id) && (spisok.versii
    ? z.versiya !== undefined && spisok.versii[z.id] === z.versiya
    : z.versiya === undefined));
}
