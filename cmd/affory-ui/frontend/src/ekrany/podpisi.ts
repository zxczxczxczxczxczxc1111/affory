import type { Sostoyanie } from "../protokol";

// One map, not strings scattered through JSX: the "seven different labels"
// test checks this object, and it cannot see strings inlined in markup.
// Dry wording, no trailing dots, no "we": the panel is for people who know.
export const podpis: Record<Sostoyanie, string> = {
  "sluzhba-molchit": "служба не отвечает",
  vyklyuchen: "выключено",
  podnimaetsya: "подключается",
  podnyat: "подключено",
  "ne-neset": "подключено, трафик не идёт",
  vosstanavlivaetsya: "восстанавливается",
  otkaz: "отказ",
};

/** Label of the single main action per state, or null when there is none.
 *  Transient states get no button: a button during a transition is an
 *  invitation to double-click the tunnel into a coma. */
export const glavnoeDeystvie: Record<Sostoyanie, string | null> = {
  // Not null any more: a window that lost the service had no button at all,
  // so the only way back was to close it and start it again (guest run,
  // 03.09.2026). Asking again is not a transition, it is a question.
  "sluzhba-molchit": "Повторить",
  vyklyuchen: "Подключить",
  podnimaetsya: null,
  podnyat: "Отключить",
  "ne-neset": "Отключить",
  vosstanavlivaetsya: null,
  otkaz: "Повторить",
};
