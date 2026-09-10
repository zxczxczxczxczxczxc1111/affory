// Mirror of internal/protokol/kadr.go, kept by hand. Generated bindings would
// tie the screens to the shell's type layout; this file ties them to the wire.
// When kadr.go changes, this changes with it, and Glavnyy.test.tsx lists the
// seven states verbatim so a drift here fails a test, not a user.

/** protokol.Versiya. The bridge already says hello with it on connect; the
 *  screen repeats hello to learn which commands are deferred (`pozzhe`). */
export const VERSIYA_PROTOKOLA = 1;

export type Sostoyanie =
  | "sluzhba-molchit"
  | "vyklyuchen"
  | "podnimaetsya"
  | "podnyat"
  | "ne-neset"
  | "vosstanavlivaetsya"
  | "otkaz";

export type Rezhim = "avto" | "ruchnoy";

export interface Oshibka {
  kod: string;
  tekst: string;
}

/** A refusal as a SCREEN receives it. Narrower than Oshibka on purpose: a
 *  tab may know the code and nothing else (the list simply did not read),
 *  and §9.1 names the code, so the text is optional here and required on
 *  the wire. */
export interface OtkazNaEkrane {
  kod: string;
  tekst?: string;
}

/** ssylki.OtkazStroki: одна строка подписки, которая не разобралась. Номер
 *  строки и причина, и НИКОГДА сама ссылка: в ней uuid и ключи, а служба уже
 *  вычищает их из текста отказа (razbor.go, bezSsylki). */
export interface OtkazStroki {
  stroka: number;
  prichina: string;
}

/** protokol.StatusOtvet. Everything but `sostoyanie` is optional on the
 *  screen side: an older service omits fields, and the screen must still
 *  render rather than crash on a missing key. */
export interface StatusOtvet {
	trafik_po_umolchaniyu?: "vpn" | "direct";
  sostoyanie: Sostoyanie;
  vybran_id?: string;
  nesushchiy_id?: string;
  /** Name of the carrier, sent next to its id (kadr.go NesushchiyImya). In
   *  auto mode the carrier changes without any command from the human, so
   *  the frame carries the name and the tray already reads it; the screen
   *  used to ignore the field and fall back to the bare id. */
  nesushchiy_imya?: string;
  rezhim_marshruta?: Rezhim;
  kill_switch?: boolean;
  avtozapusk?: boolean;
  /** Second decision, separate from autostart (§9.2 defaults differ). */
  podklyuchat_pri_starte?: boolean;
  zhurnal?: boolean;
  diagnostika?: boolean;
  /** Local mixed proxy port next to the tunnel; absent when it is not up. */
  port_proksi?: number;
  /** Полоса канала в мегабитах, объявленная человеком. Отсутствие означает «не
   *  измерена»: ядро тогда считает полосу само (BBR), и это рабочее состояние,
   *  а не пустое поле. */
  polosa_vverh?: number;
  polosa_vniz?: number;
  versiya_sluzhby?: string;
  podnyat_s?: string;
  /** Release build version; absent on dev builds. */
  versiya_programmy?: string;
  /** A newer version seen on the update server (service checks daily). */
  obnovlenie?: Obnovlenie;
  obnovlenie_provereno?: string;
  oshibka?: Oshibka;
}

/** Live numbers under the main object (§8.3). `null` on the screen means
 *  "no data", and the fallback contract forbids drawing that as zero. */
export interface Statistika {
  adres_vyhoda: string;
  /** Absent until the core has a measurement; never zero for "unknown". */
  zaderzhka_ms?: number;
  otdano?: number;
  prinyato?: number;
}

/** One line of checkLeaks. `ne_vidim` is the honest answer for what the
 *  service cannot observe (browser DoH, packets on the wire). */
export interface PunktProverki {
  imya: string;
  itog: "ok" | "utechka" | "ne_izmereno" | "ne_vidim";
  tekst: string;
}

export interface RezultatProverki {
  punkty: PunktProverki[];
  adres_vyhoda?: string;
}

/** protokol.Server as the service projects it for the screen (dlyaEkrana):
 *  identity, transport, address and the two marks. Keys never travel. */
export interface Server {
  id: string;
  imya: string;
  transport: string;
  host: string;
  port: number;
  iz_podpiski: boolean;
  nebezopasnyy_ignorirovan?: boolean;
  /** hy2 with pinSHA256: the pin stays in the service, only the mark travels. */
  s_pinom?: boolean;
}

export interface Obnovlenie {
  versiya: string;
  razmer: number;
  provereno: string;
}

/** getServerHealth (wave 6.4): the VPS snapshot found next to the subscription. */
export interface Zdorovie {
  vremya: number;
  vozrast_s: number;
  ustarel: boolean;
  trevogi: string[];
  dney_do_konca_sertifikata_maski?: number;
  dney_s_obnovleniya_xray?: number;
  xray_versiya?: string;
  hy2_aktiven?: boolean;
  /** Let's Encrypt on a bare IP lives 6 days; absent when the server runs no hy2. */
  dney_do_konca_sertifikata_hy2?: number;
}
