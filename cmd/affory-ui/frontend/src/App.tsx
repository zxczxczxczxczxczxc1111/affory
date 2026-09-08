import { useCallback, useEffect, useRef, useState } from "react";
import "./desktop.css";
import { Glavnyy } from "./ekrany/Glavnyy";
import { Skorost } from "./ekrany/Skorost";
import { useSkorost } from "./skorost";
import { Karkas } from "./ekrany/Karkas";
import { Nastroyki, type AdresVyhoda, type ZamerPolosy } from "./ekrany/Nastroyki";
import { Otkaz } from "./ekrany/Otkaz";
import type { Deystvie } from "./ekrany/otkazy";
import { PervyyZapusk, type SostoyanieUstanovki } from "./ekrany/PervyyZapusk";
import { Pravila, type PravilaOtvet } from "./ekrany/Pravila";
import {
  dobavitSEkrana, perezapustitSPravami, prochitatProfil, sohranitProfil, spisokProtsessov,
  tekstBufera, vybratArhiv, vybratKudaSohranit, vybratOtkuda, vybratPrilozhenie, type Zapushchennyy,
} from "./most";
import { Servery, type SpisokServerov, type ZamerZaderzhki } from "./ekrany/Servery";
import type { Vkladka } from "./ekrany/vkladki";
import {
  KanalNedostupen, naSobytie, oknoSvernut, oknoZakryt, sluzhbaUstanovlena,
  udalitProgrammu, ustanovitSluzhbu, zvat, type Kadr,
} from "./most";
import { VERSIYA_PROTOKOLA, type OtkazStroki, type Rezhim, type RezultatProverki, type Statistika, type StatusOtvet } from "./protokol";

// The only place that talks to most.ts. Screens get whole StatusOtvet values
// as props and never touch the bridge, so a shell swap is most.ts plus here.

const MOLCHIT: StatusOtvet = { sostoyanie: "sluzhba-molchit" };

/** State of the pipe as the window knows it. `zhdyom` is "not asked yet" and
 *  is NOT a refusal: before this existed every launch opened on "служба не
 *  отвечает" for the whole first round trip, healthy service included. */
export type Svyaz = "zhdyom" | "est" | "net";

/** How often the window re-asks status. The tray already polls every five
 *  seconds (trey.go); two halves of one program counting differently is how
 *  a green tray and a window saying "служба не отвечает" ended up on screen
 *  at the same time. */
export const PERIOD_OPROSA_MS = 5000;

/** Refusal code for a failure that is the shell's own, not the service's: a
 *  frame that will not parse, a dialog that broke, a clipboard that refused.
 *  It is deliberately absent from otkazy.ts (that table is checked against
 *  spec §9.1 both ways), so Otkaz falls back to showing the text itself. */
export const KOD_OBOLOCHKI = "oshibka-obolochki";

export interface AppProps {
  /** Poll period override. Tests shorten it; nothing else does. */
  periodOprosaMs?: number;
}

/** A refusal worth a screen: from a command answer, from status.oshibka, or
 *  from the shell itself when the pipe is squatted. */
export interface Otkazano {
  kod: string;
  tekst?: string;
  vinovnik?: string;
  /** Command to repeat for `povtorit`. */
  komanda?: string;
  /** Tab the refusal was raised on. A refusal with no tab belongs to the
   *  window as a whole (a list that would not load, a dead pipe) and is
   *  drawn everywhere; one raised by a button belongs where the button is. */
  vkladka?: Vkladka;
}

/** Identity of a refusal for the "closed stays closed" rule: the same code
 *  with the same text is the same standing refusal, a different one is news. */
function klyuchOtkaza(o: { kod: string; tekst?: string } | null): string | null {
  return o === null ? null : o.kod + " | " + (o.tekst ?? "");
}

// kanal.Podklyuchitsya reports a squatted pipe as "pipe-squatted: владелец
// канала <name>". The name is diagnostics for a person, not a trust decision.
function otkazIzKanala(soobshchenie: string): Otkazano | null {
  if (!soobshchenie.includes("pipe-squatted")) return null;
  const m = /владелец канала (.+)$/.exec(soobshchenie);
  return { kod: "pipe-squatted", vinovnik: m?.[1]?.trim() };
}

function statusIz(kadr: Kadr): StatusOtvet | null {
  const t = kadr.telo;
  if (t && typeof t === "object" && "sostoyanie" in t) return t as StatusOtvet;
  // setRouteMode answers {status, trebuet_podyoma}: the status is nested.
  if (t && typeof t === "object" && "status" in t) return statusIz({ ...kadr, telo: (t as { status: unknown }).status });
  return null;
}

// otkazyIz достаёт список непонятых строк из тела ответа подписки.
//
// Проверяется КАЖДОЕ поле, а не только наличие массива: тело приезжает с
// провода, и служба постарше может прислать другую форму. Запись без номера
// строки или без причины бесполезна на экране, поэтому она отбрасывается здесь,
// а не рисуется пустой строкой.
function otkazyIz(telo: unknown): OtkazStroki[] {
  if (!telo || typeof telo !== "object") return [];
  const syrye = (telo as { otkazy?: unknown }).otkazy;
  if (!Array.isArray(syrye)) return [];
  return syrye.flatMap((o) => {
    if (!o || typeof o !== "object") return [];
    const { stroka, prichina } = o as { stroka?: unknown; prichina?: unknown };
    if (typeof stroka !== "number" || typeof prichina !== "string" || prichina === "") return [];
    return [{ stroka, prichina }];
  });
}

// zameryIz достаёт замеры задержки. Как и otkazyIz, проверяет КАЖДОЕ поле.
//
// Число и отсутствие числа здесь разные вещи, и путать их нельзя: `null`
// означает «не измерено», а ноль означал бы «мгновенно» и поставил бы узел
// первым по задержке. Поэтому нечисло не превращается в ноль, а остаётся
// отсутствием, и рядом с ним живёт текст отказа.
function zameryIz(telo: unknown): ZamerZaderzhki[] {
  if (!telo || typeof telo !== "object") return [];
  const syrye = (telo as { zamery?: unknown }).zamery;
  if (!Array.isArray(syrye)) return [];
  return syrye.flatMap((z) => {
    if (!z || typeof z !== "object") return [];
    const o = z as Record<string, unknown>;
    if (typeof o.id !== "string" || o.id === "") return [];
    const chislo = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : null);
    const tekst = (v: unknown) => (typeof v === "string" ? v : "");
    return [{
      id: o.id,
      tcping_ms: chislo(o.tcping_ms),
      tcping_otkaz: tekst(o.tcping_otkaz),
      realping_ms: chislo(o.realping_ms),
      realping_otkaz: tekst(o.realping_otkaz),
    }];
  });
}

// polosaIz достаёт замер полосы. Нечисло остаётся отсутствием, а не нулём.
//
// Ноль тут не безобидное умолчание: объявленный ноль это Brutal с нулевой
// оценкой канала, то есть протокол, которому сказали «канала нет». Поэтому
// «не измерено» доезжает до экрана как отсутствие числа, а не как ноль.
function polosaIz(telo: unknown, vremya: string): ZamerPolosy | null {
  if (!telo || typeof telo !== "object") return null;
  const o = telo as Record<string, unknown>;
  const chislo = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : null);
  const celoe = (v: unknown) => (typeof v === "number" && Number.isInteger(v) && v > 0 ? v : null);
  return {
    mbitVniz: chislo(o.mbit_vniz),
    sovetVniz: celoe(o.sovet_vniz),
    mbitVverh: chislo(o.mbit_vverh),
    sovetVverh: celoe(o.sovet_vverh),
    otkazVverh: typeof o.otkaz_vverh === "string" ? o.otkaz_vverh : "",
    cherezTunnel: o.cherez_tunnel === true,
    vremya,
  };
}

// Commands after which the server list on screen is stale.
const MENYAYUT_SPISOK = new Set(["addServer", "removeServer", "setSubscription", "refreshSubscription", "setServer", "setRouteMode", "connect"]);
// Same for the rules list.
const MENYAYUT_PRAVILA = new Set(["setRules"]);

export function App({ periodOprosaMs = PERIOD_OPROSA_MS }: AppProps = {}) {
  const [busyCommand, setBusyCommand] = useState<string | null>(null);
  // null means "no answer yet". The screens still want a whole StatusOtvet,
  // so `naEkrane` below substitutes MOLCHIT once we know the pipe is dead.
  const [status, zadatStatus] = useState<StatusOtvet | null>(null);
  const [svyaz, zadatSvyaz] = useState<Svyaz>("zhdyom");
  const speed = useSkorost(zvat, svyaz === "est");
  // null until the service actually answers subscribeStats with numbers.
  // It answers not-implemented until wave 6, so this stays null on purpose.
  const [statistika, zadatStat] = useState<Statistika | null>(null);
  const [adresVyhoda, zadatAdresVyhoda] = useState<AdresVyhoda | null>(null);
  const [zamerPolosy, zadatZamerPolosy] = useState<ZamerPolosy | null>(null);
  // Last checkLeaks answer; lives with the settings tab, null until asked.
  const [proverka, zadatProverku] = useState<RezultatProverki | null>(null);
  // Why the last check brought nothing. Without it a failed checkLeaks left the
  // PREVIOUS report on screen, and it read as the current one.
  const [proverkaOtkaz, zadatProverkaOtkaz] = useState<Otkazano | null>(null);

  // Строки последней подписки, которые клиент не понял. Живут отдельно от
  // списка серверов намеренно: список это то, что получилось, а это то, что не
  // получилось, и второе исчезало молча.
  const [otkazyPodpiski, zadatOtkazyPodpiski] = useState<OtkazStroki[]>([]);
  const [zaderzhki, zadatZaderzhki] = useState<ZamerZaderzhki[]>([]);

  const [otkaz, zadatOtkaz] = useState<Otkazano | null>(null);
  // What the person closed by hand. status.oshibka is a STANDING refusal and
  // used to survive every zadatOtkaz(null): while the service held one, the
  // banner could not be dismissed at all.
  const [zakryto, zadatZakryto] = useState<string | null>(null);
  // Three states, not two: waiting, ready, refused. `null` in the data used
  // to mean both "not asked yet" and "asked and refused", and the screen drew
  // the second as the first for ever.
  const [spisok, zadatSpisok] = useState<SpisokServerov | null>(null);
  const [spisokOtkaz, zadatSpisokOtkaz] = useState<Otkazano | null>(null);
  // Deferred commands with wave numbers, from hello. The screens over those
  // commands take the wave from here, never from their own strings.
  const [otlozheno, zadatOtlozheno] = useState<Record<string, number> | null>(null);
  const [pravila, zadatPravila] = useState<PravilaOtvet | null>(null);
  const [pravilaOtkaz, zadatPravilaOtkaz] = useState<Otkazano | null>(null);
  // setRules answered trebuet_podyoma: the change waits for the next connect,
  // and the rules tab says so until then.
  const [pravilaZhdut, zadatPravilaZhdut] = useState(false);
  // Running processes for the rules form; null until the shell answers.
  const [zapushchennye, zadatZapushchennye] = useState<Zapushchennyy[] | null>(null);
  const [vkladka, zadatVkladku] = useState<Vkladka>("podklyuchenie");
  // Last profile outcome in the person's words. A press with no visible
  // result reads as "the button does nothing" (owner, 03.09.2026).
  const [itogProfilya, zadatItogProfilya] = useState<string | null>(null);
  // Read inside vypolnit without making it a dependency: rebuilding vypolnit
  // on every tab switch would restart the mount effect and re-subscribe.
  const vkladkaSeychas = useRef<Vkladka>(vkladka);
  useEffect(() => {
    vkladkaSeychas.current = vkladka;
  }, [vkladka]);
  // null: the service exists (answering or not). Otherwise the first-run
  // screen owns the whole window until status answers.
  const [ustanovka, zadatUstanovku] = useState<{ sostoyanie: SostoyanieUstanovki; prichina?: string } | null>(null);

  // What the banner is showing right now, for the dismiss below: naDeystvie
  // is built long before `pokazat` is computed, and a stale copy of the
  // refusal is exactly what "closed stays closed" cannot afford.
  const pokazannoe = useRef<Otkazano | null>(null);

  /** Real dismissal: the cross and every acknowledgement button. Clearing
   *  `otkaz` alone did nothing while the service held a standing refusal in
   *  status.oshibka, and the button read as broken. */
  const ubratOtkaz = useCallback(() => {
    zadatZakryto(klyuchOtkaza(pokazannoe.current));
    zadatOtkaz(null);
    zadatSpisokOtkaz(null);
    zadatPravilaOtkaz(null);
  }, []);

  // Nothing is swallowed. A dead pipe is the status path's story and gets no
  // banner of its own; everything else reaches the person as its own text,
  // because a shell failure with no §9.1 code is still a failure.
  const zhaloba = useCallback((chto: string, e: unknown) => {
    if (e instanceof KanalNedostupen) {
      zadatSvyaz("net");
      return;
    }
    zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: `${chto}: ${e instanceof Error ? e.message : String(e)}` });
  }, []);

  const obnovitProtsessy = useCallback(() => {
    void spisokProtsessov().then(zadatZapushchennye).catch((e: unknown) => {
      zadatZapushchennye(null);
      zhaloba("spisokProtsessov", e);
    });
  }, [zhaloba]);

  const obnovitSpisok = useCallback(async () => {
    try {
      const kadr = await zvat("listServers");
      if (kadr.oshibka) {
        // secrets-unreadable is a real answer here (komandy_serverov.go) and
        // it has its own screen in §9.1. Dropping it left an empty list.
        zadatSpisokOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        return;
      }
      zadatSpisokOtkaz(null);
      if (kadr.telo && typeof kadr.telo === "object") zadatSpisok(kadr.telo as SpisokServerov);
    } catch (e: unknown) {
      zhaloba("listServers", e);
    }
  }, [zhaloba]);

  const zagruzitOtlozhennye = useCallback(async () => {
    try {
      const kadr = await zvat("hello", { protocol: VERSIYA_PROTOKOLA });
      if (kadr.oshibka) {
        // protocol-mismatch: the two halves are different versions, §9.1.
        zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        return;
      }
      const t = kadr.telo as { pozzhe?: Record<string, number> } | undefined;
      zadatOtlozheno(t?.pozzhe ?? {});
    } catch (e: unknown) {
      // A failed hello left `otlozheno` null for ever: Rules stayed on
      // "служба ещё не ответила" and every Settings control stayed grey.
      zhaloba("hello", e);
    }
  }, [zhaloba]);

  const obnovitPravila = useCallback(async () => {
    try {
      const kadr = await zvat("listRules");
      if (kadr.oshibka) {
        zadatPravilaOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        return;
      }
      zadatPravilaOtkaz(null);
      if (kadr.telo && typeof kadr.telo === "object") zadatPravila(kadr.telo as PravilaOtvet);
    } catch (e: unknown) {
      zhaloba("listRules", e);
    }
  }, [zhaloba]);

  // Runs a command and folds its answer into state. Every command goes
  // through here so a refusal frame becomes a refusal screen in one place.
  const vypolnit = useCallback(async (komanda: string, telo: unknown = {}) => {
    const blocksControls = ["connect", "disconnect", "setRules", "setRouteMode", "setServer"].includes(komanda);
    if (blocksControls) setBusyCommand(komanda);
    try {
      const kadr = await zvat(komanda, telo);
      // A frame came back, refusal or not: the pipe is alive.
      zadatSvyaz("est");
      if (kadr.oshibka) {
        zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst, komanda, vkladka: vkladkaSeychas.current });
      } else {
        zadatOtkaz(null);
      }
      const s = statusIz(kadr);
      if (s) zadatStatus(s);
      zadatUstanovku(null);
      // Even a refused addServer may have changed the list (a subscription
      // that saved but did not download), so reload after both outcomes.
      if (MENYAYUT_SPISOK.has(komanda)) void obnovitSpisok();
      // Обе команды подписки отдают строки, которые не разобрались. Пустой
      // ответ ОБНУЛЯЕТ прежний список: старые отказы после удачного обновления
      // это разговор о позапрошлой подписке.
      if (komanda === "setSubscription" || komanda === "refreshSubscription") {
        zadatOtkazyPodpiski(otkazyIz(kadr.telo));
      }
      // Замеры задержки. Ответ ЗАМЕЩАЕТ прежний целиком: показывать замер
      // позапрошлого прохода рядом со свежим значит врать про оба.
      if (komanda === "measureDelays" && !kadr.oshibka) {
        zadatZaderzhki(zameryIz(kadr.telo));
      }
      // Замер полосы. Отказ УНОСИТ прошлые числа: они были правдой про прошлый
      // проход, а на экране читались бы как ответ на нажатие, которое только
      // что не удалось. Ровно этим болела проверка утечек до 03.09.2026.
      if (komanda === "measureBandwidth") {
        if (kadr.oshibka) {
          zadatZamerPolosy(null);
        } else {
          const vremya = new Date().toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
          zadatZamerPolosy(polosaIz(kadr.telo, vremya));
        }
      }
      if (MENYAYUT_PRAVILA.has(komanda)) {
        await obnovitPravila();
        const t = kadr.telo;
        if (!kadr.oshibka && t && typeof t === "object") zadatPravilaZhdut((t as { trebuet_podyoma?: unknown }).trebuet_podyoma === true);
      }
      // A fresh connect reads the rules from disk: nothing waits any more.
      if (komanda === "connect") zadatPravilaZhdut(false);
      if (komanda === "checkLeaks") {
        if (kadr.oshibka) {
          // The old report is not an answer about now: it goes out with the
          // refusal that replaced it.
          zadatProverku(null);
          zadatProverkaOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst });
        } else {
          const r = kadr.telo as RezultatProverki;
          zadatProverku(r);
          zadatProverkaOtkaz(null);
          if (r.adres_vyhoda) zadatStat((s) => ({ ...(s ?? {}), adres_vyhoda: r.adres_vyhoda as string }));
        }
      }
      if (!kadr.oshibka && komanda === "checkExitIp") {
        const t = kadr.telo as { adres?: string; cherez?: string };
        // Direct measurement is not the tunnel's exit: keep it off the main object.
        if (t.adres && t.cherez === "tunnel") zadatStat((s) => ({ ...(s ?? {}), adres_vyhoda: t.adres as string }));
        // The settings row shows the answer itself: a press with no visible
        // result read as "the button does nothing" (owner, 03.09.2026).
        if (t.adres) {
          const vremya = new Date().toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
          zadatAdresVyhoda({ adres: t.adres, cherez: t.cherez === "tunnel" ? "tunnel" : "napryamuyu", vremya });
        }
      }
      return !kadr.oshibka;
    } catch (e: unknown) {
      // A closed pipe is a state, not an exception to hide. Anything else is
      // a broken frame, and rethrowing it here bought an unhandled rejection
      // and a dead button with no trace on screen.
      if (!(e instanceof KanalNedostupen)) {
        zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: `${komanda}: ${e instanceof Error ? e.message : String(e)}`, komanda, vkladka: vkladkaSeychas.current });
        return;
      }
      zadatSvyaz("net");
      zadatStatus(MOLCHIT);
      const squat = otkazIzKanala(e.message);
      zadatOtkaz(squat);
      // "No service" and "service not answering" are different screens
      // (§9.1: no-admin vs pipe-squatted vs sluzhba-molchit). Ask the SCM.
      if (!squat) {
        const est = await sluzhbaUstanovlena().catch(() => true);
        if (!est) zadatUstanovku((u) => u ?? { sostoyanie: "net-sluzhby" });
      }
      return false;
    } finally {
      if (blocksControls) setBusyCommand(current => current === komanda ? null : current);
    }
  }, [obnovitSpisok, obnovitPravila]);

  const naUstanovku = useCallback(() => {
    zadatUstanovku({ sostoyanie: "ustanavlivaetsya" });
    ustanovitSluzhbu()
      .then(() => {
        // UAC answered, install running. Poll until the service speaks;
        // vypolnit clears `ustanovka` on the first real answer.
        let popytok = 0;
        const t = setInterval(() => {
          popytok += 1;
          zvat("status")
            .then(() => {
              // The service speaks: stop polling and let the normal path
              // fold the answer in and drop the first-run screen.
              clearInterval(t);
              void vypolnit("status");
            })
            .catch(() => {
              if (popytok >= 30) {
                clearInterval(t);
                zadatUstanovku({ sostoyanie: "otkaz", prichina: "служба не ответила за минуту после установки" });
              }
            });
        }, 2000);
      })
      .catch((e: unknown) => {
        zadatUstanovku({ sostoyanie: "otkaz", prichina: e instanceof Error ? e.message : String(e) });
      });
  }, [vypolnit]);

  const oprosit = useCallback(() => vypolnit("status"), [vypolnit]);

  // The repeating poll must NOT go through vypolnit: that one clears the
  // refusal banner on every answered frame, and a banner that dies five
  // seconds after it appears is the same as no banner at all.
  const oprositTiho = useCallback(async () => {
    try {
      const kadr = await zvat("status");
      zadatSvyaz("est");
      const s = statusIz(kadr);
      if (s) zadatStatus(s);
      if (!kadr.oshibka) zadatUstanovku(null);
    } catch (e: unknown) {
      if (!(e instanceof KanalNedostupen)) {
        // A broken frame is not a dead pipe: say so instead of polling on
        // forever against a shell that answers rubbish.
        zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: e instanceof Error ? e.message : String(e) });
        return;
      }
      zadatSvyaz("net");
      zadatStatus(MOLCHIT);
    }
  }, []);

  /** Main action of the `sluzhba-molchit` state: ask again, out loud. Until
   *  03.09.2026 that state had no button at all, so a window that had lost
   *  the service could only be closed and started again. */
  const povtorit = useCallback(() => void vypolnit("status"), [vypolnit]);

  // What each spec §9.1 action does here. Actions with no home yet in this
  // wave (install the service, elevate, update) only clear the screen, and
  // the fallback contract records them as such until 4.6 and 4.11 land.
  const naDeystvie = useCallback((d: Deystvie) => {
    switch (d) {
      case "povtorit":
        void vypolnit(otkaz?.komanda ?? "status");
        break;
      case "perepodklyuchitsya":
        void vypolnit("disconnect").then(() => vypolnit("connect"));
        break;
      case "otkryt-servery":
        zadatVkladku("servery");
        zadatOtkaz(null);
        break;
      case "obnovit-podpisku":
        void vypolnit("refreshSubscription");
        break;
      // Права: окно перезапускается повышенным. Служба смотрит на токен
      // того, кто пришёл в канал, поэтому повысить одну команду нельзя.
      // До 03.09.2026 эта ветка просто гасила баннер, и человек, поставивший
      // программу и перезапустивший её, не мог ни добавить сервер, ни удалить,
      // ни обновить подписку: живой прогон в госте это показал.
      case "zaprosit-prava":
        // Declining UAC used to clear the banner, so refusing the prompt
        // looked exactly like being granted the rights (guest run 03.09.2026).
        void perezapustitSPravami().catch((e: unknown) =>
          zadatOtkaz({
            kod: "admin-required",
            tekst: e instanceof Error ? e.message : String(e),
            vkladka: vkladkaSeychas.current,
          }),
        );
        break;
      case "proverit-set":
      case "pokazat-vinovnika":
      case "nichego":
      case "postavit-sluzhbu":
      case "obnovit-programmu":
        ubratOtkaz();
        break;
    }
  }, [ubratOtkaz, vypolnit, otkaz]);

  // Everything the window asks for at startup, in one place: the reconnect
  // path below repeats exactly this list and nothing else.
  const sprositVsyo = useCallback(() => {
    void obnovitSpisok();
    void zagruzitOtlozhennye();
    // Stats subscription lives with THIS screen: without on/off the service
    // would poll clash_api every second even for a minimised window.
    void zvat("subscribeStats", { vkl: true }).catch((e: unknown) => zhaloba("subscribeStats", e));
    // listRules is NOT here: it is fetched by the effect below, once hello
    // has said the command exists. Asking a deferred command for data would
    // turn its refusal into a banner about a wave that has not landed yet.
  }, [obnovitSpisok, zagruzitOtlozhennye, zhaloba]);

  useEffect(() => {
    void oprosit();
    sprositVsyo();
    const otpisatsya = naSobytie((kadr) => {
      if (kadr.imya === "state") {
        const s = statusIz(kadr);
        if (s) zadatStatus(s);
      } else if (kadr.imya === "stats") {
        // Numbers come only while subscribed; the service omits zaderzhka_ms
        // when the core has not measured yet, and the screen draws a dash.
        zadatStat(kadr.telo as Statistika);
      } else if (kadr.imya === "kanal-zakryt") {
        zadatSvyaz("net");
        zadatStatus(MOLCHIT);
        zadatStat(null);
      }
    });
    return () => {
      otpisatsya();
      void zvat("subscribeStats", { vkl: false }).catch(() => undefined);
    };
  }, [oprosit, sprositVsyo]);

  // The tray recovers from a service restart on its own and the window did
  // not: status was asked once, at mount. Five seconds is the tray's period.
  useEffect(() => {
    const t = window.setInterval(() => void oprositTiho(), periodOprosaMs);
    return () => window.clearInterval(t);
  }, [oprositTiho, periodOprosaMs]);

  // On the way back from a dead pipe everything has to be asked again: the
  // service drops the stats subscription when a client disconnects
  // (komandy.go) and emits `state` only on a CHANGE, so a service that came
  // back in the same state tells a fresh subscriber nothing at all.
  const proshlayaSvyaz = useRef<Svyaz>("zhdyom");
  useEffect(() => {
    const bylo = proshlayaSvyaz.current;
    proshlayaSvyaz.current = svyaz;
    if (bylo !== "net" || svyaz !== "est") return;
    sprositVsyo();
    obnovitProtsessy();
  }, [svyaz, sprositVsyo, obnovitProtsessy]);

  // listRules is fetched only once the service says it exists: asking a
  // deferred command for data would turn its refusal into a banner.
  useEffect(() => {
    if ((vkladka === "pravila" || vkladka === "podklyuchenie") && otlozheno !== null && !("listRules" in otlozheno)) void obnovitPravila();
  }, [vkladka, otlozheno, obnovitPravila]);

  // Refresh after returning from another app; yesterday's snapshot is not clairvoyant.
  useEffect(() => {
    if (vkladka !== "pravila") return;
    obnovitProtsessy();
    window.addEventListener("focus", obnovitProtsessy);
    return () => window.removeEventListener("focus", obnovitProtsessy);
  }, [vkladka, obnovitProtsessy]);

  // The service refreshes the subscription on its own schedule; opening the
  // tab is the cheap moment to catch up with that.
  useEffect(() => {
    if (vkladka === "servery") void obnovitSpisok();
  }, [vkladka, obnovitSpisok]);

  // most.ts now throws instead of turning a broken clipboard into "": the
  // screen still wants a string, so the reason goes to the banner and the
  // empty string means only "the clipboard was empty".
  const chitatBufer = useCallback(async (): Promise<string> => {
    try {
      return await tekstBufera();
    } catch (e: unknown) {
      zhaloba("буфер обмена", e);
      return "";
    }
  }, [zhaloba]);

  /** Export: the path comes from the native dialog, the password from the
   *  person. `exportProfile` needs an administrator, and its refusal is a
   *  banner like any other. */
  const vyvestiProfil = useCallback(async (parol: string) => {
    zadatItogProfilya(null);
    try {
      const put = await vybratKudaSohranit();
      if (!put) return;
      const kadr = await zvat("exportProfile", { parol });
      if (kadr.oshibka) {
        zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst, komanda: "exportProfile", vkladka: "nastroyki" });
        return;
      }
      const telo = kadr.telo as { profil?: string } | undefined;
      if (!telo?.profil) {
        zadatOtkaz({ kod: KOD_OBOLOCHKI, tekst: "служба ответила без профиля", vkladka: "nastroyki" });
        return;
      }
      await sohranitProfil(put, telo.profil);
      zadatOtkaz(null);
      // The path is not a secret, the password is. Naming the file is the
      // only way the person can tell a save from a silent no-op.
      zadatItogProfilya("профиль записан: " + put);
    } catch (e: unknown) {
      zhaloba("exportProfile", e);
    }
  }, [zhaloba]);

  /** Import replaces the whole server list, so the list is re-read after. */
  const vvestiProfil = useCallback(async (parol: string) => {
    zadatItogProfilya(null);
    try {
      const put = await vybratOtkuda();
      if (!put) return;
      const profil = await prochitatProfil(put);
      const kadr = await zvat("importProfile", { parol, profil });
      if (kadr.oshibka) {
        zadatOtkaz({ kod: kadr.oshibka.kod, tekst: kadr.oshibka.tekst, komanda: "importProfile", vkladka: "nastroyki" });
        return;
      }
      zadatOtkaz(null);
      zadatItogProfilya("профиль принят, список серверов заменён");
      void obnovitSpisok();
      void oprosit();
    } catch (e: unknown) {
      zhaloba("importProfile", e);
    }
  }, [obnovitSpisok, oprosit, zhaloba]);

  const naRezhim = useCallback((rezhim: Rezhim) => void vypolnit("setRouteMode", { rezhim }), [vypolnit]);
  const naServery = useCallback(() => zadatVkladku("servery"), []);

  // Screens take a whole StatusOtvet and have no "not asked yet" case: while
  // svyaz is `zhdyom` they are not drawn at all, so this stand-in is only ever
  // read once the pipe answered or died.
  const naEkrane: StatusOtvet = status ?? MOLCHIT;

  const naGlavnoe = useCallback(() => {
    // The silent-service state has one honest action, and it is not
    // "disconnect": there is nothing on the other end to disconnect from.
    if (naEkrane.sostoyanie === "sluzhba-molchit") {
      povtorit();
      return;
    }
    const cmd = naEkrane.sostoyanie === "vyklyuchen" || naEkrane.sostoyanie === "otkaz"
      ? "connect"
      : "disconnect";
    void vypolnit(cmd);
  }, [naEkrane.sostoyanie, povtorit, vypolnit]);

  // status.oshibka is the service's standing refusal (e.g. why the last
  // connect failed); a fresh command answer with oshibka takes precedence.
  // A list that refused sits between the two and carries its own retry: the
  // person must not have to guess which tab failed to load.
  let pokazat: Otkazano | null = otkaz;
  let naPovtor: (() => void) | undefined;
  if (!pokazat && spisokOtkaz) {
    pokazat = spisokOtkaz;
    naPovtor = () => void obnovitSpisok();
  }
  if (!pokazat && pravilaOtkaz) {
    pokazat = pravilaOtkaz;
    naPovtor = () => void obnovitPravila();
  }
  if (!pokazat && naEkrane.oshibka) {
    const izStatusa = { kod: naEkrane.oshibka.kod, tekst: naEkrane.oshibka.tekst };
    if (klyuchOtkaza(izStatusa) !== zakryto) pokazat = izStatusa;
  }
  // A refusal raised by a button stays on that button's tab.
  if (pokazat?.vkladka && pokazat.vkladka !== vkladka) pokazat = null;
  pokazannoe.current = pokazat;



  return (
    <main className="affory-desktop bg-background text-foreground h-screen" data-testid="oboloshka">
      <Karkas vkladka={vkladka} naVkladku={zadatVkladku} naSvernut={oknoSvernut} naZakryt={oknoZakryt} zablokirovany={ustanovka !== null}>
        {/* A refusal is shown on whichever tab is open: a declined UAC on
            Settings must not wait for the human to walk back to Connection. */}
        {pokazat && !ustanovka && (
          <div className="px-8 pt-6">
            <Otkaz
              kod={pokazat.kod}
              tekst={pokazat.tekst}
              vinovnik={pokazat.vinovnik}
              naDeystvie={naDeystvie}
              naPovtor={naPovtor}
              naZakrytie={ubratOtkaz}
            />
          </div>
        )}
        {svyaz === "zhdyom" ? (
          // Neutral, not a refusal: nothing is known yet, and a screen that
          // accuses the service of silence before the first answer is a lie
          // the person cannot tell from a real outage.
          <div className="text-fg-muted flex h-full items-center justify-center text-sm" data-testid="zhdyom">
            ожидание ответа службы
          </div>
        ) : ustanovka ? (
          // §9.2: until the service exists nothing else is active, tabs
          // included in spirit; the tab strip stays so the window can close.
          <PervyyZapusk sostoyanie={ustanovka.sostoyanie} prichina={ustanovka.prichina} naUstanovku={naUstanovku} />
        ) : vkladka === "podklyuchenie" ? (
          <Glavnyy
            skorost={<Skorost {...speed} disabled={busyCommand !== null || (naEkrane.sostoyanie !== "vyklyuchen" && naEkrane.sostoyanie !== "podnyat")} />}
            pravila={pravila}
            zaderzhki={zaderzhki}
            zanyato={busyCommand !== null}
            naPravila={() => zadatVkladku("pravila")}
            naTrafik={r => { if (pravila?.trafik) void vypolnit("setRules", { trafik: { ...pravila.trafik, po_umolchaniyu: r }, bez_ru_spiska: pravila.bez_ru_spiska }); }}
            naVyborServera={id => { void (async () => { if (await vypolnit("setRouteMode", { rezhim: "ruchnoy" })) await vypolnit("connect", { server: id }); })(); }}
            spisokOtkaz={spisokOtkaz}
            status={naEkrane}
            statistika={statistika}
            servery={spisok?.servery}
            spisok={spisok}
            naDeystvie={naGlavnoe}
            naServery={naServery}
            naRezhim={naRezhim}
          />
        ) : vkladka === "servery" ? (
          // The refusal is drawn BY the tab now, not instead of it: the screen
          // names the reason, offers the repeat, and still lets a server be
          // added by link. Hiding the whole tab was a dead end (03.09.2026).
          <Servery
            status={naEkrane}
            spisok={spisok}
            spisokOtkaz={spisokOtkaz}
            obnovitSpisok={() => void obnovitSpisok()}
            naKomandu={(komanda, telo) => void vypolnit(komanda, telo)}
            otkazyPodpiski={otkazyPodpiski}
            zaderzhki={zaderzhki}
            chitatBufer={chitatBufer}
            // The shell adds the server itself, so the list is reloaded here:
            // vypolnit never saw an addServer for it.
            naQrSEkrana={async () => {
              const imya = await dobavitSEkrana();
              void obnovitSpisok();
              return imya;
            }}
          />
        ) : vkladka === "pravila" ? (
          <Pravila
            zanyato={busyCommand !== null}
            status={naEkrane}
            otlozheno={otlozheno}
            pravila={pravila}
            pravilaOtkaz={pravilaOtkaz}
            obnovitPravila={() => void obnovitPravila()}
            zhdutPodyoma={pravilaZhdut && naEkrane.sostoyanie !== "vyklyuchen"}
            zapushchennye={zapushchennye}
            obnovitProtsessy={obnovitProtsessy}
            naVyborPrilozheniya={vybratPrilozhenie}
            naKomandu={(komanda, telo) => void vypolnit(komanda, telo)}
          />
        ) : vkladka === "nastroyki" ? (
          <Nastroyki
            adresVyhoda={adresVyhoda}
            zamerPolosy={zamerPolosy}
            status={naEkrane}
            otlozheno={otlozheno}
            svyaz={svyaz}
            povtorit={() => void sprositVsyo()}
            proverka={proverka}
            proverkaOtkaz={proverkaOtkaz}
            vyvestiProfil={(parol) => void vyvestiProfil(parol)}
            vvestiProfil={(parol) => void vvestiProfil(parol)}
            itogProfilya={itogProfilya}
            naKomandu={(komanda, telo) => void vypolnit(komanda, telo)}
            naObnovlenie={() =>
              // The dialog lives on the Go side; "" means nothing chosen and
              // nothing is sent. The service verifies sha256, not the window.
              vybratArhiv()
                .then((put) => { if (put) void vypolnit("installUpdate", { path: put }); })
                .catch((e: unknown) => zadatOtkaz({ kod: "update-archive-invalid", tekst: e instanceof Error ? e.message : String(e) }))
            }
            naUdalenie={(steret) =>
              // A declined UAC is a refusal like any other: the human sees the
              // system's own text and the program stays where it was.
              udalitProgrammu(steret).catch((e: unknown) =>
                zadatOtkaz({ kod: "admin-required", tekst: e instanceof Error ? e.message : String(e) }),
              )
            }
          />
        ) : null}
      </Karkas>
    </main>
  );
}
