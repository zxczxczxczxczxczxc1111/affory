// WCAG contrast for the two colour notations spec §8.2 actually uses:
// "#rrggbb" and "rgb(r g b / a)". Nothing else, on purpose. A colour parser
// that accepts everything is where a typo in a token goes to be forgiven.

type RGB = { r: number; g: number; b: number };

const HEX = /^#([0-9a-f]{6})$/i;
const RGB_A = /^rgb\(\s*(\d+)\s+(\d+)\s+(\d+)\s*(?:\/\s*([0-9.]+)\s*)?\)$/i;

/** Parses one token value into opaque RGB. Alpha is composited over `fon`,
 *  because a 60% white on black is a grey, and the grey is what the eye
 *  measures. Throws on anything it does not recognise: a silent fallback to
 *  black would make a bad token pass the black-background test. */
export function razobrat(cvet: string, fon: RGB = { r: 0, g: 0, b: 0 }): RGB {
  const c = cvet.trim();
  const h = HEX.exec(c);
  if (h) {
    const n = parseInt(h[1], 16);
    return { r: (n >> 16) & 255, g: (n >> 8) & 255, b: n & 255 };
  }
  const m = RGB_A.exec(c);
  if (m) {
    const a = m[4] === undefined ? 1 : Number(m[4]);
    const smes = (k: number, f: number) => Math.round(k * a + f * (1 - a));
    return {
      r: smes(Number(m[1]), fon.r),
      g: smes(Number(m[2]), fon.g),
      b: smes(Number(m[3]), fon.b),
    };
  }
  throw new Error(`цвет не разобран: ${JSON.stringify(cvet)}`);
}

function kanal(v: number): number {
  const s = v / 255;
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

/** Relative luminance per WCAG 2.x. */
export function yarkost(c: RGB): number {
  return 0.2126 * kanal(c.r) + 0.7152 * kanal(c.g) + 0.0722 * kanal(c.b);
}

/** Contrast ratio between a foreground token and a background token, both
 *  as written in tokeny.css. The background is resolved first and the
 *  foreground's alpha is composited over it, which is what actually happens
 *  on screen. Always >= 1. */
export function kontrast(perednii: string, fon: string): number {
  const f = razobrat(fon);
  const p = razobrat(perednii, f);
  const l1 = yarkost(p);
  const l2 = yarkost(f);
  const [svetlee, temnee] = l1 >= l2 ? [l1, l2] : [l2, l1];
  return (svetlee + 0.05) / (temnee + 0.05);
}
