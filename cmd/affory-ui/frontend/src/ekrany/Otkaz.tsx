import {
  KOD_OBOLOCHKI, KOD_SROKA, TEKST_BEZ_KODA, TEKST_OBOLOCHKI, TEKST_SLUZHBY_DOSLOVNO,
  podpisDeystviya, tekstOtkaza, type Deystvie,
} from "./otkazy";
import { useState } from "react";
import { ponyatno } from "../ponyatno";

// One refusal, one screen. Pure over props: the code names the text and the
// action, App decides what the action does. An unknown code still renders,
// with the code itself as text: a refusal the screen cannot name is worse
// shown raw than swallowed.
//
// Своё у экрана одно: строка итога под кнопками журнала. App ставит экрану
// key по отказу, и новый отказ начинается с чистой строки.

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
  /** «Сохранить журнал» (1.10.0, просьба владельца 21.10.2026): журнал
   *  пересылают тому, кто разбирает, и кнопка стоит там, где человек видит
   *  ошибку. Отдаёт строку итога, пустая значит «передумал в диалоге». */
  naSohranitZhurnal?: () => Promise<string>;
  naSkopirovat?: (tekst: string) => Promise<void>;
  /** «Affory 1.10.0, Windows 11 (26200)»: её переписывают в жалобу первой. */
  versiya?: string;
}

export function Otkaz({ kod, tekst, vinovnik, naDeystvie, naPovtor, naZakrytie, naSohranitZhurnal, naSkopirovat, versiya }: OtkazProps) {
  const [itogZhurnala, zadatItogZhurnala] = useState("");
  const [sohranyaetsya, zadatSohranyaetsya] = useState(false);
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
  // Истёкший срок канала это тоже голос оболочки, а не службы, и рисуется так
  // же, как было до того, как у него появился свой код.
  const obolochka = kod === KOD_OBOLOCHKI || kod === KOD_SROKA;
  const nash = obolochka ? TEKST_OBOLOCHKI : z?.tekst;
  // Сообщение панели подписки идёт как есть: оно может быть и по-английски, и
  // в нём бывает код оплаты.
  const panel = TEKST_SLUZHBY_DOSLOVNO.has(kod) && !!tekst;
  const neizvesten = z === undefined && !obolochka && !!tekst;
  // Английский хвост Windows и Go отрезается (01.10.2026), целиком текст
  // остаётся в подсказке у причины, в «Скопировать текст» и в журнале окна
  // (с 1.10.0 App пишет туда каждый показанный отказ).
  const chistyy = tekst ? ponyatno(tekst) : "";
  // Нечем сказать вообще: ни строки по коду, ни текста от отправителя. Тогда
  // общая фраза, но НЕ код: код это имя для нас, а не ответ человеку.
  const osnovnoy = panel ? tekst : neizvesten ? (chistyy || TEKST_BEZ_KODA) : (nash ?? TEKST_BEZ_KODA);
  const prichina = panel || neizvesten ? "" : chistyy;
  const podpis = podpisDeystviya[deystvie];
  // Служба и окно про одно и то же говорят своими словами, и на экране это
  // читалось как две строки об одном: «эта команда только для администратора
  // машины», а под ней «команда доступна только администратору этой машины».
  // Повтор не рисуем, а живой текст (например, отказ от запроса прав) рисуем.
  const povtor = prichina.trim() === (osnovnoy ?? "").trim();
  // Если от причины ничего не осталось, сырой текст висит подсказкой на
  // главной строке, иначе он пропал бы с экрана совсем.
  const skrytyy = tekst && tekst !== osnovnoy && (!prichina || povtor) ? tekst : undefined;

  const sohranitZhurnal = naSohranitZhurnal && (async () => {
    zadatSohranyaetsya(true);
    zadatItogZhurnala("");
    try {
      zadatItogZhurnala(await naSohranitZhurnal());
    } catch (e) {
      zadatItogZhurnala(`журнал не сохранён: ${ponyatno(e instanceof Error ? e.message : String(e)) || "что-то пошло не так"}`);
    } finally {
      zadatSohranyaetsya(false);
    }
  });
  // Копируется всё, что нужно разбирающему: наша строка, текст отправителя
  // целиком, код и версия. Код человеку не показывается, а в жалобе он
  // находит место в журнале быстрее любого пересказа.
  const skopirovat = naSkopirovat && (async () => {
    const stroki = [osnovnoy ?? "", tekst && tekst !== osnovnoy ? tekst : "", `код: ${kod}`, versiya ?? ""];
    try {
      await naSkopirovat(stroki.filter((s) => s !== "").join("\n"));
      zadatItogZhurnala("текст ошибки скопирован");
    } catch (e) {
      zadatItogZhurnala(`текст не скопирован: ${ponyatno(e instanceof Error ? e.message : String(e)) || "что-то пошло не так"}`);
    }
  });

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
      <p className="text-foreground pr-8 text-base font-medium first-letter:uppercase" data-testid="otkaz-tekst" title={skrytyy}>{osnovnoy}</p>
      {prichina && !povtor && (
        <p className="text-fg-secondary text-sm" data-testid="otkaz-prichina" title={tekst !== prichina ? tekst : undefined}>{prichina}</p>
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
      {(sohranitZhurnal || skopirovat || versiya) && (
        <div className="border-border flex flex-col gap-1.5 border-t pt-3" data-testid="otkaz-zhurnal">
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
            {sohranitZhurnal && (
              <button
                type="button"
                data-testid="otkaz-sohranit-zhurnal"
                disabled={sohranyaetsya}
                onClick={() => void sohranitZhurnal()}
                className="text-accent-ink hover:underline text-sm font-medium disabled:opacity-60"
              >
                {sohranyaetsya ? "Сохраняю журнал…" : "Сохранить журнал"}
              </button>
            )}
            {skopirovat && (
              <button
                type="button"
                data-testid="otkaz-skopirovat"
                onClick={() => void skopirovat()}
                className="text-accent-ink hover:underline text-sm font-medium"
              >
                Скопировать текст
              </button>
            )}
            {versiya && <span className="text-fg-muted ml-auto text-xs" data-testid="otkaz-versiya">{versiya}</span>}
          </div>
          {itogZhurnala && (
            <p role="status" className="text-fg-secondary text-sm" data-testid="otkaz-itog-zhurnala">{itogZhurnala}</p>
          )}
        </div>
      )}
    </section>
  );
}
