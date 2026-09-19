import {
  KOD_OBOLOCHKI, TEKST_BEZ_KODA, TEKST_OBOLOCHKI, TEKST_SLUZHBY_DOSLOVNO,
  podpisDeystviya, tekstOtkaza, type Deystvie,
} from "./otkazy";

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
  //
  // Сбой оболочки из этого правила ВЫНУТ, и ровно по той же причине, по
  // которой правило написано. Текст службы человеческий («подписка отдала
  // сообщение вместо серверов»), а текст оболочки технический и часто
  // английский, потому что приходит из Windows. Крупно он и есть тот самый
  // отчёт о сбое, которого правило избегает.
  const nash = kod === KOD_OBOLOCHKI ? TEKST_OBOLOCHKI : z?.tekst;
  const doslovno = (TEKST_SLUZHBY_DOSLOVNO.has(kod)
    || (z === undefined && kod !== KOD_OBOLOCHKI)) && tekst;
  // Нечем сказать вообще: ни строки по коду, ни текста от отправителя. Тогда
  // общая фраза, но НЕ код: код это имя для нас, а не ответ человеку.
  const osnovnoy = doslovno ? tekst : (nash ?? TEKST_BEZ_KODA);
  const podpis = podpisDeystviya[deystvie];
  // Служба и окно про одно и то же говорят своими словами, и на экране это
  // читалось как две строки об одном: «эта команда только для администратора
  // машины», а под ней «команда доступна только администратору этой машины».
  // Повтор не рисуем, а живой текст (например, отказ от запроса прав) рисуем.
  const povtor = (tekst ?? "").trim() === (osnovnoy ?? "").trim();

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
      {/* Прописная первая буква и полужирный не косметика: со строчной буквы
          и обычным начертанием сообщение читалось как системная ошибка,
          заглянувшая из чужой программы (владелец, 19.09.2026). Остальные
          заголовки окна начинаются с прописной, и этот теперь тоже.
          Регистр правится ПОКАЗОМ, а не таблицей §9.1: та сверяется с
          спекой построчно, и трогать в ней написание значило бы чинить
          внешний вид в контракте. */}
      <p className="text-foreground pr-8 text-base font-medium first-letter:uppercase" data-testid="otkaz-tekst">{osnovnoy}</p>
      {!doslovno && tekst && !povtor && (
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
