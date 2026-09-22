import { useEffect, useState, type ReactNode } from "react";
import type {
  OtkazNaEkrane,
  Rezhim,
  Server,
  Statistika,
  StatusOtvet,
} from "../protokol";
import { glavnoeDeystvie, podpis } from "./podpisi";
import { type PodpiskaNaEkrane, type SpisokServerov, type ZamerZaderzhki } from "./Servery";
import type { PravilaOtvet } from "./Pravila";
import type { Marshrut } from "../trafik";
import sphere from "../assets/affory-sphere.png";
import { Knopka, Poisk, PROCHERK, Segment } from "./ui";
import { KnopkaSpravki, SpravkaProtokolov } from "./SpravkaProtokolov";
import { KatalogServerov, gruppyServerov } from "./KatalogServerov";
import { slovoPosleChisla } from "../chisla";
import { formatSkorosti, useSkorostTrafika } from "./skorostTrafika";
import "./Glavnyy.css";

// Pure over props. No subscription, no bridge, no runtime import: App.tsx
// owns the wiring and hands the whole StatusOtvet down in one piece. Half
// the state arriving in two events is exactly how two halves start to
// disagree, so there is one prop, not five.
//
// Перерисован 16.09.2026 по макетам владельца: две колонки, слева состояние
// туннеля, справа набор серверов, внизу служебная строка на всю ширину.
// Отдельной кнопки «Отключить» под сферой нет: сфера и есть выключатель.
// Свечение меняется только внутри ядра; оболочка остаётся неподвижной.

export interface GlavnyyProps {
  podpiski?: PodpiskaNaEkrane[];
  naObnovitPodpisku?: (id: string) => void;
  obnovlenieIdet?: boolean;
  obnovlyaemyePodpiski?: string[];
  pravila?: PravilaOtvet | null;
  zaderzhki?: ZamerZaderzhki[];
  zanyato?: boolean;
  naProverit?: () => void;
  proverkaIdet?: boolean;
  naVyborServera?: (id: string, podpiska?: string) => void;
  naTrafik?: (r: Marshrut) => void;
  naPravila?: () => void;
  skorost?: ReactNode;
  status: StatusOtvet;
  /** `null` is "no data yet" and renders as a dash, never as zero. */
  statistika?: Statistika | null;
  /** Known servers, to name the carrier instead of showing its id. */
  servery?: Server[];
  /** The whole listServers answer, when the tab has it; `null` means the
   *  list is simply not here, which is no longer a reason to show an id. */
  spisok?: SpisokServerov | null;
  spisokOtkaz?: OtkazNaEkrane | null;
  naDeystvie?: () => void;
  naServery?: () => void;
  naRezhim?: (r: Rezhim) => void;
}

function imyaServera(
  id: string | undefined,
  servery: Server[],
): string | undefined {
  if (!id) return undefined;
  return servery.find((s) => s.id === id)?.imya ?? id;
}

function transportServera(
  id: string | undefined,
  servery: Server[],
): string | undefined {
  return servery.find((s) => s.id === id)?.transport;
}

function nesushchiy(s: StatusOtvet, servery: Server[]): string {
  // The core names the carrier; our choice is a different question. In auto
  // mode vybran_id is empty by construction and that is not "none chosen".
  //
  // The frame's own name wins over the list: in auto mode the carrier changes
  // between two listServers answers, and the list is then the stale one.
  const imya =
    s.nesushchiy_imya ||
    imyaServera(s.nesushchiy_id, servery) ||
    imyaServera(s.vybran_id, servery);
  if (imya) return imya;
  return s.rezhim_marshruta === "avto"
    ? "сервер выберется автоматически"
    : "сервер не выбран";
}

function chislo(v: number | undefined, ed: string): string {
  return v === undefined ? PROCHERK : `${v} ${ed}`;
}

/** Bytes for humans: unit grows with the number, one decimal, Russian comma.
 *  Raw "471172330 Б" on the strip was unreadable (owner, 03.09.2026). */
export function obyom(v: number | undefined): string {
  if (v === undefined) return PROCHERK;
  if (v < 1024) return `${v} Б`;
  const edinicy = ["КБ", "МБ", "ГБ", "ТБ"];
  let z = v / 1024;
  let i = 0;
  while (z >= 1024 && i < edinicy.length - 1) {
    z /= 1024;
    i++;
  }
  return `${z.toFixed(1).replace(".", ",")} ${edinicy[i]}`;
}

/** "1 ч 05 мин" since the tunnel came up; a dash when it is not up. */
export function vSeti(
  podnyatS: string | undefined,
  seychas: number = Date.now(),
): string {
  if (!podnyatS) return PROCHERK;
  const t = Date.parse(podnyatS);
  if (Number.isNaN(t)) return PROCHERK;
  const min = Math.max(0, Math.floor((seychas - t) / 60000));
  const ch = Math.floor(min / 60);
  if (ch === 0) return `${min} мин`;
  return `${ch} ч ${String(min % 60).padStart(2, "0")} мин`;
}

export function Glavnyy({
  podpiski = [],
  naObnovitPodpisku,
  obnovlenieIdet = false,
  obnovlyaemyePodpiski = [],
  status,
  statistika = null,
  servery,
  spisok = null,
  spisokOtkaz = null,
  naDeystvie,
  naServery,
  naRezhim,
  pravila = null,
  zaderzhki = [],
  zanyato = false,
  naVyborServera,
  naTrafik,
  skorost,
  naProverit,
  proverkaIdet = false,
  naPravila,
}: GlavnyyProps) {
  const izvestnye = servery ?? spisok?.servery ?? [];
  const podnyat = status.sostoyanie === "podnyat";
  const skorosti = useSkorostTrafika(statistika, podnyat, `${status.podnyat_s ?? ""}:${status.nesushchiy_id ?? ""}`);
  const busy =
    status.sostoyanie === "podnimaetsya" ||
    status.sostoyanie === "vosstanavlivaetsya";
  const molchit = status.sostoyanie === "sluzhba-molchit";
  const rezhim = status.rezhim_marshruta ?? "avto";
  const selected = status.nesushchiy_id ?? status.vybran_id ?? spisok?.vybran;
  const loading = spisok === null && servery === undefined && !spisokOtkaz;
  const route = pravila?.trafik?.po_umolchaniyu ?? "vpn";
  const [query, setQuery] = useState("");
  const [spravka, zadatSpravku] = useState(false);
  const [seychas, zadatSeychas] = useState(() => Date.now());
  useEffect(() => {
    if (!podnyat) return;
    const timer = setInterval(() => zadatSeychas(Date.now()), 30000);
    return () => clearInterval(timer);
  }, [podnyat]);
  const action = busy
    ? "Отменить подключение"
    : (glavnoeDeystvie[status.sostoyanie] ?? "Отключить");
  const imya = podnyat
    ? nesushchiy(status, izvestnye)
    : rezhim === "avto"
      ? "Сервер выберется сам"
      : (imyaServera(selected, izvestnye) ?? "Сервер не выбран");
  const transport = podnyat ? transportServera(status.nesushchiy_id, izvestnye) : undefined;
  const gruppy = gruppyServerov(izvestnye, podpiski, spisok?.podpiska_uzel);
  const vsego = gruppy.reduce((n, g) => n + g.servery.length, 0);
  return (
    <section
      className="flex min-h-0 flex-1 flex-col"
      data-state={status.sostoyanie}
      aria-label="Подключение"
    >
      <div className="flex min-h-0 flex-1">
        {/* Левая колонка: состояние туннеля */}
        <div className="affory-connection border-border flex w-[42%] min-w-[380px] shrink-0 flex-col items-center overflow-y-auto border-r px-8 py-6">
          <div className="affory-identity flex w-full flex-1 flex-col items-center justify-center">
          <button
            type="button"
            className="affory-core group relative flex aspect-square w-full max-w-[260px] shrink-0 items-center justify-center rounded-3xl"
            aria-label={action}
            aria-pressed={podnyat}
            disabled={!naDeystvie || (zanyato && !busy)}
            data-testid="glavnoe-deystvie"
            onClick={naDeystvie}
          >
            <span
              aria-hidden
              className={`absolute inset-[22%] rounded-full blur-2xl transition-colors duration-500 ${
                podnyat ? "bg-accent/20" : "bg-accent/8"
              }`}
            />
            <img
              src={sphere}
              alt=""
              draggable={false}
              className={`relative h-full w-full select-none object-contain transition-all duration-500 ${
                podnyat || busy ? "opacity-100" : "opacity-45 saturate-0"
              }`}
            />
            <span className="affory-core-light" aria-hidden />
          </button>
          <div className="affory-status mt-2 flex flex-wrap items-baseline justify-center gap-x-2.5 gap-y-1 text-center">
            <span className="text-foreground text-base font-medium first-letter:uppercase" role="status" data-testid="sostoyanie">
              {podpis[status.sostoyanie]}
            </span>
            <span className={podnyat ? "text-fg-secondary text-sm" : "sr-only"}>
              <span className="sr-only">В сети </span>
              <span data-testid="v-seti">{vSeti(podnyat ? status.podnyat_s : undefined, seychas)}</span>
            </span>
          </div>
          <p className="affory-carrier text-foreground mt-2 w-full break-all text-center text-[17px] font-medium" data-testid="nesushchiy">
            <span className="affory-server-name">{imya}</span>
            {transport && <span className="affory-protocol text-fg-muted">{transport}</span>}
          </p>
          </div>

          <div className="affory-routing mt-5 w-full max-w-[380px]">
            <Segment
              aria-label="Куда идёт трафик"
              rastyanut
              ton="tihiy"
              aktiven={!(molchit || busy || zanyato || !pravila?.trafik || !naTrafik)}
              vybrano={route}
              naVybor={(r) => naTrafik?.(r as Marshrut)}
              znacheniya={[
                { z: "vpn", podpis: "Всё через VPN" },
                { z: "direct", podpis: "Только выбранное",disabled:status.kill_switch },
              ]}
            />
            <p className="text-fg-muted mt-2.5 text-center text-[13px]">
              {status.kill_switch ? "Включена блокировка сети вне VPN. Режим «Только выбранное» недоступен." : route === "vpn"
                ? "Через VPN, кроме прямых маршрутов"
                : "Через VPN только то, что в правилах"}
            </p>
            {/* Отсюда до списка исключений был один путь: догадаться про
                раздел «Правила» в полосе наверху. Подчёркивания нет, ссылка
                опознаётся цветом (правило интерфейса). */}
            {naPravila && (
              <p className="mt-1.5 text-center">
                <button
                  type="button"
                  data-testid="k-pravilam"
                  onClick={naPravila}
                  className="text-accent-ink hover:brightness-125 text-[13px] font-medium"
                >
                  Настроить правила
                </button>
              </p>
            )}
          </div>

          <div className="affory-traffic border-border mt-6 w-full max-w-[420px] border-t pt-5" data-testid="statistika">
            <dl className="grid grid-cols-2">
              <div className="border-border border-r pr-3">
                <Cifra nazvanie="↓ Приём" znachenie={formatSkorosti(skorosti.priem)} testId="skorost-priema" />
                <div className="affory-total text-fg-muted mt-2"><dt>Получено</dt><dd className="text-fg-secondary">{obyom(podnyat ? statistika?.prinyato : undefined)}</dd></div>
              </div>
              <div className="pl-4">
                <Cifra nazvanie="↑ Отдача" znachenie={formatSkorosti(skorosti.otdacha)} testId="skorost-otdachi" />
                <div className="affory-total text-fg-muted mt-2"><dt>Отправлено</dt><dd className="text-fg-secondary">{obyom(podnyat ? statistika?.otdano : undefined)}</dd></div>
              </div>
            </dl>
            {/* Задержка это итог блока, а не третья колонка: она одна на оба
                направления. Поэтому строка во всю ширину под своей чертой, в
                том же ритме, что «Получено» и «Отправлено» внутри колонок.
                По центру и без черты она читалась как подпись, случайно
                оставшаяся под таблицей (владелец, 19.09.2026). */}
            <dl
              className="affory-latency border-border mt-4 flex items-baseline justify-between border-t pt-3 text-[13px]"
              /* Пояснение нужно, потому что число сравнивают с чужими: с пингом
                 в игре и в Discord. Оно и правда про то же - один круг, а не
                 весь запрос с рукопожатием (21.09.2026). */
              title="Сколько идёт один круг до интернета через VPN. То же, что пинг в играх и в Discord."
            >
              <dt className="text-fg-muted">Задержка</dt>
              <dd className="text-foreground font-medium" data-testid="zaderzhka">{podnyat ? chislo(statistika?.zaderzhka_ms, "мс") : PROCHERK}</dd>
            </dl>
          </div>
        </div>

        {/* Правая колонка: набор серверов и замер полосы */}
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-8 py-6">
            <header className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                {/* Значок справки стоит здесь, а не только на экране
                    «Управлять»: имя протокола человек читает именно в этом
                    списке, и вопрос «что из этого брать» возникает тут. */}
                <div className="flex items-center gap-2">
                  <h2 className="text-foreground text-[22px] font-semibold leading-tight">Серверы</h2>
                  <KnopkaSpravki onClick={() => zadatSpravku(true)} />
                </div>
                <p className="text-fg-muted mt-1 text-[13px]">
                  {spisok
                    ? `${vsego} ${slovoPosleChisla(vsego, "сервер", "сервера", "серверов")}${podpiski.length ? ` · ${podpiski.length} ${slovoPosleChisla(podpiski.length, "подписка", "подписки", "подписок")}` : " в списке"}`
                    : spisokOtkaz
                      ? "Список не прочитался"
                      // «Читаю список» при молчащей службе это обещание работы,
                      // которой не идёт: читать не у кого.
                      : molchit ? "Служба не отвечает" : "Читаю список"}
                </p>
              </div>
              <Knopka rang="vtoraya" bolshaya onClick={naServery}>Управлять</Knopka>
            </header>

            <div className="flex flex-col gap-2">
              <Segment
                aria-label="Как выбирается сервер"
                rastyanut
                ton="tihiy"
                aktiven={!(molchit || busy || zanyato)}
                vybrano={rezhim}
                naVybor={(r) => naRezhim?.(r as Rezhim)}
                znacheniya={[
                  { z: "avto", podpis: "Автоматически" },
                  { z: "ruchnoy", podpis: "Вручную" },
                ]}
              />
              {/* Служба применяет выбор на живом ядре через clash_api, поэтому
                  обещать «на следующем подключении» на поднятом туннеле
                  значит врать. Подпись зависит от того, поднят ли он. */}
              <p className="text-fg-muted text-[13px]" data-testid="podskazka-rezhima">
                {rezhim === "avto"
                  ? "Сервер выбирается сам, по отклику"
                  : podnyat
                    ? "Нажми на сервер, чтобы перейти на него сразу"
                    : "Нажми на сервер, чтобы подключиться к нему"}
              </p>
            </div>

            <div className="flex items-center gap-3">
              <Poisk
                id="af-server-search"
                aria-label="Поиск сервера"
                placeholder="Найти сервер..."
                znachenie={query}
                naVvod={setQuery}
              />
              <Knopka
                rang="vtoraya"
                bolshaya
                zhdyot={proverkaIdet}
                aktiven={!(molchit || busy || zanyato || loading || !izvestnye.length || !naProverit)}
                onClick={naProverit}
              >
                {proverkaIdet ? "Проверяю отклик" : "Проверить серверы"}
              </Knopka>
            </div>

            {!loading && !spisokOtkaz && gruppy.length > 0 && <KatalogServerov
              servery={izvestnye} podpiski={podpiski} uzel={spisok?.podpiska_uzel} zapros={query}
              zaderzhki={zaderzhki} nesushchiy={status.nesushchiy_id} vybran={selected} podnyat={podnyat}
              disabled={molchit || busy || zanyato} naVybor={naVyborServera}
              naObnovit={naObnovitPodpisku} obnovlyaetsya={obnovlenieIdet} obnovlyaemyePodpiski={obnovlyaemyePodpiski}
            />}
            <ul>
              {loading && (
                <li className="text-fg-muted border-border rounded-lg border px-4 py-6 text-center text-[13px]" role="status">
                  {molchit ? "Служба не отвечает, список серверов недоступен" : "Читаю список серверов"}
                </li>
              )}
              {spisokOtkaz && (
                <li className="text-fg-muted border-border rounded-lg border px-4 py-6 text-center text-[13px]">
                  Список серверов недоступен. Причина и повторная загрузка указаны выше
                </li>
              )}
              {!loading && !spisokOtkaz && vsego === 0 && (
                <li className="border-border flex flex-col items-center gap-2 rounded-lg border border-dashed px-4 py-10 text-center">
                  <span className="text-foreground text-sm font-medium">
                    {query ? "Ничего не найдено" : "Серверов пока нет"}
                  </span>
                  <span className="text-fg-muted text-[13px]">
                    {query
                      ? "Попробуй другое имя или адрес"
                      : "Вставь ссылку на сервер или подписку"}
                  </span>
                  {!query && (
                    <Knopka rang="glavnaya" className="mt-1" onClick={naServery}>
                      Добавить сервер
                    </Knopka>
                  )}
                </li>
              )}
            </ul>
          </div>

          {skorost}
        </div>
      </div>

      <footer className="border-border bg-rail text-fg-muted flex h-11 shrink-0 items-center justify-between border-t px-6 text-[13px]">
        <span>
          Адрес выхода{" "}
          <b className="text-fg-secondary font-medium">{podnyat ? statistika?.adres_vyhoda || PROCHERK : PROCHERK}</b>
        </span>
        {/* Без номера, а не со словом «dev»: версия приходит от службы, и её
            отсутствие значит «служба молчит», что и так написано выше. Во
            время обновления служба молчит намеренно, и подпись «Affory dev»
            прочли 13.09.2026 как подмену сборкой разработчика. */}
        <span>
          Affory{" "}
          {status.versiya_programmy && <b className="text-fg-secondary font-medium">{status.versiya_programmy}</b>}
        </span>
      </footer>

      {spravka && (
        <SpravkaProtokolov
          zakryt={() => zadatSpravku(false)}
          svoi={izvestnye.map((s) => s.transport)}
        />
      )}
    </section>
  );
}

function Cifra({
  nazvanie,
  znachenie,
  testId,
}: {
  nazvanie: string;
  znachenie: string;
  testId?: string;
}) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-fg-muted text-[13px]">{nazvanie}</dt>
      <dd className="affory-rate text-foreground font-semibold" data-testid={testId}>
        {znachenie.endsWith(" Мбит/с")
          ? <>{znachenie.replace(" Мбит/с", "")} <span className="affory-rate-unit">Мбит/с</span></>
          : znachenie}
      </dd>
    </div>
  );
}
