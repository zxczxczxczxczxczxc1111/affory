import { useMemo, useState } from "react";
import type { OtkazNaEkrane, OtkazStroki, Server, StatusOtvet } from "../protokol";
import { slovoPosleChisla } from "../chisla";
import { IkonkaKorzina, Karta, Knopka, Kolonka, Neudacha, Pole, Razdel, Ryad, Segment, Shapka, Teg } from "./ui";

// Servers tab (task 4.9). Pure over props like every screen: App fetches
// listServers and hands the answer down whole; every button sends one named
// command up and waits for the list to come back changed.
//
// Two ids, two questions. `vybran` (and status.vybran_id) is the human's
// choice; status.nesushchiy_id is what the core actually carries. In auto
// mode the choice is empty by construction, and that is not "nothing chosen".

/** Answer body of listServers, see cmd/affory-svc/komandy_serverov.go. */
/** Schemes the service accepts as one server link. Kept next to the form so
 *  the clipboard button refuses junk without a round trip. */
const SHEMY = ["vless://", "hy2://", "hysteria2://", "ss://", "trojan://", "vmess://"];
export function pohozheNaSsylku(t: string): boolean {
  const s = t.trim().toLowerCase();
  return SHEMY.some((sh) => s.startsWith(sh));
}

export interface SpisokServerov {
  servery: Server[];
  vybran: string;
  podpiska_zadana: boolean;
  /** Host of the subscription URL. The URL itself is a secret of the same
   *  class as a key and never leaves the service. */
  podpiska_uzel: string;
  podpiska_obnovlena?: string;
}

/** Замер одного сервера: два числа, потому что они про разное.
 *
 *  `tcping` это дорога до узла, мимо туннеля, и туннель для неё не нужен.
 *  `realping` это весь путь через туннель вместе с рукопожатием. Узел,
 *  отвечающий на TCP мгновенно и не несущий ни байта, по одной цифре
 *  неотличим от далёкого, но исправного. Отсутствие числа это отказ с
 *  текстом, а НЕ ноль: ноль читается как «мгновенно» и ставит мёртвый узел
 *  первым по задержке. */
export interface ZamerZaderzhki {
  id: string;
  tcping_ms?: number | null;
  tcping_otkaz?: string;
  realping_ms?: number | null;
  realping_otkaz?: string;
}

export interface ServeryProps {
  status: StatusOtvet;
  /** `null` until the first listServers answer arrives. */
  spisok: SpisokServerov | null;
  /** Why the list is missing, when it is missing because of a refusal and
   *  not because it is still on its way. `null` means "still on its way". */
  spisokOtkaz?: OtkazNaEkrane | null;
  /** Repeats listServers. A refusal without a way back is a dead end, and
   *  this tab had one: no list, no button, no form (03.09.2026). */
  obnovitSpisok?: () => void;
  naKomandu: (komanda: string, telo: unknown) => void;
  /** Строки последней подписки, которые не разобрались. Пустой список и
   *  отсутствие это одно и то же: разговора нет. */
  otkazyPodpiski?: OtkazStroki[];
  /** Reads the clipboard through the shell; absent in tests that do not care. */
  chitatBufer?: () => Promise<string>;
  /** Shell-side screen QR: resolves with the added server name, rejects with the reason. */
  naQrSEkrana?: () => Promise<string>;
  /** Последний замер задержек, по одному на сервер. Пусто значит «не мерили»,
   *  и тогда строки задержки нет вовсе: пустая строка врала бы про замер,
   *  которого не было. */
  zaderzhki?: ZamerZaderzhki[];
}

const TRANSPORT: Record<string, string> = {
  // xhttp left here ON PURPOSE after the core dropped it on 06.09.2026. The Go
  // registry no longer lists it, so nothing new can carry it, but records saved
  // by older versions still sit in people's lists. Delete this key and such a
  // record shows an empty cell, which reads as a broken entry rather than an
  // unsupported one. It names itself, and connecting to it fails by name.
  xhttp: "xhttp", "reality-tcp": "reality", ws: "websocket", grpc: "grpc", hy2: "hysteria2", ss: "shadowsocks",
  // Added 03.09.2026. A transport missing from this map shows as an empty
  // cell in the list, which reads as a broken record rather than a new one.
  httpupgrade: "httpupgrade", trojan: "trojan", "trojan-ws": "trojan + websocket",
  vmess: "vmess", "vmess-ws": "vmess + websocket",
  // Added 05.09.2026. The Go-side test TestEkranNazyvaetVseTransportyYadra now
  // compares this map against the core registry, so a transport added without
  // a name here fails the build instead of showing an empty cell.
  anytls: "anytls", tuic: "tuic",
};

/** "3 ч назад" for the subscription row; `undefined` when unknown. */
export function vozrast(iso: string | undefined, seychas: number = Date.now()): string | undefined {
  if (!iso) return undefined;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return undefined;
  const min = Math.max(0, Math.round((seychas - t) / 60000));
  if (min < 1) return "только что";
  if (min < 60) return `${min} мин назад`;
  const ch = Math.round(min / 60);
  if (ch < 48) return `${ch} ч назад`;
  return `${Math.round(ch / 24)} дн назад`;
}

function sovpadaet(s: Server, zapros: string): boolean {
  const z = zapros.trim().toLowerCase();
  if (!z) return true;
  return s.imya.toLowerCase().includes(z) || s.host.toLowerCase().includes(z);
}

export function Servery({ status, spisok, spisokOtkaz = null, obnovitSpisok, naKomandu, otkazyPodpiski = [], chitatBufer, naQrSEkrana, zaderzhki = [] }: ServeryProps) {
  const aktiven = status.sostoyanie !== "sluzhba-molchit";
  const [poisk, zadatPoisk] = useState("");
  // One "Добавить", one question: what is being added. A server link and a
  // subscription address used to live behind two buttons with two verbs
  // (owner's remark 02.09.2026), and that was two meanings for one act.
  const [dobavlyayu, zadatDobavlyayu] = useState(false);
  const [chto, zadatChto] = useState<"server" | "podpiska">("server");
  const [ssylka, zadatSsylku] = useState("");
  // Outcome line under the link field: the clipboard held junk, the screen
  // held no QR, or a server was added from it. Never the link text itself.
  const [ishodVvoda, zadatIshodVvoda] = useState<string | null>(null);
  // The shell's refusal is a refusal, not a footnote: it used to be small grey
  // text under the field, past §9.1 and with no way to try again.
  // Своя фраза, а не OtkazNaEkrane: чтение QR делает окно, и кода на проводе у
  // этого отказа нет. Прежде здесь стоял выдуманный `qr-s-ekrana`, которого нет
  // ни в словаре протокола, ни в §9.1, ни в otkazy.ts.
  const [otkazQr, zadatOtkazQr] = useState<string | null>(null);
  const [adres, zadatAdres] = useState("");
  // Two-click delete: the first click turns the icon into a question, the
  // second answers it. One click on a trash icon next to the row you are
  // hovering is how a server disappears by accident.
  const [udalyayu, zadatUdalyayu] = useState<string | null>(null);

  const servery = spisok?.servery ?? [];
  const vidimye = useMemo(() => servery.filter((s) => sovpadaet(s, poisk)), [servery, poisk]);
  // По id, а не поиском в списке на каждую строку: серверов бывает под шесть
  // десятков, и вложенный обход превратил бы отрисовку в квадрат.
  const poZaderzhkam = useMemo(() => new Map(zaderzhki.map((z) => [z.id, z])), [zaderzhki]);
  const vybran = spisok?.vybran || status.vybran_id || "";
  const vybranPropal = vybran !== "" && !servery.some((s) => s.id === vybran);
  const avto = status.rezhim_marshruta === "avto";
  const izPodpiski = servery.filter((s) => s.iz_podpiski).length;

  // The form opens by itself only on the honest empty list (§9.2 first run).
  // A refused list is NOT an empty one, so it gets the header button instead.
  const pervyyZapusk = spisok !== null && servery.length === 0;

  const svodka = spisok === null
    ? spisokOtkaz
      ? "список серверов не прочитался"
      : undefined
    : servery.length === 0
      ? "серверов пока нет"
      : `${servery.length} ${sklon(servery.length)}${izPodpiski ? `, ${izPodpiski} из подписки` : ""} · ${
        avto ? "выбор автоматический" : vybranPropal ? "выбранного нет в списке" : `выбран ${imyaPo(servery, vybran)}`
      }`;

  const izBufera = async () => {
    if (!chitatBufer) return;
    const t = (await chitatBufer()).trim();
    if (!pohozheNaSsylku(t)) {
      zadatIshodVvoda(t ? "в буфере не ссылка на сервер" : "буфер обмена пуст");
      return;
    }
    naKomandu("addServer", { ssylka: t });
    zadatIshodVvoda(null);
    zadatSsylku("");
    zadatDobavlyayu(false);
  };
  const sEkrana = async () => {
    if (!naQrSEkrana) return;
    zadatOtkazQr(null);
    try {
      const imya = await naQrSEkrana();
      zadatIshodVvoda(imya ? "добавлен " + imya : null);
    } catch (e: unknown) {
      zadatIshodVvoda(null);
      zadatOtkazQr(e instanceof Error ? e.message : String(e));
    }
  };
  const otpravitSsylku = () => {
    const s = ssylka.trim();
    if (!s) return;
    naKomandu("addServer", { ssylka: s });
    zadatSsylku("");
    zadatDobavlyayu(false);
  };
  const otpravitAdres = () => {
    const a = adres.trim();
    if (!a) return;
    naKomandu("setSubscription", { adres: a });
    zadatAdres("");
    zadatDobavlyayu(false);
  };
  const zakrytFormu = () => { zadatDobavlyayu(false); zadatSsylku(""); zadatAdres(""); };

  const otmena = !pervyyZapusk && <Knopka rang="tekst" onClick={zakrytFormu}>отмена</Knopka>;
  const forma = (
    <Karta testId="forma">
      <div className="flex flex-col gap-3 px-4 py-3">
        <Segment<"server" | "podpiska">
          aria-label="что добавить"
          znacheniya={[{ z: "server", podpis: "сервер по ссылке" }, { z: "podpiska", podpis: "подписка" }]}
          vybrano={chto}
          naVybor={zadatChto}
        />
        {chto === "server" ? (
          <div className="flex items-center gap-2">
            <Pole
              testId="ssylka"
              aria-label="ссылка на сервер"
              znachenie={ssylka}
              naVvod={zadatSsylku}
              placeholder="vless://, hy2:// или ss:// из буфера"
              aktiven={aktiven}
              className="flex-1"
            />
            <Knopka rang="glavnaya" testId="dobavit-ssylku" aktiven={aktiven && ssylka.trim() !== ""} onClick={otpravitSsylku}>
              Добавить
            </Knopka>
            {chitatBufer && (
              <Knopka rang="vtoraya" testId="iz-bufera" aktiven={aktiven} onClick={() => void izBufera()}>
                из буфера
              </Knopka>
            )}
            {naQrSEkrana && (
              <Knopka rang="vtoraya" testId="qr-s-ekrana" aktiven={aktiven} onClick={() => void sEkrana()}>
                QR с экрана
              </Knopka>
            )}
            {otmena}
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <Pole
              testId="adres-podpiski"
              aria-label="адрес подписки"
              znachenie={adres}
              naVvod={zadatAdres}
              placeholder="https://"
              aktiven={aktiven}
              className="flex-1"
            />
            <Knopka rang="glavnaya" testId="sohranit-podpisku" aktiven={aktiven && adres.trim() !== ""} onClick={otpravitAdres}>
              Сохранить
            </Knopka>
            {otmena}
          </div>
        )}
        {ishodVvoda && chto === "server" && (
          <p className="text-fg-secondary text-xs" data-testid="ishod-vvoda">{ishodVvoda}</p>
        )}
        {otkazQr && chto === "server" && (
          <Neudacha
            testId="otkaz-qr"
            zagolovok="QR с экрана не прочитался"
            tekst={otkazQr}
            deystvie={() => void sEkrana()}
            podpisDeystviya="Повторить"
          />
        )}
        <p className="text-fg-muted text-xs">
          {chto === "server"
            ? "одна ссылка это один сервер; подписка обновляет список сама"
            : spisok?.podpiska_zadana
              ? "новый адрес заменит текущую подписку; адрес хранится в службе и на экране не показывается"
              : "список серверов будет обновляться сам раз в 12 часов; адрес хранится в службе и на экране не показывается"}
        </p>
      </div>
    </Karta>
  );

  return (
    <Kolonka aria-label="Серверы">
      <Shapka zagolovok="Серверы" svodka={<span data-testid="svodka">{svodka}</span>}>
        {!pervyyZapusk && (
          <Knopka rang="glavnaya" testId="dobavit" aktiven={aktiven && !dobavlyayu} onClick={() => zadatDobavlyayu(true)}>
            <Plyus />Добавить
          </Knopka>
        )}
      </Shapka>

      {spisok === null && spisokOtkaz && (
        // The dead end this screen used to be: no list, no button, no form.
        // The reason is named, the way back is one click, and adding a server
        // by link still works without any list at all.
        <Neudacha
          testId="otkaz-spiska"
          kod={spisokOtkaz.kod}
          tekst={spisokOtkaz.tekst}
          deystvie={obnovitSpisok}
          podpisDeystviya="Повторить"
        />
      )}

      {spisok === null && !spisokOtkaz && (
        <p className="text-fg-muted text-sm" data-testid="zagruzka">список серверов загружается</p>
      )}

      {spisok !== null && vybranPropal && (
        <div
          data-testid="vybran-propal"
          className="border-warn/35 bg-surface flex items-center gap-3 rounded-lg border px-4 py-3 text-sm"
        >
          <div className="flex flex-1 flex-col gap-0.5">
            <span className="text-foreground">выбранного сервера больше нет в подписке</span>
            <span className="text-fg-muted text-xs">работает первый по списку; выбери другой или обнови подписку</span>
          </div>
          <Teg ton="preduprezhdenie">нет в подписке</Teg>
        </div>
      )}

      {!pervyyZapusk && dobavlyayu && forma}

      {spisok !== null && spisok.podpiska_zadana && (
        <Razdel nazvanie="подписка">
          <Karta testId="podpiska">
            <Ryad
              nazvanie={spisok.podpiska_uzel || "подписка задана"}
              poyasnenie={[
                vozrast(spisok.podpiska_obnovlena) ? `обновлена ${vozrast(spisok.podpiska_obnovlena)}` : "ещё не обновлялась",
                izPodpiski ? `${izPodpiski} ${sklon(izPodpiski)}` : undefined,
                "проверка раз в 12 часов",
              ].filter(Boolean).join(" · ")}
              aktiven={aktiven}
            >
              <Knopka rang="vtoraya" testId="obnovit-podpisku" aktiven={aktiven} onClick={() => naKomandu("refreshSubscription", {})}>
                <Obnovit />Обновить
              </Knopka>
            </Ryad>
          </Karta>

          {/* Строки, которые панель прислала, а клиент не понял. Без этой
              карточки они исчезают молча, и человек видит просто меньше
              серверов, чем ожидал. Причина приходит от службы уже без самой
              ссылки: в ней uuid и ключи. */}
          {otkazyPodpiski.length > 0 && (
            <Karta testId="otkazy-podpiski">
              <Ryad
                nazvanie={`${otkazyPodpiski.length} ${slovoPosleChisla(otkazyPodpiski.length, "строка не разобрана", "строки не разобраны", "строк не разобрано")}`}
                poyasnenie="остальные серверы подписки работают"
                aktiven={aktiven}
              />
              <ul className="px-4 pb-3 text-xs text-neutral-400">
                {otkazyPodpiski.map((o) => (
                  <li key={o.stroka} className="py-0.5">
                    строка {o.stroka}: {o.prichina}
                  </li>
                ))}
              </ul>
            </Karta>
          )}
        </Razdel>
      )}

      {pervyyZapusk && (
        <Razdel nazvanie="первый сервер" aria-label="Первый сервер">
          <p className="text-fg-secondary text-sm" data-testid="pusto">
            серверов пока нет: вставь ссылку на сервер или адрес подписки
          </p>
          {forma}
        </Razdel>
      )}

      {spisok !== null && servery.length > 0 && (
        <Razdel nazvanie="серверы" aria-label="Список серверов">
          <div className="flex items-center gap-2">
            <Pole
              tip="search"
              testId="poisk"
              aria-label="поиск"
              znachenie={poisk}
              naVvod={zadatPoisk}
              placeholder="поиск по имени или адресу"
              className="w-full"
            />
            {/* Работает и при опущенном туннеле: tcping туннеля не требует, а
                выбирать сервер человеку надо как раз до подключения. */}
            <Knopka
              rang="vtoraya"
              testId="zamerit-zaderzhki"
              aktiven={aktiven}
              onClick={() => naKomandu("measureDelays", {})}
            >
              проверить
            </Knopka>
          </div>
          <div
            role="listbox"
            aria-label="серверы"
            className="bg-surface border-border max-h-[60vh] overflow-y-auto rounded-lg border"
          >
            {vidimye.length === 0 && (
              <p className="text-fg-muted px-4 py-3 text-sm">ничего не найдено</p>
            )}
            {vidimye.map((s) => {
              const on = s.id === vybran;
              const neset = s.id === status.nesushchiy_id;
              return (
                <div
                  key={s.id}
                  role="option"
                  aria-selected={on}
                  tabIndex={0}
                  data-testid={`server-${s.id}`}
                  onClick={() => aktiven && naKomandu("setServer", { id: s.id })}
                  onKeyDown={(e) => { if (e.key === "Enter" && aktiven) naKomandu("setServer", { id: s.id }); }}
                  className={
                    "border-border hover:bg-fill-subtle group flex h-11 cursor-pointer items-center gap-3 border-t px-3.5 text-[13px] first:border-t-0 " +
                    (on ? "bg-fill-subtle shadow-[inset_2px_0_0_var(--color-accent-ink)]" : "")
                  }
                >
                  <div className="flex min-w-0 flex-1 items-baseline gap-2.5">
                    <span
                      data-testid="imya-servera"
                      className={on ? "text-accent-ink min-w-0 truncate text-sm font-medium" : "text-foreground min-w-0 truncate text-sm font-medium"}
                    >
                      {s.imya}
                    </span>
                    <span className="text-fg-muted truncate text-xs">
                      {s.host}:{s.port} · {TRANSPORT[s.transport] ?? s.transport}
                      {!s.iz_podpiski && " · добавлен вручную"}
                    </span>
                  </div>
                  <Zaderzhka zamer={poZaderzhkam.get(s.id)} />
                  {s.s_pinom && <Teg ton="akcent">пин сертификата</Teg>}
                  {s.nebezopasnyy_ignorirovan && <Teg ton="preduprezhdenie">проверка сертификата включена принудительно</Teg>}
                  {neset && <Teg ton="akcent">активен</Teg>}
                  {udalyayu === s.id ? (
                    <Knopka
                      rang="opasnaya"
                      testId={`udalit-${s.id}`}
                      aktiven={aktiven}
                      onClick={(e) => { e.stopPropagation(); naKomandu("removeServer", { id: s.id }); zadatUdalyayu(null); }}
                    >
                      удалить?
                    </Knopka>
                  ) : (
                    <Knopka
                      rang="tekst"
                      testId={`udalit-${s.id}`}
                      aria-label={`удалить ${s.imya}`}
                      aktiven={aktiven}
                      className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                      onClick={(e) => { e.stopPropagation(); zadatUdalyayu(s.id); }}
                    >
                      <IkonkaKorzina />
                    </Knopka>
                  )}
                </div>
              );
            })}
          </div>
        </Razdel>
      )}
    </Kolonka>
  );
}

/** Две задержки одной строкой. Отсутствие числа это ТЕКСТ отказа, не ноль.
 *
 *  Ноль на экране читается как «мгновенно» и ставит узел первым по задержке,
 *  то есть ровно наверх списка, что противоположно правде о мёртвом узле. */
function Zaderzhka({ zamer }: { zamer?: ZamerZaderzhki }) {
  if (!zamer) return null;
  const uzel =
    typeof zamer.tcping_ms === "number" ? `узел ${zamer.tcping_ms} мс` : zamer.tcping_otkaz || "узел не измерен";
  const tunnel =
    typeof zamer.realping_ms === "number"
      ? `туннель ${zamer.realping_ms} мс`
      : zamer.realping_otkaz || "туннель не измерен";
  return (
    <span data-testid={`zaderzhka-${zamer.id}`} className="text-fg-muted shrink-0 text-xs">
      {uzel} · {tunnel}
    </span>
  );
}

function imyaPo(servery: Server[], id: string): string {
  return servery.find((s) => s.id === id)?.imya ?? id;
}

function sklon(n: number): string {
  const d = n % 10;
  const s = n % 100;
  if (s >= 11 && s <= 14) return "серверов";
  if (d === 1) return "сервер";
  if (d >= 2 && d <= 4) return "сервера";
  return "серверов";
}

// Three 16px line icons, inline: no icon library for three glyphs.
function Plyus() {
  return <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round"><path d="M12 5v14M5 12h14" /></svg>;
}
function Obnovit() {
  return <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M21 12a9 9 0 1 1-3-6.7M21 3v6h-6" /></svg>;
}
