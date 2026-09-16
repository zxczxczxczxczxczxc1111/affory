import { useEffect, useState, type ReactNode } from "react";
import type {
  OtkazNaEkrane,
  Rezhim,
  Server,
  Statistika,
  StatusOtvet,
} from "../protokol";
import { glavnoeDeystvie, podpis } from "./podpisi";
import { Zaderzhka, type SpisokServerov, type ZamerZaderzhki } from "./Servery";
import type { PravilaOtvet } from "./Pravila";
import type { Marshrut } from "../trafik";
import sphere from "../assets/affory-sphere.png";
import { Knopka, Poisk, PROCHERK, Segment, Vertushka } from "./ui";
import { IkGalka, IkPitanie, IkServer } from "../ikonki";
import { slovoPosleChisla } from "../chisla";

// Pure over props. No subscription, no bridge, no runtime import: App.tsx
// owns the wiring and hands the whole StatusOtvet down in one piece. Half
// the state arriving in two events is exactly how two halves start to
// disagree, so there is one prop, not five.
//
// Перерисован 16.09.2026 по макетам владельца: две колонки, слева состояние
// туннеля, справа набор серверов, внизу служебная строка на всю ширину.
// Отдельной кнопки «Отключить» под сферой нет: сфера и есть выключатель.
// Постоянной анимации у сферы тоже нет, четыре варианта были показаны и
// отвергнуты все четыре.

export interface GlavnyyProps {
  pravila?: PravilaOtvet | null;
  zaderzhki?: ZamerZaderzhki[];
  zanyato?: boolean;
  naProverit?: () => void;
  proverkaIdet?: boolean;
  naVyborServera?: (id: string) => void;
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

/** Цвет точки состояния. Зелёного нет: сиреневый уже значит «поднят», а
 *  второй цвет с тем же смыслом это начало смерти палитры. */
const TOCHKA: Record<string, string> = {
  podnyat: "bg-accent-ink",
  "ne-neset": "bg-warn",
  podnimaetsya: "bg-warn",
  vosstanavlivaetsya: "bg-warn",
  otkaz: "bg-danger",
  "sluzhba-molchit": "bg-danger",
  vyklyuchen: "bg-fg-faint",
};

export function Glavnyy({
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
}: GlavnyyProps) {
  const izvestnye = servery ?? spisok?.servery ?? [];
  const podnyat = status.sostoyanie === "podnyat";
  const busy =
    status.sostoyanie === "podnimaetsya" ||
    status.sostoyanie === "vosstanavlivaetsya";
  const molchit = status.sostoyanie === "sluzhba-molchit";
  const rezhim = status.rezhim_marshruta ?? "avto";
  const selected = status.nesushchiy_id ?? status.vybran_id ?? spisok?.vybran;
  const loading = spisok === null && servery === undefined && !spisokOtkaz;
  const route = pravila?.trafik?.po_umolchaniyu ?? "vpn";
  const [query, setQuery] = useState("");
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
  const shown = izvestnye.filter((s) =>
    `${s.imya} ${s.host} ${s.transport}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  return (
    <section
      className="flex min-h-0 flex-1 flex-col"
      data-state={status.sostoyanie}
      aria-label="Подключение"
    >
      <div className="flex min-h-0 flex-1">
        {/* Левая колонка: состояние туннеля */}
        <div className="border-border flex w-[42%] min-w-[380px] shrink-0 flex-col items-center overflow-y-auto border-r px-8 py-6">
          {/* Подпись состояния приходит из podpisi.ts строчными: заглавная
              делается кромкой, а не второй копией семи строк. */}
          <div
            className="border-border bg-elevated text-foreground inline-flex h-9 items-center gap-2.5 rounded-full px-3.5 text-sm font-medium first-letter:uppercase"
            role="status"
            data-testid="sostoyanie"
          >
            {busy
              ? <Vertushka className="text-warn h-3.5 w-3.5" />
              : <span className={`h-2 w-2 shrink-0 rounded-full ${TOCHKA[status.sostoyanie] ?? "bg-fg-faint"}`} aria-hidden />}
            {podpis[status.sostoyanie]}
          </div>

          <button
            type="button"
            className="group relative mt-6 flex aspect-square w-full max-w-[320px] shrink-0 items-center justify-center rounded-full"
            aria-label={action}
            aria-pressed={podnyat}
            disabled={!naDeystvie || (zanyato && !busy)}
            data-testid="glavnoe-deystvie"
            onClick={naDeystvie}
          >
            <span
              aria-hidden
              className={`absolute inset-0 rounded-full border transition-all duration-500 ${
                podnyat ? "border-accent/45" : "border-border"
              } group-hover:border-accent-ink/60 group-active:scale-[0.985]`}
            />
            <span
              aria-hidden
              className={`absolute inset-[22%] rounded-full blur-2xl transition-colors duration-500 ${
                podnyat ? "bg-accent/40" : "bg-accent/12"
              }`}
            />
            <img
              src={sphere}
              alt=""
              draggable={false}
              className={`relative h-[84%] w-[84%] select-none object-contain transition-all duration-500 ${
                podnyat ? "opacity-100" : "opacity-45 saturate-0"
              }`}
            />
            {/* Слово о том, что сделает нажатие, проступает на наведении: на
                самой сфере нет ни одной надписи, и без него она молчит. */}
            <span className="text-fg-secondary pointer-events-none absolute bottom-[6%] inline-flex items-center gap-1.5 rounded-full bg-black/70 px-3 py-1 text-[13px] opacity-0 transition-opacity duration-150 group-hover:opacity-100 group-focus-visible:opacity-100">
              {busy
                ? <><Vertushka className="h-3.5 w-3.5" />{action}</>
                : <><IkPitanie className="h-3.5 w-3.5" />{action}</>}
            </span>
          </button>

          <p className="text-foreground mt-5 text-center text-[17px] font-medium" data-testid="nesushchiy">
            {imya}
            {transport && <span className="text-fg-muted"> · {transport}</span>}
          </p>

          <div className="mt-5 w-full max-w-[380px]">
            <Segment
              aria-label="Куда идёт трафик"
              rastyanut
              ton="tihiy"
              aktiven={!(molchit || busy || zanyato || !pravila?.trafik || !naTrafik)}
              vybrano={route}
              naVybor={(r) => naTrafik?.(r as Marshrut)}
              znacheniya={[
                { z: "vpn", podpis: "Всё через VPN" },
                { z: "direct", podpis: "Только выбранное" },
              ]}
            />
            <p className="text-fg-muted mt-2.5 text-center text-[13px]">
              {route === "vpn"
                ? "Через VPN, кроме прямых маршрутов"
                : "Через VPN только то, что в правилах"}
            </p>
          </div>

          <dl className="border-border mt-6 grid w-full max-w-[420px] grid-cols-3 border-t pt-5 text-center" data-testid="statistika">
            <Cifra
              nazvanie="Задержка"
              znachenie={podnyat ? chislo(statistika?.zaderzhka_ms, "мс") : PROCHERK}
            />
            <Cifra
              nazvanie="Получено"
              znachenie={obyom(podnyat ? statistika?.prinyato : undefined)}
            />
            <Cifra
              nazvanie="Отправлено"
              znachenie={obyom(podnyat ? statistika?.otdano : undefined)}
            />
          </dl>
        </div>

        {/* Правая колонка: набор серверов и замер полосы */}
        <div className="flex min-w-0 flex-1 flex-col">
          <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-8 py-6">
            <header className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <h2 className="text-foreground text-[22px] font-semibold leading-tight">Серверы</h2>
                <p className="text-fg-muted mt-1 text-[13px]">
                  {spisok
                    ? `${izvestnye.length} ${slovoPosleChisla(izvestnye.length, "сервер", "сервера", "серверов")} в списке`
                    : spisokOtkaz
                      ? "Список не прочитался"
                      // «Читаю список» при молчащей службе это обещание работы,
                      // которой не идёт: читать не у кого.
                      : molchit ? "Список некому отдать" : "Читаю список"}
                </p>
              </div>
              <Knopka rang="vtoraya" bolshaya onClick={naServery}>Управлять</Knopka>
            </header>

            <div className="flex flex-col gap-2">
              <Segment
                aria-label="Как выбирается сервер"
                rastyanut
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

            <ul className="flex flex-col gap-1" aria-label="Список серверов">
              {shown.map((server) => {
                const active = podnyat && status.nesushchiy_id === server.id;
                const zamer = zaderzhki.find((z) => z.id === server.id);
                return (
                  <li key={server.id}>
                    <button
                      type="button"
                      className={`group/ryad flex h-[52px] w-full items-center gap-3 rounded-lg px-3 text-left transition-colors disabled:pointer-events-none disabled:opacity-45 ${
                        active ? "bg-accent-soft" : "hover:bg-surface-hover"
                      }`}
                      disabled={molchit || busy || zanyato || !naVyborServera}
                      aria-label={`Подключиться к ${server.imya}`}
                      aria-pressed={active}
                      onClick={() => naVyborServera?.(server.id)}
                    >
                      <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-md border ${active ? "border-accent/60 text-accent-ink" : "border-border text-fg-muted"}`} aria-hidden>
                        <IkServer className="h-4 w-4" />
                      </span>
                      <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                        <span className={`truncate text-sm font-medium ${active ? "text-foreground" : "text-fg-secondary"}`}>{server.imya}</span>
                        <span className="text-fg-muted flex items-center gap-1.5 truncate text-[13px]">
                          {server.transport}
                          {zamer && <><span aria-hidden>·</span><Zaderzhka zamer={zamer} compact /></>}
                        </span>
                      </span>
                      <span className="flex h-7 w-7 shrink-0 items-center justify-center" aria-hidden>
                        {active ? (
                          <span className="bg-accent text-foreground flex h-6 w-6 items-center justify-center rounded-full">
                            <IkGalka className="h-3.5 w-3.5" />
                          </span>
                        ) : (
                          // Пустой кружок, а не значок питания: строка не
                          // выключает сервер, она выбирает, через какой идти.
                          <span className="border-border-active group-hover/ryad:border-accent-ink h-[18px] w-[18px] rounded-full border transition-colors" />
                        )}
                      </span>
                    </button>
                  </li>
                );
              })}
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
              {!loading && !spisokOtkaz && shown.length === 0 && (
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
        <span>
          В сети{" "}
          <b className="text-fg-secondary font-medium" data-testid="v-seti">
            {vSeti(podnyat ? status.podnyat_s : undefined, seychas)}
          </b>
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
      <dd className="text-foreground text-[19px] font-semibold leading-none" data-testid={testId}>
        {znachenie}
      </dd>
    </div>
  );
}
