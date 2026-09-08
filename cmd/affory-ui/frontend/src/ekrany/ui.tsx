import type { ButtonHTMLAttributes, ReactNode } from "react";
import { tekstOtkaza } from "./otkazy";

// The layout grammar of the whole window, accepted on the mockup 02.09.2026.
// Screens compose these pieces and never restyle them: one column, cards of
// rows, three ranks of buttons, a native checkbox dressed as a switch. Every
// class string lives here once, so "make the buttons compact" is one edit.

/** The shared column every tab renders into: 32px gutter, 800px cap. */
export function Kolonka({ children, "aria-label": podpis }: { children: ReactNode; "aria-label"?: string }) {
  return (
    <section aria-label={podpis} className="mx-auto flex w-full max-w-[980px] flex-col gap-6 px-8 pt-6 pb-8">
      {children}
    </section>
  );
}

/** Tab header: title and one-line summary on the left, the primary action
 *  on the right, in the same place on every tab. */
export function Shapka({ zagolovok, svodka, children }: { zagolovok: string; svodka?: ReactNode; children?: ReactNode }) {
  return (
    <header className="flex min-h-9 items-center justify-between gap-4">
      <div>
        <h2 className="text-foreground text-xl font-semibold leading-tight">{zagolovok}</h2>
        {svodka && <div className="text-fg-muted mt-0.5 text-[13px]">{svodka}</div>}
      </div>
      {children && <div className="flex shrink-0 gap-2">{children}</div>}
    </header>
  );
}

/** Section: small muted label over a card. */
export function Razdel({ nazvanie, children, "aria-label": podpis }: { nazvanie?: string; children: ReactNode; "aria-label"?: string }) {
  return (
    <section aria-label={podpis ?? nazvanie} className="flex flex-col gap-2">
      {nazvanie && <h3 className="text-fg-muted text-xs font-medium tracking-wide">{nazvanie}</h3>}
      {children}
    </section>
  );
}

/** The card: a surface with a hairline border, rows inside. */
export function Karta({ children, testId, bezObrezki = false }: {
  children: ReactNode;
  testId?: string;
  /** Let a popup inside the card hang past its edge. The clipping is there
   *  for the rounded corners of the rows, and a card WITHOUT rows pays for
   *  it with a cut popup: the process picker on the rules tab showed one row
   *  and the rest died at the card's border (owner, 04.09.2026). */
  bezObrezki?: boolean;
}) {
  return (
    <div data-testid={testId} className={"bg-surface border-border rounded-lg border" + (bezObrezki ? "" : " overflow-hidden")}>
      {children}
    </div>
  );
}

/** A settings row: name and explanation on the left, the control on the
 *  right. `aktiven=false` greys the name; the control disables itself. */
export function Ryad({ nazvanie, poyasnenie, podskazka, aktiven = true, lomat = false, children, testId }: {
  nazvanie: ReactNode;
  poyasnenie?: ReactNode;
  /** Подсказка при наведении. Для того, что нужно знать ПОТОМ, а не при
   *  первом чтении экрана: постоянная строка тут только мешала бы. Родной
   *  title, а не свой всплывающий слой: собственного паттерна подсказок в
   *  программе нет, и заводить его ради одной строки не за что. */
  podskazka?: string;
  aktiven?: boolean;
  /** The name is a path or a domain, i.e. one word 200 characters long.
   *  Without break-all such a row simply widens the column and the window
   *  scrolls sideways (Karkas has overflow-auto). */
  lomat?: boolean;
  children?: ReactNode;
  testId?: string;
}) {
  const ton = aktiven ? "text-foreground text-sm font-medium" : "text-fg-muted text-sm font-medium";
  return (
    <div data-testid={testId} title={podskazka} className="border-border flex min-h-[52px] items-center gap-4 border-t px-4 py-2 first:border-t-0">
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <span className={lomat ? `${ton} break-all` : ton}>{nazvanie}</span>
        {poyasnenie && <span className="text-fg-muted break-words text-xs">{poyasnenie}</span>}
      </div>
      {children && <div className="flex shrink-0 items-center gap-2">{children}</div>}
    </div>
  );
}

/** Либо код словаря, либо своя фраза экрана, и одно из двух ОБЯЗАТЕЛЬНО.
 *
 *  Без второй половины экран, у которого отказ не приезжает от службы, вынужден
 *  выдумывать код: ровно так `qr-s-ekrana` и появился в Servery.tsx, а
 *  компонент пережил его молча. Чтение QR с экрана делает окно, кода на проводе
 *  у такого отказа нет и быть не должно, а сказать человеку есть что. */
type NeudachaProps = {
  tekst?: string;
  deystvie?: () => void;
  podpisDeystviya?: string;
  testId?: string;
} & (
  | {
    kod: string;
    /** The tab's own sentence, when the code's wording is about something
     *  else: `secrets-unreadable` says "серверы", and on the rules tab that
     *  would be a lie about which list failed. The code's own text then
     *  moves to the second line, so nothing is lost. */
    zagolovok?: string;
  }
  | { kod?: undefined; zagolovok: string }
);

/** §9.1 refusal drawn INSIDE a tab: what failed, and the one way out of the
 *  tab it broke. Otkaz.tsx draws the window-wide banner for the same code;
 *  this is the local twin for a refusal that belongs to one list, and it
 *  names its own way out, because "Открыть серверы" on the servers tab is
 *  not a way out of anything. */
export function Neudacha({ kod, zagolovok, tekst, deystvie, podpisDeystviya, testId }: NeudachaProps) {
  const poKodu = kod ? tekstOtkaza[kod]?.tekst : undefined;
  // An unknown code still renders (Otkaz.tsx does the same): raw as the title
  // when there is nothing else, and silently when the screen brought its own
  // sentence. A code printed as a second line would be noise, not a reason.
  const prichina = [zagolovok ? poKodu : undefined, tekst].filter(Boolean).join("; ");
  return (
    <section
      role="alert"
      data-testid={testId}
      data-kod={kod}
      className="border-danger/40 bg-surface flex flex-col gap-3 rounded-lg border p-5"
    >
      <p className="text-foreground break-words text-base">{zagolovok ?? poKodu ?? kod}</p>
      {prichina && <p className="text-fg-secondary break-words text-sm">{prichina}</p>}
      {deystvie && podpisDeystviya && (
        <Knopka rang="glavnaya" className="self-start" testId={testId ? `${testId}-deystvie` : undefined} onClick={deystvie}>
          {podpisDeystviya}
        </Knopka>
      )}
    </section>
  );
}

export type RangKnopki = "glavnaya" | "vtoraya" | "opasnaya" | "tekst" | "znachok";

const RANG: Record<RangKnopki, string> = {
  glavnaya: "bg-accent text-foreground hover:brightness-110 px-3",
  vtoraya: "border-border-hover text-foreground hover:bg-fill-subtle border px-3",
  opasnaya: "border-border-hover text-danger hover:bg-fill-subtle border px-3",
  tekst: "text-fg-muted hover:text-foreground px-1.5",
  znachok: "border-border-hover text-fg-muted hover:text-foreground w-8 border px-0",
};

/** Button. 32px tall, 13px text, flex-centred label: the mockup's crooked
 *  captions were inline-block plus padding, and this is the fix. `bolshaya`
 *  is the 40px variant for the one main action of the connection card. */
export function Knopka({ rang, aktiven = true, bolshaya = false, testId, className = "", children, ...rest }: {
  rang: RangKnopki;
  aktiven?: boolean;
  bolshaya?: boolean;
  testId?: string;
} & ButtonHTMLAttributes<HTMLButtonElement>) {
  const razmer = bolshaya ? "h-10 px-5 text-sm" : "h-8 text-[13px]";
  return (
    <button
      type="button"
      data-testid={testId}
      disabled={!aktiven}
      className={`inline-flex shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium leading-none disabled:opacity-40 ${razmer} ${RANG[rang]} ${className}`}
      {...rest}
    >
      {children}
    </button>
  );
}

/** Switch. A native checkbox with role=switch: keyboard, focus and disabled
 *  come from the browser, only the paint is ours. */
export function Tumbler({ testId, vkl, aktiven, naSmenu, podpis }: {
  testId: string;
  vkl: boolean;
  aktiven: boolean;
  naSmenu: (vkl: boolean) => void;
  podpis: string;
}) {
  return (
    <input
      type="checkbox"
      role="switch"
      aria-checked={vkl}
      aria-label={podpis}
      data-testid={testId}
      checked={vkl}
      disabled={!aktiven}
      onChange={(e) => naSmenu(e.target.checked)}
      className="tumbler"
    />
  );
}

export type TonTega = "obychnyy" | "akcent" | "preduprezhdenie" | "opasnost";

const TON: Record<TonTega, string> = {
  obychnyy: "bg-fill text-fg-secondary",
  akcent: "bg-accent-ink/15 text-accent-ink",
  preduprezhdenie: "bg-warn/12 text-warn",
  opasnost: "bg-danger/12 text-danger",
};

/** Small label chip next to a list row: "нет в подписке" and friends. */
export function Teg({ ton = "obychnyy", testId, children }: { ton?: TonTega; testId?: string; children: ReactNode }) {
  return (
    <span data-testid={testId} className={`shrink-0 whitespace-nowrap rounded-md px-1.5 py-0.5 text-[11px] ${TON[ton]}`}>
      {children}
    </span>
  );
}

/** Segmented control: a radio group drawn as one pill. */
export function Segment<T extends string>({ znacheniya, vybrano, naVybor, aktiven = true, "aria-label": podpis }: {
  znacheniya: { z: T; podpis: string }[];
  vybrano: T;
  naVybor: (z: T) => void;
  aktiven?: boolean;
  "aria-label"?: string;
}) {
  return (
    <div role="radiogroup" aria-label={podpis} className="bg-fill-subtle border-border inline-flex rounded-md border p-0.5 text-[13px]">
      {znacheniya.map(({ z, podpis: p }) => {
        const on = z === vybrano;
        return (
          <button
            key={z}
            type="button"
            role="radio"
            aria-checked={on}
            disabled={!aktiven}
            onClick={() => naVybor(z)}
            className={
              on
                ? "bg-fill-hover text-foreground rounded-[5px] px-3 py-1 font-medium"
                : "text-fg-muted hover:text-foreground rounded-[5px] px-3 py-1 disabled:opacity-40"
            }
          >
            {p}
          </button>
        );
      })}
    </div>
  );
}

/** Text field, same height as a button so the two sit in one line. */
export function Pole({ testId, znachenie, naVvod, placeholder, aktiven = true, tip = "text", "aria-label": podpis, className = "" }: {
  testId?: string;
  znachenie: string;
  naVvod: (v: string) => void;
  placeholder?: string;
  aktiven?: boolean;
  tip?: "text" | "search";
  "aria-label"?: string;
  className?: string;
}) {
  return (
    <input
      type={tip}
      data-testid={testId}
      aria-label={podpis}
      value={znachenie}
      disabled={!aktiven}
      placeholder={placeholder}
      spellCheck={false}
      onChange={(e) => naVvod(e.target.value)}
      className={`bg-fill-subtle border-border-hover text-foreground placeholder:text-fg-faint h-8 min-w-0 rounded-md border px-2.5 text-[13px] disabled:opacity-40 ${className}`}
    />
  );
}

// A hyphen, not an em dash: banned everywhere, screens included.
export const PROCHERK = "-";

/** The bin: delete affordance on servers and rules, one drawing for both. */
export function IkonkaKorzina() {
  return <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round"><path d="M4 7h16M10 11v6M14 11v6M6 7l1 13h10l1-13M9 7V4h6v3" /></svg>;
}
