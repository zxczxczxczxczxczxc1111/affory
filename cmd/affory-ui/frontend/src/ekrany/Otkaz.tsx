import { TEKST_SLUZHBY_DOSLOVNO, podpisDeystviya, tekstOtkaza, type Deystvie } from "./otkazy";

// One refusal, one screen. Pure over props: the code names the text and the
// action, App decides what the action does. An unknown code still renders,
// with the code itself as text: a refusal the screen cannot name is worse
// shown raw than swallowed.

export interface OtkazProps {
  kod: string;
  /** The service's own text. Shown under ours, or INSTEAD of ours for codes
   *  in TEKST_SLUZHBY_DOSLOVNO (the panel's message must not be paraphrased). */
  tekst?: string;
  /** Name of the foreign process for pokazat-vinovnika screens. */
  vinovnik?: string;
  naDeystvie: (d: Deystvie) => void;
  /** Extra retry for a refusal the shell can re-issue itself, such as a list
   *  that did not load. Drawn next to the code's own action, or alone. */
  naPovtor?: () => void;
  /** Closes the banner. Present ALWAYS when the shell can dismiss: twelve of
   *  the codes have no action at all, and without a cross they hung on screen
   *  until the next command that happened to succeed. */
  naZakrytie?: () => void;
}

export function Otkaz({ kod, tekst, vinovnik, naDeystvie, naPovtor, naZakrytie }: OtkazProps) {
  const z = tekstOtkaza[kod];
  const deystvie: Deystvie = z?.deystvie ?? "nichego";
  // An unknown code has no wording of ours, so the sender's text IS the
  // message. Printing the bare code as the headline and the reason in small
  // grey under it read as a crash report, not as an answer. The code stays
  // in data-kod for whoever is reading the DOM.
  const doslovno = (TEKST_SLUZHBY_DOSLOVNO.has(kod) || z === undefined) && tekst;
  const osnovnoy = doslovno ? tekst : (z?.tekst ?? kod);
  const podpis = podpisDeystviya[deystvie];

  return (
    <section
      role="alert"
      data-testid="otkaz"
      data-kod={kod}
      className="border-danger/40 bg-surface relative flex flex-col gap-3 rounded-lg border p-5"
    >
      {naZakrytie && (
        <button
          type="button"
          data-testid="otkaz-zakryt"
          aria-label="Закрыть сообщение"
          onClick={naZakrytie}
          className="text-fg-muted hover:text-foreground absolute top-3 right-3 h-6 w-6 text-base leading-none"
        >
          &#x00D7;
        </button>
      )}
      <p className="text-foreground pr-8 text-base" data-testid="otkaz-tekst">{osnovnoy}</p>
      {!doslovno && tekst && (
        <p className="text-fg-secondary text-sm" data-testid="otkaz-prichina">{tekst}</p>
      )}
      {deystvie === "pokazat-vinovnika" && (
        <p className="text-fg-secondary text-sm">
          виновник: <span className="text-foreground font-medium" data-testid="vinovnik">{vinovnik ?? "не определён"}</span>
        </p>
      )}
      {(podpis || naPovtor) && (
        <div className="flex items-center gap-2">
          {podpis && (
            <button
              type="button"
              data-testid="otkaz-deystvie"
              onClick={() => naDeystvie(deystvie)}
              className="bg-accent text-foreground hover:brightness-110 self-start rounded-md px-4 py-2 text-sm font-medium"
            >
              {podpis}
            </button>
          )}
          {naPovtor && (
            <button
              type="button"
              data-testid="otkaz-povtor"
              onClick={naPovtor}
              className="border-border-hover text-foreground hover:bg-fill-subtle self-start rounded-md border px-4 py-2 text-sm font-medium"
            >
              Повторить
            </button>
          )}
        </div>
      )}
    </section>
  );
}
