// The four tabs of spec §8.3, in order. Exactly four: every attempt to add a
// fifth is the reason §8.3 pinned the seven items of §5 to these four.
export const VKLADKI = ["podklyuchenie", "pravila", "servery", "nastroyki"] as const;

export type Vkladka = (typeof VKLADKI)[number];

const NAZVANIYA: Record<Vkladka, string> = {
  podklyuchenie: "Подключение",
  pravila: "Правила",
  servery: "Серверы",
  nastroyki: "Настройки",
};

export function nazvanieVkladki(v: Vkladka): string {
  return NAZVANIYA[v];
}
