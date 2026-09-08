import { useEffect, useState, type ReactNode } from "react";
import type {
  OtkazNaEkrane,
  Rezhim,
  Server,
  Statistika,
  StatusOtvet,
} from "../protokol";
import { glavnoeDeystvie } from "./podpisi";
import { Zaderzhka, type SpisokServerov, type ZamerZaderzhki } from "./Servery";
import type { PravilaOtvet } from "./Pravila";
import type { Marshrut } from "../trafik";
import { VyborTrafika } from "./Marshruty";
import sphere from "../assets/affory-sphere.png";
import { Knopka, PROCHERK } from "./ui";
import { CheckIcon } from "./Vybor";

// Pure over props. No subscription, no bridge, no runtime import: App.tsx
// owns the wiring and hands the whole StatusOtvet down in one piece. Half
// the state arriving in two events is exactly how two halves start to
// disagree, so there is one prop, not five.
//
// Redrawn 02.09.2026 on the accepted mockup: one card with the state, the
// server by NAME, the one action; five numbers in a strip under it; the
// route mode segment below (§5 item 2 lives here per §8.3).

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
  naPravila,
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
  const effectiveRoute = status.trafik_po_umolchaniyu ?? route;
  const message =
    status.sostoyanie === "vyklyuchen"
      ? "Интернет работает напрямую"
      : podnyat
        ? effectiveRoute === "direct"
          ? "VPN для выбранных приложений и сайтов"
          : "Соединение через VPN установлено"
        : status.sostoyanie === "podnimaetsya"
          ? "Устанавливаем соединение…"
          : status.sostoyanie === "vosstanavlivaetsya"
            ? "Восстанавливаем соединение…"
            : status.sostoyanie === "ne-neset"
              ? "Соединение не передаёт трафик"
              : molchit
                ? "Нет связи со службой"
                : "Не удалось подключиться";
  const action = busy
    ? "Отменить подключение"
    : (glavnoeDeystvie[status.sostoyanie] ?? "Отключить");
  const shown = izvestnye.filter((s) =>
    `${s.imya} ${s.host} ${s.transport}`
      .toLowerCase()
      .includes(query.toLowerCase()),
  );
  return (
    <section
      className="af-home"
      data-state={status.sostoyanie}
      aria-label="Подключение"
    >
      <div className="af-home-grid">
        <div className="af-home-left">
          <div className="af-state" role="status" data-testid="sostoyanie">
            <span className="af-state-dot" aria-hidden />
            <span>{message}</span>
          </div>
          <button
            type="button"
            className="af-power"
            aria-label={action}
            aria-pressed={podnyat}
            disabled={!naDeystvie || (zanyato && !busy)}
            data-testid="glavnoe-deystvie"
            onClick={naDeystvie}
          >
            <span className="af-orbit" aria-hidden />
            <img src={sphere} alt="" draggable={false} />
          </button>
          <p className="af-power-hint">
            {busy
              ? "Нажмите на сферу, чтобы отменить"
              : molchit
                ? "Нажмите на сферу, чтобы повторить"
                : "Нажмите на сферу, чтобы " +
                  (podnyat || status.sostoyanie === "ne-neset"
                    ? "отключиться"
                    : "подключиться")}
          </p>
          <button
            type="button"
            className="af-current"
            data-testid="smenit-server"
            onClick={() => document.getElementById("af-server-search")?.focus()}
          >
            <span data-testid="nesushchiy">
              {podnyat
                ? nesushchiy(status, izvestnye)
                : rezhim === "avto"
                  ? "Лучший доступный сервер"
                  : (imyaServera(selected, izvestnye) ??
                    "Выберите сервер справа")}
            </span>
            {podnyat && (
              <small>{transportServera(status.nesushchiy_id, izvestnye)}</small>
            )}
          </button>
          <div className="af-traffic-control">
            <div className="af-section-label">
              <span>Куда направлять трафик</span>
              <button className="af-link" type="button" onClick={naPravila}>
                Правила ↗
              </button>
            </div>
            <VyborTrafika
              value={route}
              disabled={
                molchit || busy || zanyato || !pravila?.trafik || !naTrafik
              }
              onChange={(r) => naTrafik?.(r)}
            />
            <p className="af-note">
              {route === "vpn"
                ? "Через туннель, кроме ваших прямых маршрутов."
                : "Напрямую, кроме выбранных приложений и сайтов."}
            </p>
          </div>
          <dl className="af-stats" data-testid="statistika">
            <Cifra
              nazvanie="задержка VPN"
              znachenie={
                podnyat ? chislo(statistika?.zaderzhka_ms, "мс") : PROCHERK
              }
            />
            <Cifra
              nazvanie="получено"
              znachenie={obyom(podnyat ? statistika?.prinyato : undefined)}
            />
            <Cifra
              nazvanie="отправлено"
              znachenie={obyom(podnyat ? statistika?.otdano : undefined)}
            />
          </dl>
        </div>
        <div className="af-home-right">
          <div className="af-pane-head">
            <div>
              <h2>Серверы</h2>
              <p>
                {spisok
                  ? `${izvestnye.length} доступно в вашем списке`
                  : spisokOtkaz ? "Не удалось загрузить список" : "Загружаем список…"}
              </p>
            </div>
            <button type="button" className="af-manage" onClick={naServery}>
              Управлять
            </button>
          </div>
          <div className="af-section-label">Выбор сервера</div>
          <div className="af-segment" role="group" aria-label="режим маршрута">
            <button
              type="button"
              aria-pressed={rezhim === "avto"}
              disabled={molchit || busy || zanyato}
              onClick={() => naRezhim?.("avto")}
            >
              Автоматически
            </button>
            <button
              type="button"
              aria-pressed={rezhim === "ruchnoy"}
              disabled={molchit || busy || zanyato}
              onClick={() => naRezhim?.("ruchnoy")}
            >
              Вручную
            </button>
          </div>
          <p className="af-mode-note">
            {rezhim === "avto"
              ? "Affory выбирает сервер с наименьшей задержкой."
              : "Нажмите на сервер, чтобы сразу подключиться к нему."}
          </p>
          <div className="af-server-tools">
          <input
            className="af-search"
            id="af-server-search"
            aria-label="Поиск сервера"
            placeholder="Найти сервер…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <button type="button" className="af-manage" disabled={molchit || busy || zanyato || loading || !izvestnye.length || !naProverit} onClick={naProverit}>
            {proverkaIdet ? "Проверяем…" : "Проверить серверы"}
          </button>
          </div>
          <p className="af-delay-note">Последняя проверка: VPN через туннель, узел при выключенном VPN.</p>
          <div className="af-server-list" aria-label="Список серверов">
            {shown.map((server) => {
              const active = podnyat && status.nesushchiy_id === server.id;
              const zamer = zaderzhki.find((z) => z.id === server.id);
              return (
                <button
                  type="button"
                  className="af-server-row"
                  key={server.id}
                  disabled={molchit || busy || zanyato || !naVyborServera}
                  aria-label={`Подключиться к ${server.imya}`}
                  aria-pressed={active}
                  onClick={() => naVyborServera?.(server.id)}
                >
                  <span className="af-server-mark" aria-hidden>
                    {active ? <CheckIcon /> : <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><rect x="4" y="4" width="16" height="6" rx="2" /><rect x="4" y="14" width="16" height="6" rx="2" /><path d="M8 7h.01M8 17h.01" /></svg>}
                  </span>
                  <span className="af-server-copy">
                    <b>{server.imya}</b>
                    <small>{server.transport}</small>
                  </span>
                  <span className="af-server-ping">
                    <Zaderzhka zamer={zamer} compact />
                  </span>
                  <span className="af-server-action" aria-hidden="true" title={active ? "Подключён" : "Подключиться"}>
                    {active ? <CheckIcon /> : <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3v9M6.3 5.7a8 8 0 1 0 11.4 0" /></svg>}
                  </span>
                </button>
              );
            })}
            {loading && <div className="af-empty" role="status">Загружаем серверы…</div>}
            {spisokOtkaz && <div className="af-empty">Список серверов недоступен. Причина и повторная загрузка указаны выше.</div>}
            {!loading && !spisokOtkaz && shown.length === 0 && (
              <div className="af-empty">
                <b>{query ? "Ничего не найдено" : "Добавьте первый сервер"}</b>
                <p>
                  {query
                    ? "Попробуйте другое имя или адрес."
                    : "Вставьте ссылку на сервер или подписку."}
                </p>
                {!query && (
                  <Knopka rang="glavnaya" onClick={naServery}>
                    Добавить сервер
                  </Knopka>
                )}
              </div>
            )}
          </div>
          {skorost}
        </div>
      </div>
      <footer className="af-session">
        <span>
          Адрес выхода{" "}
          <b>{podnyat ? statistika?.adres_vyhoda || PROCHERK : PROCHERK}</b>
        </span>
        <span>
          В сети{" "}
          <b data-testid="v-seti">
            {vSeti(podnyat ? status.podnyat_s : undefined, seychas)}
          </b>
        </span>
        <span>Affory {status.versiya_programmy ?? "dev"}</span>
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
    <div className="border-border flex flex-1 flex-col gap-0.5 border-l px-4 py-2.5 first:border-l-0">
      <dt className="text-fg-muted text-[11px]">{nazvanie}</dt>
      <dd
        className="text-foreground text-[15px] font-medium"
        data-testid={testId}
      >
        {znachenie}
      </dd>
    </div>
  );
}
