import { Browser, Call, Clipboard, Events, Window } from "@wailsio/runtime";

/** Window controls. The title bar is ours (frameless window), so minimise and
 *  close are bridge calls too; Karkas asks App, App asks here. */
export function oknoSvernut(): void {
  void Window.Minimise();
}

export function oknoZakryt(): void {
  void Window.Close();
}

// The only file that knows Wails sits underneath. Screens import this and
// nothing from @wailsio/runtime or bindings; granitsa.test.ts enforces it.
// Swapping the shell later means rewriting this file, not every screen.

/** Frame shape mirrors internal/protokol/kadr.go. Kept by hand on purpose:
 *  generated bindings would pin the screens to the shell's type layout. */
export interface Kadr {
  tip: "cmd" | "otvet" | "sobytie";
  id: number;
  imya: string;
  telo?: unknown;
  oshibka?: { kod: string; tekst: string };
}

/** Thrown when the shell itself cannot reach the service: the pipe is closed,
 *  refused or owned by someone else. Distinct from a service-side refusal,
 *  which comes back as a frame with `oshibka`. */
export class KanalNedostupen extends Error {
  constructor(prichina: string) {
    super(prichina);
    this.name = "KanalNedostupen";
  }
}

// Go side: package main, unexported struct `most`, exported method `Zvat`.
// Wails names services by reflect's Type.String(), so "main.most" it is.
const IMYA_ZVAT = "main.most.Zvat";
const SOBYTIE_KANALA = "kanal";
const SOBYTIE_OKNA = "okno";

/** Sends one command and returns the answer frame. The body travels as JSON
 *  text both ways; typing it is the caller's business. */
export async function zvat(imya: string, telo: unknown = {}): Promise<Kadr> {
  let syroe: string;
  try {
    syroe = await Call.ByName(IMYA_ZVAT, imya, JSON.stringify(telo));
  } catch (e) {
    throw new KanalNedostupen(e instanceof Error ? e.message : String(e));
  }
  return JSON.parse(syroe) as Kadr;
}

/** First run: is AfforySvc registered with the service manager at all. */
export async function sluzhbaUstanovlena(): Promise<boolean> {
  return (await Call.ByName("main.most.SluzhbaUstanovlena")) as boolean;
}

/** Launches `affory-svc install` elevated (the one UAC prompt). Resolves as
 *  soon as the prompt is answered; rejects when it is declined or the
 *  service binary is missing next to the program. */
export async function ustanovitSluzhbu(): Promise<void> {
  try {
    await Call.ByName("main.most.UstanovitSluzhbu");
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Uninstall, elevated, with the keys question already answered. The shell
 *  quits right after the launch; nothing to await beyond the UAC answer. */
export async function udalitProgrammu(steretKlyuchi: boolean): Promise<void> {
  try {
    await Call.ByName("main.most.UdalitProgrammu", steretKlyuchi);
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Перезапуск ОКНА с повышением прав. Служба смотрит на токен того, кто пришёл
 *  в канал, поэтому повысить одну команду нельзя: повышается всё окно. Старое
 *  закрывается само, через мгновение после запроса прав. Отказ от запроса это
 *  отклонённое обещание, а не поломка. */
export async function perezapustitSPravami(): Promise<void> {
  try {
    await Call.ByName("main.most.PerezapustitSPravami");
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Native file dialog for an update archive; "" when nothing was chosen. */
export async function vybratArhiv(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.VybratArhiv")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** One running process of this session, for the rules form. */
export interface Zapushchennyy {
  imya: string;
  put: string;
}

/** Processes of the person's session, one per exe, sorted by name. The shell
 *  answers, not the service: session 0 sees svchost, not the games. */
export async function spisokProtsessov(): Promise<Zapushchennyy[]> {
  return ((await Call.ByName("main.most.SpisokProtsessov")) ?? []) as Zapushchennyy[];
}

/** The dialog returns a path, not permission to launch its unsuspecting executable. */
export async function vybratPrilozhenie(): Promise<string> {
  const path: unknown = await Call.ByName("main.most.VybratPrilozhenie");
  if (path == null) return "";
  if (typeof path !== "string") throw new Error("Окно выбора вернуло некорректный путь.");
  return path;
}

/** Subscribes to service events. Returns the unsubscribe function. */
export function naSobytie(obrabotchik: (kadr: Kadr) => void): () => void {
  return Events.On(SOBYTIE_KANALA, (sobytie: { data: unknown }) => {
    // Data arrives as the JSON string the Go side emitted; anything else is
    // a shell bug, and swallowing it would hide exactly that.
    if (typeof sobytie.data !== "string") return;
    obrabotchik(JSON.parse(sobytie.data) as Kadr);
  });
}

/** Видимость окна: Go сообщает «показалось» и «спряталось» отсюда, а не Wails
 *  своими WindowShow и WindowHide. Причина записана в main.go: при ПЕРВОМ
 *  показе родное событие не приходит вовсе, а окно у нас ещё и прячется
 *  крестиком вместо закрытия, чего родные события не различают.
 *
 *  Возвращает функцию отписки. */
export function naVidimostOkna(obrabotchik: (vidno: boolean) => void): () => void {
  return Events.On(SOBYTIE_OKNA, (sobytie: { data: unknown }) => {
    if (typeof sobytie.data !== "boolean") return;
    obrabotchik(sobytie.data);
  });
}

/** Clipboard text for the "из буфера" button; "" means EMPTY and nothing
 *  else. An unreadable clipboard throws: turning it into "" made the screen
 *  say "буфер обмена пуст" about a clipboard it never managed to read.
 *  The text never goes back to the screen: it is judged and sent, or refused. */
export async function tekstBufera(): Promise<string> {
  try {
    return (await Clipboard.Text()) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Screen QR: the shell hides the window, shoots every display and decodes.
 *  The link stays in the shell and goes to addServer from there; the screen
 *  only learns the added server's name, or why nothing was added. */
export async function dobavitSEkrana(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.DobavitSEkrana")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Native "save as" dialog for the profile file; "" means the person closed
 *  it, and anything else that goes wrong throws. */
export async function vybratKudaSohranit(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.VybratKudaSohranit")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Native open dialog for the profile file; "" means the person closed it. */
export async function vybratOtkuda(): Promise<string> {
  try {
    return ((await Call.ByName("main.most.VybratOtkuda")) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Writes what exportProfile answered. The body travels as base64 because a
 *  frame body is JSON; the file on disk is the raw bytes, byte for byte the
 *  same as `affory-cli profile export` writes.
 *
 *  The profile PASSWORD never comes anywhere near this call: it goes to the
 *  service inside the command body and nowhere else. */
export async function sohranitProfil(put: string, profil: string): Promise<void> {
  try {
    await Call.ByName("main.most.SohranitProfil", put, profil);
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** Reads a profile file for importProfile and returns it base64-encoded. */
export async function prochitatProfil(put: string): Promise<string> {
  try {
    return ((await Call.ByName("main.most.ProchitatProfil", put)) as string | null) ?? "";
  } catch (e) {
    throw new Error(e instanceof Error ? e.message : String(e));
  }
}

/** A fixed project URL, because the browser is not a command interpreter. */
export function otkrytGitHub(): Promise<void> {
  return Browser.OpenURL("https://github.com/zxczxczxczxczxczxc1111/affory");
}
