// Russian eleven does not care what the last digit promised the designer.
export function slovoPosleChisla(n: number, one: string, few: string, many: string): string {
  const count = Math.abs(n);
  if (!Number.isInteger(count)) return few;
  if (count % 100 >= 11 && count % 100 <= 14) return many;
  if (count % 10 === 1) return one;
  if (count % 10 >= 2 && count % 10 <= 4) return few;
  return many;
}
