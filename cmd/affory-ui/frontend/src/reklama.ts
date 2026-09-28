import { slovoPosleChisla } from "./chisla";
import type { ReklamaSostoyanie, Sostoyanie } from "./protokol";

// Блокировка рекламы (28.09.2026): настройка в правилах и строки состояния
// вкладки «Реклама». Строки считаются здесь, чистой функцией, а не в экране:
// ветвей восемь, и каждую надо проверить без отрисовки.

/** Уровни списка HaGeZi. Служба знает ровно эти два (internal/reklama). */
export type UrovenReklamy = "light" | "multi";

/** Настройка, как её шлёт listRules: объект всегда целиком, исключения всегда
 *  список (teloReklamy в cmd/affory-svc/komandy_pravil.go). */
export interface ReklamaPravila {
  vkl: boolean;
  uroven: UrovenReklamy;
  razresheno: string[];
}

export const UROVNI: { z: UrovenReklamy; podpis: string; opisanie: string }[] = [
  { z: "light", podpis: "Базовый", opisanie: "Самые частые рекламные и следящие адреса. Сайты почти не ломаются" },
  {
    z: "multi",
    podpis: "Расширенный",
    opisanie: "Больше трекеров, а ещё фишинг и вредоносные сайты. Изредка ломает вход или оплату на сайтах",
  },
];

/** Столько исключений принимает служба (predelIsklyucheniyReklamy). Набор
 *  сверх предела отвергается целиком, поэтому счёт ведётся и здесь. */
export const PREDEL_ISKLYUCHENIY = 256;

export interface StrokaSostoyaniya {
  tekst: string;
  /** Предупреждение: рисуется цветом warn, а не приглушённым. */
  vazhnoe: boolean;
}

const CHISLO = new Intl.NumberFormat("ru-RU");

function imyaUrovnya(u: string): string {
  return UROVNI.find((x) => x.z === u)?.podpis ?? u;
}

/** Хвост строки о списке: дата сборки и число правил. Дата без времени:
 *  список собирается раз в сутки, и часы тут ничего не говорят. */
function opisatSpisok(st: ReklamaSostoyanie): string {
  const pravil = `${CHISLO.format(st.pravil)} ${slovoPosleChisla(st.pravil, "правило", "правила", "правил")}`;
  const d = st.sobran ? new Date(st.sobran) : null;
  return d && !Number.isNaN(d.getTime()) ? ` от ${d.toLocaleDateString("ru-RU")} · ${pravil}` : `: ${pravil}`;
}

/** Строки под переключателем. Считаются по СОХРАНЁННОЙ настройке: черновик
 *  ещё ничего не поменял ни в файле, ни в ядре. */
export function strokiSostoyaniya(
  sohranyonnoe: ReklamaPravila,
  st: ReklamaSostoyanie | undefined,
  sostoyanie: Sostoyanie,
): StrokaSostoyaniya[] {
  // Файл и мета остаются на диске и после выключения. Без этой отсечки окно
  // показало бы у выключенной блокировки предупреждение о прошлом отказе.
  if (!sohranyonnoe.vkl) return [{ tekst: "Реклама и трекеры не блокируются. Список не скачивается", vazhnoe: false }];
  const podnyat = sostoyanie === "podnyat";
  const itog: StrokaSostoyaniya[] = [];
  if (!podnyat) itog.push({ tekst: "Блокировка работает, пока VPN подключён", vazhnoe: false });
  // Состояние без уровня служба заводит одним отказом, когда файла ещё нет.
  if (!st || !st.uroven) itog.push({ tekst: "Список готовится", vazhnoe: false });
  else if (st.uroven !== sohranyonnoe.uroven)
    itog.push({
      tekst: `Готовится список «${imyaUrovnya(sohranyonnoe.uroven)}». Пока действует «${imyaUrovnya(st.uroven)}»`,
      vazhnoe: false,
    });
  else if (st.vstroennyy)
    itog.push({ tekst: `Встроенный список${opisatSpisok(st)}. Свежий скачается при первой возможности`, vazhnoe: false });
  else itog.push({ tekst: `Список «${imyaUrovnya(st.uroven)}»${opisatSpisok(st)}. Обновляется раз в сутки`, vazhnoe: false });

  if (podnyat && st && st.deystvuet === false) {
    itog.push({
      tekst: st.otkaz
        ? `На этом подключении блокировка не действует: ${st.otkaz}. Переподключись, чтобы попробовать снова`
        : "На этом подключении блокировка не действует. Переподключись, чтобы попробовать снова",
      vazhnoe: true,
    });
  } else if (st?.otkaz) {
    itog.push({ tekst: `Последнее обновление не удалось: ${st.otkaz}`, vazhnoe: true });
  }
  return itog;
}

/** Порядок ключей как у окна, а не как у службы: служба сортирует ключи map,
 *  и тот же объект иначе давал бы ложное «есть неприменённые изменения». */
export function normReklama(r: ReklamaPravila): ReklamaPravila {
  return { vkl: r.vkl, uroven: r.uroven, razresheno: [...(r.razresheno ?? [])] };
}
