import { useEffect, useMemo, useRef, useState } from "react";
import type { OtkazNaEkrane, OtkazStroki, Server, StatusOtvet } from "../protokol";
import { slovoPosleChisla } from "../chisla";
import { IkonkaKorzina, Karta, Knopka, Kolonka, Neudacha, Pole, Razdel, Ryad, Segment, Shapka, Teg } from "./ui";
import { KnopkaSpravki, SpravkaProtokolov } from "./SpravkaProtokolov";

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

/** A subscription address is an http(s) URL and nothing else. The check lives
 *  here for the same reason as the one above: the clipboard button refuses
 *  junk on the spot instead of sending the service a command it will refuse
 *  with a code the human cannot read. A server link is junk here too. */
export function pohozheNaAdres(t: string): boolean {
  const a = t.trim().toLowerCase();
  return a.startsWith("http://") || a.startsWith("https://");
}

export interface SpisokServerov {
  versii?: Record<string, string>;
  servery: Server[];
  vybran: string;
  podpiska_zadana: boolean;
  /** Host of the subscription URL. The URL itself is a secret of the same
   *  class as a key and never leaves the service. */
  podpiska_uzel: string;
  podpiska_obnovlena?: string;
}

/** Одна подписка в списке экрана.
 *
 *  Адреса здесь нет: он секрет класса ключа и из службы не выезжает. Опознают
 *  подписку по узлу и по имени, которое человек ей дал. */
export interface PodpiskaNaEkrane {
  servery?: Server[];
  id: string;
  uzel: string;
  imya?: string;
  obnovlena?: string;
  aktivnaya: boolean;
  /** Сколько ключей у этой подписки готово. У запасной они приезжают обходом
   *  раз в 12 часов, поэтому переключение мгновенно. */
  serverov?: number;
  /** Причина последней неудачи обхода. Без неё живая запасная и просроченная
   *  выглядят одинаково, и узнать разницу можно только переключившись. */
  otkaz?: string;
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
  versiya?: string;
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
  /** Команды, которые сейчас в полёте. Кнопка над долгой командой рисует
   *  вертушку сама: замер задержек и поход за подпиской занимают секунды, и
   *  неподвижный экран всё это время читается как зависшая программа. */
  zanyatyeKomandy?: Record<string, boolean>;
  /** Строки последней подписки, которые не разобрались. Пустой список и
   *  отсутствие это одно и то же: разговора нет. */
  otkazyPodpiski?: OtkazStroki[];
  /** Reads the clipboard through the shell; absent in tests that do not care. */
  chitatBufer?: () => Promise<string>;
  /** Shell-side screen QR: resolves with the ready outcome line ("добавлен
   *  Германия", "подписка добавлена про запас"), rejects with the reason. The
   *  line is ready because the code may hold either a key or a subscription
   *  address, and only the shell knows which one went through. */
  naQrSEkrana?: () => Promise<string>;
  /** Подписки списком: активная одна, остальные про запас. Пустой список это
   *  «подписок нет», и тогда раздела нет вовсе. */
  podpiski?: PodpiskaNaEkrane[];
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

export function Servery({ status, spisok, spisokOtkaz = null, obnovitSpisok, naKomandu, zanyatyeKomandy = {}, otkazyPodpiski = [], chitatBufer, naQrSEkrana, zaderzhki = [], podpiski = [] }: ServeryProps) {
  const aktiven = status.sostoyanie !== "sluzhba-molchit";
  const zhdyot = (k: string) => zanyatyeKomandy[k] === true;
  const [poisk, zadatPoisk] = useState("");
  // One "Добавить", one question: what is being added. A server link and a
  // subscription address used to live behind two buttons with two verbs
  // (owner's remark 02.09.2026), and that was two meanings for one act.
  const [dobavlyayu, zadatDobavlyayu] = useState(false);
  // Справка о протоколах: список даёт имена, но не даёт выбора между ними.
  const [spravka, zadatSpravku] = useState(false);
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
  // Caret goes where the human is about to type. Without it the focus stayed
  // on the segment button, and its :focus-visible ring hung around the form
  // like a selection nobody made (живой отзыв 13.09.2026).
  const poleSsylki = useRef<HTMLInputElement>(null);
  const poleAdresa = useRef<HTMLInputElement>(null);
  // Two-click delete: the first click turns the icon into a question, the
  // second answers it. One click on a trash icon next to the row you are
  // hovering is how a server disappears by accident.
  const [udalyayu, zadatUdalyayu] = useState<string | null>(null);
  const [udalyayuPodpisku, zadatUdalyayuPodpisku] = useState<string | null>(null);

  const servery = spisok?.servery ?? [];
  const vidimye = useMemo(() => servery.filter((s) => sovpadaet(s, poisk)), [servery, poisk]);
  // По id, а не поиском в списке на каждую строку: серверов бывает под шесть
  // десятков, и вложенный обход превратил бы отрисовку в квадрат.
  const poZaderzhkam = useMemo(() => new Map(zaderzhki.map((z) => [z.id, z])), [zaderzhki]);
  const vybran = spisok?.vybran || status.vybran_id || "";
  const vybranPropal = vybran !== "" && !servery.some((s) => s.id === vybran);
  const avto = status.rezhim_marshruta === "avto";
  const izPodpiski = servery.filter((s) => s.iz_podpiski).length;

  // Служба старее окна не знает команды listSubscriptions, и список приходит
  // пустым. Одна строка из полей listServers это не украшение, а единственное,
  // что тогда вообще можно показать про подписку.
  const stroki: PodpiskaNaEkrane[] = podpiski.length > 0
    ? podpiski
    : spisok?.podpiska_zadana
      ? [{ id: "odna", uzel: spisok.podpiska_uzel, obnovlena: spisok.podpiska_obnovlena, aktivnaya: true }]
      : [];

  // Список серверов полосами, а не сплошным столбцом.
  //
  // Две подписки подряд дают полтора десятка одинаковых строк, и понять, какая
  // из них откуда, нельзя ничем: ключи активной лежат в общем списке, а остатки
  // прежней остаются в нём же до переподключения. Заголовок полосы отвечает на
  // это одним словом, а хвост строки («пропал из подписки», «добавлен вручную»)
  // после него не нужен: он говорил то же самое на каждой строке.
  //
  // Порядок полос задан смыслом, а не алфавитом: подписка, её остатки, ручные.
  // Порядок ВНУТРИ полосы оставлен тот, что прислала служба: его задаёт сама
  // подписка, и своя сортировка меняла бы список под руками на каждом обходе.
  const polosy = useMemo(() => {
    const aktivnaya = stroki.find((p) => p.aktivnaya);
    const imyaPodpiski = aktivnaya?.imya || aktivnaya?.uzel || "";
    return [
      {
        klyuch: "podpiska",
        podpis: imyaPodpiski ? `из подписки ${imyaPodpiski}` : "из подписки",
        servery: vidimye.filter((s) => s.iz_podpiski && !s.uderzhan),
      },
      {
        klyuch: "uderzhannye",
        podpis: "пропали из подписки, работают до переподключения",
        servery: vidimye.filter((s) => s.uderzhan),
      },
      {
        klyuch: "ruchnye",
        podpis: "добавлены вручную",
        servery: vidimye.filter((s) => !s.iz_podpiski && !s.uderzhan),
      },
    ].filter((p) => p.servery.length > 0);
  }, [vidimye, stroki]);

  // The form opens by itself only on the honest empty list (§9.2 first run).
  // A refused list is NOT an empty one, so it gets the header button instead.
  const pervyyZapusk = spisok !== null && servery.length === 0;

  // Открытая форма и смена ветки ставят курсор в поле этой ветки.
  useEffect(() => {
    if (!dobavlyayu && !pervyyZapusk) return;
    (chto === "server" ? poleSsylki : poleAdresa).current?.focus();
  }, [dobavlyayu, pervyyZapusk, chto]);

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
  const adresIzBufera = async () => {
    if (!chitatBufer) return;
    const a = (await chitatBufer()).trim();
    if (!pohozheNaAdres(a)) {
      zadatIshodVvoda(a ? "в буфере не адрес подписки" : "буфер обмена пуст");
      return;
    }
    // Адрес подписки это секрет того же разряда, что и ключ: он уходит в
    // службу и на экран не попадает ни здесь, ни в строке исхода.
    naKomandu("addSubscription", { adres: a });
    zadatIshodVvoda(null);
    zadatAdres("");
    zadatDobavlyayu(false);
  };
  const sEkrana = async () => {
    if (!naQrSEkrana) return;
    zadatOtkazQr(null);
    try {
      // Строка исхода приходит готовой: в коде может лежать и ключ, и адрес
      // подписки, и собрать фразу здесь значило бы гадать, что из двух
      // добавилось.
      const itog = await naQrSEkrana();
      zadatIshodVvoda(itog || null);
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
    // addSubscription, а не setSubscription: вторая подписка ложится ПРО ЗАПАС.
    // Подменять ту, по которой человек сейчас работает, нажатием «добавить»
    // значило бы менять список серверов действием, которое об этом не говорит.
    naKomandu("addSubscription", { adres: a });
    zadatAdres("");
    zadatDobavlyayu(false);
  };
  const zakrytFormu = () => { zadatDobavlyayu(false); zadatSsylku(""); zadatAdres(""); };

  const otmena = !pervyyZapusk && <Knopka rang="tekst" onClick={zakrytFormu}>Отмена</Knopka>;
  // Одна кнопка на обе вкладки: в коде лежит либо ключ, либо адрес подписки, и
  // читается он одинаково. Пока кнопка стояла только у ссылки, QR подписки -
  // а именно им её и выдают - прочитать было нечем.
  const knopkaQr = naQrSEkrana && (
    <Knopka
      rang="vtoraya"
      testId="qr-s-ekrana"
      aktiven={aktiven}
      onClick={() => void sEkrana()}
      title="Окно спрячется, снимет экраны и найдёт на них QR. Годится и ключ, и адрес подписки: что в коде, то и добавится. В окно ссылка не попадает, она уходит прямо в службу."
    >
      QR с экрана
    </Knopka>
  );
  const forma = (
    <Karta testId="forma">
      <div className="flex flex-col gap-3 px-4 py-3">
        <Segment<"server" | "podpiska">
          aria-label="что добавить"
          znacheniya={[{ z: "server", podpis: "сервер по ссылке" }, { z: "podpiska", podpis: "подписка" }]}
          vybrano={chto}
          naVybor={(z) => { zadatChto(z); zadatIshodVvoda(null); zadatOtkazQr(null); }}
        />
        {chto === "server" ? (
          <div className="flex items-center gap-2">
            <Pole
              testId="ssylka"
              priv={poleSsylki}
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
            {knopkaQr}
            {otmena}
          </div>
        ) : (
          <div className="flex items-center gap-2">
            <Pole
              testId="adres-podpiski"
              priv={poleAdresa}
              aria-label="адрес подписки"
              znachenie={adres}
              naVvod={zadatAdres}
              placeholder="https://"
              aktiven={aktiven}
              className="flex-1"
            />
            {/* Служба идёт за подпиской по сети прямо в этой команде, и
                ответа ждать секунды. */}
            <Knopka rang="glavnaya" testId="sohranit-podpisku" zhdyot={zhdyot("addSubscription")}
                    aktiven={aktiven && adres.trim() !== ""} onClick={otpravitAdres}>
              {zhdyot("addSubscription") ? "Спрашиваю" : "Сохранить"}
            </Knopka>
            {chitatBufer && (
              <Knopka rang="vtoraya" testId="adres-iz-bufera" aktiven={aktiven} onClick={() => void adresIzBufera()}>
                из буфера
              </Knopka>
            )}
            {knopkaQr}
            {otmena}
          </div>
        )}
        {ishodVvoda && (
          <p className="text-fg-secondary text-xs" data-testid="ishod-vvoda">{ishodVvoda}</p>
        )}
        {otkazQr && (
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
              ? "ляжет про запас, активной останется прежняя; адрес хранится в службе и на экране не показывается"
              : "список серверов будет обновляться сам раз в 12 часов; адрес хранится в службе и на экране не показывается"}
        </p>
      </div>
    </Karta>
  );

  return (
    <Kolonka aria-label="Серверы">
      {spravka && (
        <SpravkaProtokolov
          zakryt={() => zadatSpravku(false)}
          svoi={servery.map((s) => s.transport)}
        />
      )}
      <Shapka
        zagolovok="Серверы"
        svodka={<span data-testid="svodka">{svodka}</span>}
        uZagolovka={<KnopkaSpravki onClick={() => zadatSpravku(true)} />}
      >
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

      {spisok !== null && (spisok.podpiska_zadana || stroki.length > 0) && (
        <Razdel nazvanie={stroki.length > 1 ? "подписки" : "подписка"}>
          <Karta testId="podpiska">
            {stroki.map((p) => (
              <Ryad
                key={p.id}
                testId={`podpiska-${p.id}`}
                nazvanie={
                  <span className="flex items-center gap-2">
                    {p.imya || p.uzel || "подписка задана"}
                  </span>
                }
                poyasnenie={[
                  vozrast(p.obnovlena) ? `обновлена ${vozrast(p.obnovlena)}` : "ещё не обновлялась",
                  `${p.serverov ?? (p.aktivnaya ? izPodpiski : 0)} ${sklon(p.serverov ?? (p.aktivnaya ? izPodpiski : 0))}`,
                  "проверка раз в 12 часов",
                  // Причина отказа последней в строке: она важнее остального,
                  // но и длиннее всего, а перенос строки тут один.
                  p.otkaz || undefined,
                ].filter(Boolean).join(" · ")}
                aktiven={aktiven}
              >
                <Knopka rang="vtoraya" testId={p.aktivnaya ? "obnovit-podpisku" : `obnovit-podpisku-${p.id}`} zhdyot={zhdyot("refreshSubscription")}
                        aktiven={aktiven} onClick={() => naKomandu("refreshSubscription", podpiski.length ? { id: p.id } : {})}>
                  {zhdyot("refreshSubscription") ? "Спрашиваю" : <><Obnovit />Обновить</>}
                </Knopka>
                {/* Удаление в два нажатия, как у сервера: подписка уносит с
                    собой весь список ключей, а отмены у этого действия нет. */}
                {(podpiski.length > 0) && (
                  udalyayuPodpisku === p.id ? (
                    <Knopka
                      rang="opasnaya"
                      testId={`udalit-podpisku-${p.id}`}
                      aktiven={aktiven}
                      onClick={() => { naKomandu("removeSubscription", { id: p.id }); zadatUdalyayuPodpisku(null); }}
                    >
                      удалить?
                    </Knopka>
                  ) : (
                    <Knopka
                      rang="tekst"
                      testId={`udalit-podpisku-${p.id}`}
                      // Кнопка рисуется одной иконкой, и без подписи у неё нет
                      // имени вовсе: в дереве доступности 12.09.2026 её было не
                      // отличить от соседних корзин, и с клавиатуры тоже.
                      aria-label={`удалить подписку ${p.imya || p.uzel}`}
                      aktiven={aktiven}
                      onClick={() => zadatUdalyayuPodpisku(p.id)}
                    >
                      <IkonkaKorzina />
                    </Knopka>
                  )
                )}
              </Ryad>
            ))}
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
              zhdyot={zhdyot("measureDelays")}
              aktiven={aktiven}
              onClick={() => naKomandu("measureDelays", {})}
            >
              {zhdyot("measureDelays") ? "Меряю" : "Проверить"}
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
            {polosy.map((polosa) => (
            <div key={polosa.klyuch} role="group" aria-label={polosa.podpis} data-testid={`polosa-${polosa.klyuch}`}>
              <div className="bg-elevated text-fg-muted border-border border-t px-3.5 py-1.5 text-[11px] font-medium tracking-wide">
                {polosa.podpis}
              </div>
              {polosa.servery.map((s) => {
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
            ))}
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
export function Zaderzhka({ zamer, compact = false }: { zamer?: ZamerZaderzhki; compact?: boolean }) {
  if (!zamer) return null;
  const uzel =
    typeof zamer.tcping_ms === "number" ? `узел ${zamer.tcping_ms} мс` : zamer.tcping_otkaz || "узел не измерен";
  const tunnel =
    typeof zamer.realping_ms === "number"
      ? `VPN ${zamer.realping_ms} мс`
      : zamer.realping_otkaz || "VPN не измерен";
  // «VPN отключён» это не отказ замера, а состояние: в сжатом виде оно
  // печатается как «не измерен», всё прочее как «недоступен».
  const tunnelKratko =
    typeof zamer.realping_ms === "number"
      ? `VPN ${zamer.realping_ms} мс`
      : zamer.realping_otkaz && !zamer.realping_otkaz.includes("VPN отключён")
        ? "VPN недоступен"
        : "VPN не измерен";
  return (
    <span
      data-testid={`zaderzhka-${zamer.id}`}
      className={compact ? "min-w-0 truncate" : "text-fg-muted shrink-0 text-xs"}
      /* Число «VPN» здесь БОЛЬШЕ «Задержки» на главном экране, и это не
         расхождение: там один круг по готовому соединению, здесь весь запрос
         вместе с рукопожатием протокола. Без этой строки одно из двух чисел
         выглядит враньём. */
      title={`${uzel} · ${tunnel}\nузел: дорога до сервера мимо VPN\nVPN: весь запрос через него, вместе с рукопожатием, поэтому больше «Задержки» на главном экране`}
    >
      {/* Сжатый вид это ОДНА строка. Двумя строками он стоял в строке
          сервера высотой 52px рядом с названием, не помещался и наезжал на
          соседние строки списка (16.09.2026). Длинный отказ узла в сжатом
          виде не печатается вовсе: он длиннее строки, а целиком всё лежит в
          подсказке. */}
      {compact ? `${tunnelKratko} · ${typeof zamer.tcping_ms === "number" ? uzel : "узел не измерен"}` : `${uzel} · ${tunnel}`}
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
