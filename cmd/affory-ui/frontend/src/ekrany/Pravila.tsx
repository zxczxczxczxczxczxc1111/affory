import { useEffect, useState } from "react";
import type { KatalogServisov, PravilaTrafika } from "../trafik";
import { Marshruty } from "./Marshruty";
import type { OtkazNaEkrane, StatusOtvet } from "../protokol";
import type { Zapushchennyy } from "../most";
import { IkonkaKorzina, Karta, Knopka, Kolonka, Neudacha, Pole, Razdel, Ryad, Segment, Shapka, Tumbler } from "./ui";

// Rules tab (task 4.10, form and deletion in 5.4): exclusions by process and
// by domain, and the connection journal panel (§8.3 puts it here: same
// subject, after the fact).
//
// The journal commands answer not-implemented until wave 6. Fallback contract:
// the screen exists, the wave number is the service's (hello names deferred
// commands), the controls are disabled with the reason beside them, and
// nothing draws a zero it did not measure. When a command lands, it leaves
// `otlozheno` and the control wakes up here without touching the layout.

/** Body of listRules: two lists, normalized process paths and domains. */
export interface PravilaOtvet {
  trafik?: PravilaTrafika;
  katalog?: KatalogServisov;
  trebuet_podyoma?: boolean;
  protsessy: string[];
  domeny: string[];
  /** Человек попросил вести российские сайты через туннель наравне со всем
   *  остальным. Необязательное: служба прошлой версии этого поля не шлёт, и
   *  его отсутствие означает список на месте, а не выключенный список. */
  bez_ru_spiska?: boolean;
}

export interface PravilaProps {
  zanyato?: boolean;
  status: StatusOtvet;
  /** Deferred commands with wave numbers, from hello. `null` until it answers. */
  otlozheno: Record<string, number> | null;
  /** listRules answer; `null` while deferred or not yet fetched. */
  pravila: PravilaOtvet | null;
  /** Why the list is missing, when it is missing because of a refusal. An
   *  empty array and a failed read used to draw the SAME line, and "весь
   *  трафик идёт через туннель" is a lie about rules nobody managed to read. */
  pravilaOtkaz?: OtkazNaEkrane | null;
  /** Repeats listRules. */
  obnovitPravila?: () => void;
  /** setRules answered trebuet_podyoma: the core has no hot reload of rules. */
  zhdutPodyoma?: boolean;
  /** Running processes of this session for the picker; `null` until loaded
   *  (or when the shell could not list them), and then the path field alone. */
  zapushchennye?: Zapushchennyy[] | null;
  obnovitProtsessy?: () => void;
  naVyborPrilozheniya?: () => Promise<string>;
  naKomandu: (komanda: string, telo: unknown) => void;
}

// One choice, not two. The form used to keep its own type, always starting at
// "процесс", so the domains tab opened a process form and the processes tab
// could add a domain that immediately vanished into the other view (owner,
// 04.09.2026). The tab IS the type now.
type Vid = "protsessy" | "domeny";

/** "появится в волне N" for a deferred command, or undefined once it exists. */
export function otlozhenaDo(otlozheno: Record<string, number> | null, komanda: string): string | undefined {
  const volna = otlozheno?.[komanda];
  return volna === undefined ? undefined : `команда ${komanda} появится в волне ${volna}`;
}

export function Pravila(props: PravilaProps) {
  return props.pravila?.trafik ? <Marshruty {...props} trafik={props.pravila.trafik} /> : <PrezhniePravila {...props} />;
}

function PrezhniePravila({ status, otlozheno, pravila, pravilaOtkaz = null, obnovitPravila, zhdutPodyoma = false, zapushchennye = null, naKomandu }: PravilaProps) {
  const [vid, zadatVid] = useState<Vid>("protsessy");
  const [dobavlyayu, zadatDobavlyayu] = useState(false);
  const [put, zadatPut] = useState("");
  const [domen, zadatDomen] = useState("");
  // Two clicks to delete, as on the servers tab. The key is the RULE ITSELF,
  // never its position: the service does not reorder, but it does SHORTEN the
  // list by deduplication (komandy_pravil.go), and an index then points at a
  // neighbour. And any new answer drops the armed question altogether.
  const [udalyayu, zadatUdalyayu] = useState<string | null>(null);
  useEffect(() => { zadatUdalyayu(null); }, [pravila]);

  const molchit = status.sostoyanie === "sluzhba-molchit";
  const zhdyomHello = otlozheno === null;
  const spisokOtlozhen = otlozhenaDo(otlozheno, "listRules");
  const zapisOtlozhena = otlozhenaDo(otlozheno, "setRules");
  const zhurnalOtlozhen = otlozhenaDo(otlozheno, "setJournal");
  const chistkaOtlozhena = otlozhenaDo(otlozheno, "clearJournal");
  const mozhnoPravit = !molchit && !zhdyomHello && !spisokOtlozhen && !zapisOtlozhena && pravila !== null;
  const stroki = pravila ? (vid === "protsessy" ? pravila.protsessy : pravila.domeny) : [];
  // Why editing is off, in the cases that have no marker of their own next
  // to the segment. Silence here is what made the rows look undeletable for
  // no reason while the service was down (03.09.2026).
  const prichinaPravki = molchit
    ? "служба не отвечает"
    : zhdyomHello || spisokOtlozhen
      ? undefined
      : zapisOtlozhena ?? (pravila === null && !pravilaOtkaz ? "список правил не получен" : undefined);

  // setRules takes the WHOLE list: the service replaces, never merges, so a
  // stale screen cannot resurrect a rule someone else deleted.
  const otpravit = (protsessy: string[], domeny: string[]) => naKomandu("setRules", { protsessy, domeny });
  const zakrytFormu = () => { zadatDobavlyayu(false); zadatPut(""); zadatDomen(""); };
  const dobavit = () => {
    if (!pravila) return;
    if (vid === "protsessy") otpravit([...pravila.protsessy, put.trim()], pravila.domeny);
    else otpravit(pravila.protsessy, [...pravila.domeny, domen.trim()]);
    zakrytFormu();
  };
  const udalit = (p: string) => {
    if (!pravila) return;
    if (vid === "protsessy") otpravit(pravila.protsessy.filter((x) => x !== p), pravila.domeny);
    else otpravit(pravila.protsessy, pravila.domeny.filter((x) => x !== p));
    zadatUdalyayu(null);
  };
  const vvedeno = vid === "protsessy" ? put.trim() !== "" : domen.trim() !== "";

  return (
    <Kolonka aria-label="Правила">
      <Shapka zagolovok="Правила" svodka="что идёт мимо туннеля: процессы и домены">
        <Knopka rang="glavnaya" testId="dobavit-pravilo" aktiven={mozhnoPravit && !dobavlyayu} onClick={() => zadatDobavlyayu(true)}>
          <Plyus />Добавить
        </Knopka>
      </Shapka>

      <Razdel aria-label="Исключения">
        <div className="flex items-center gap-3">
          <Segment<Vid>
            aria-label="вид правил"
            znacheniya={[{ z: "protsessy", podpis: "процессы" }, { z: "domeny", podpis: "домены" }]}
            vybrano={vid}
            // Switching the view closes the form: what was typed belonged to
            // the other kind of rule, and carrying it over is how a domain
            // ended up in the process field.
            naVybor={(v) => { zadatVid(v); zadatUdalyayu(null); zakrytFormu(); }}
          />
          {zhdyomHello && <span className="text-fg-muted text-[13px]" data-testid="zagruzka">служба ещё не ответила</span>}
          {spisokOtlozhen && <span className="text-fg-muted text-[13px]" data-testid="pravila-otlozheny">{spisokOtlozhen}</span>}
          {prichinaPravki && <span className="text-fg-muted text-[13px]" data-testid="pravka-nedostupna">{prichinaPravki}</span>}
        </div>

        {pravilaOtkaz && (
          <Neudacha
            testId="otkaz-pravil"
            kod={pravilaOtkaz.kod}
            zagolovok="правила не прочитались"
            tekst={pravilaOtkaz.tekst}
            deystvie={obnovitPravila}
            podpisDeystviya="Повторить"
          />
        )}

        {dobavlyayu && mozhnoPravit && (
          <Karta testId="forma" bezObrezki>
            <div className="flex flex-col gap-3 px-4 py-3">
              {vid === "protsessy" && zapushchennye && zapushchennye.length > 0 && (
                // Picking fills the path field; the field stays editable, so
                // a program that is not running right now can still be typed.
                <VyborZapushchennogo zapushchennye={zapushchennye} naVybor={zadatPut} />
              )}
              <div className="flex items-center gap-2">
                {vid === "protsessy" ? (
                  <Pole
                    testId="put-protsessa"
                    aria-label="путь к файлу процесса"
                    znachenie={put}
                    naVvod={zadatPut}
                    placeholder={"C:\\Program Files\\...\\app.exe"}
                    className="flex-1"
                  />
                ) : (
                  <Pole
                    testId="domen"
                    aria-label="домен"
                    znachenie={domen}
                    naVvod={zadatDomen}
                    placeholder="example.org, поддомены тоже"
                    className="flex-1"
                  />
                )}
                <Knopka rang="glavnaya" testId="dobavit-zapis" aktiven={vvedeno} onClick={dobavit}>
                  Добавить
                </Knopka>
                <Knopka rang="tekst" onClick={zakrytFormu}>отмена</Knopka>
              </div>
            </div>
          </Karta>
        )}

        <Karta>
          {zhdutPodyoma && (
            <Ryad
              testId="zhdut-podyoma"
              nazvanie="изменения применятся со следующего подключения"
              poyasnenie="ядро читает правила при подъёме туннеля"
            />
          )}
          {status.kill_switch && (
            <Ryad
              testId="ves-trafik"
              nazvanie="в режиме «весь трафик» исключения не действуют"
              poyasnenie="выключи его в настройках, чтобы правила снова применялись"
            />
          )}
          {vid === "domeny" && (
            // The honest warning of §«Домены в режиме TUN». The cause is
            // named (browser DoH on 443, invisible to the tunnel's DNS
            // hijack), and so is the action; "ходит по голому адресу" is a
            // verdict, switching DoH off is something a person can do.
            <Ryad
              testId="doh-preduprezhdenie"
              nazvanie="браузер с DoH: правило держится только на рукопожатии TLS"
              poyasnenie="Chrome и Firefox по умолчанию резолвят через DoH на 443, и этот DNS туннель не видит; выключи DoH в браузере, чтобы домен исключался и по DNS"
              aktiven={false}
            />
          )}
          {pravilaOtkaz ? (
            // The refusal is drawn above, outside the card: this row must not
            // repeat it, and it must NOT say the exclusions are absent.
            <Ryad nazvanie="список правил не показан" poyasnenie="причина названа выше" aktiven={false} />
          ) : spisokOtlozhen || zhdyomHello ? (
            <Ryad nazvanie="список правил недоступен" poyasnenie={spisokOtlozhen ?? "ожидание ответа службы"} aktiven={false} />
          ) : stroki.length === 0 ? (
            <Ryad nazvanie={vid === "protsessy" ? "процессов в исключениях нет" : "доменов в исключениях нет"} poyasnenie="весь трафик идёт через туннель" aktiven={false} />
          ) : (
            <div
              data-testid="spisok-pravil"
              className={
                "max-h-[60vh] overflow-y-auto" +
                (zhdutPodyoma || status.kill_switch || vid === "domeny" || prichinaPravki ? " border-border border-t" : "")
              }
            >
              {stroki.map((p) => (
                <Ryad key={p} testId={`pravilo-${p}`} nazvanie={p} lomat>
                  {udalyayu === p ? (
                    <Knopka rang="opasnaya" testId={`podtverdit-${p}`} aktiven={mozhnoPravit} onClick={() => udalit(p)}>
                      удалить?
                    </Knopka>
                  ) : (
                    <Knopka
                      rang="tekst"
                      testId={`udalit-${p}`}
                      aria-label={`удалить ${p}`}
                      aktiven={mozhnoPravit}
                      onClick={() => zadatUdalyayu(p)}
                    >
                      <IkonkaKorzina />
                    </Knopka>
                  )}
                </Ryad>
              ))}
            </div>
          )}
        </Karta>
      </Razdel>

      <Razdel nazvanie="российские сайты">
        <Karta>
          <Ryad
            testId="ru-spisok-ryad"
            nazvanie="российские сайты мимо туннеля"
            poyasnenie={[
              "готовый список доменов, обновляется сам",
              status.kill_switch ? "в режиме «весь трафик» не действует" : undefined,
              prichinaPravki,
            ].filter(Boolean).join(" · ")}
            aktiven={mozhnoPravit}
          >
            <Tumbler
              testId="ru-spisok"
              podpis="российские сайты мимо туннеля"
              // Тумблер положительный, поле в наборе отрицательное. Так
              // намеренно: в окне человек включает список, а в наборе
              // отсутствие поля означает список на месте.
              vkl={pravila?.bez_ru_spiska !== true}
              aktiven={mozhnoPravit}
              // Списки уходят вместе с флагом: setRules заменяет набор целиком,
              // и тело без них стёрло бы все исключения человека.
              naSmenu={(vkl) => {
                if (!pravila) return;
                naKomandu("setRules", { protsessy: pravila.protsessy, domeny: pravila.domeny, bez_ru_spiska: !vkl });
              }}
            />
          </Ryad>
        </Karta>
      </Razdel>

      <Razdel nazvanie="журнал соединений">
        <Karta>
          <Ryad
            testId="zhurnal-ryad"
            nazvanie="вести журнал"
            poyasnenie={[
              "что и куда пошло: время, процесс, адрес, выход; хранится сутки",
              zhurnalOtlozhen,
            ].filter(Boolean).join(" · ")}
            aktiven={!molchit && !zhurnalOtlozhen}
          >
            <Tumbler
              testId="zhurnal"
              podpis="вести журнал"
              vkl={status.zhurnal ?? false}
              aktiven={!molchit && !zhdyomHello && !zhurnalOtlozhen}
              naSmenu={(vkl) => naKomandu("setJournal", { vkl })}
            />
          </Ryad>
          <Ryad
            testId="diagnostika-ryad"
            nazvanie="подробный журнал для отладки"
            poyasnenie="раз в секунду: дескрипторы, занятые порты, соединения, скорость, задержка ядра; нужен, когда связь пропадает без видимой причины"
            aktiven={!molchit}
          >
            <Tumbler
              testId="diagnostika"
              podpis="подробный журнал для отладки"
              vkl={status.diagnostika ?? false}
              aktiven={!molchit && !zhdyomHello}
              naSmenu={(vkl) => naKomandu("setDiagnostics", { vkl })}
            />
          </Ryad>
          <Ryad
            nazvanie="очистить журнал"
            poyasnenie={chistkaOtlozhena ?? "стирает файл соединений"}
            aktiven={!molchit && !chistkaOtlozhena}
          >
            <Knopka
              rang="vtoraya"
              testId="ochistit-zhurnal"
              aktiven={!molchit && !zhdyomHello && !chistkaOtlozhena}
              onClick={() => naKomandu("clearJournal", {})}
            >
              Очистить
            </Knopka>
          </Ryad>
        </Karta>
      </Razdel>
    </Kolonka>
  );
}

function Plyus() {
  return <svg viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round"><path d="M12 5v14M5 12h14" /></svg>;
}

/** Our own picker instead of the native <select>: the native one painted a
 *  white list with blank rows over the dark window (owner, 03.09.2026). Same
 *  grammar as the server list: a button-like field, a listbox of rows. */
function VyborZapushchennogo({ zapushchennye, naVybor }: { zapushchennye: Zapushchennyy[]; naVybor: (put: string) => void }) {
  const [otkryt, zadatOtkryt] = useState(false);
  return (
    <div className="relative">
      <button
        type="button"
        data-testid="zapushchennye"
        aria-haspopup="listbox"
        aria-expanded={otkryt}
        onClick={() => zadatOtkryt((o) => !o)}
        className="bg-fill-subtle text-foreground border-border hover:bg-fill flex h-8 w-full items-center justify-between rounded-md border px-2 text-[13px]"
      >
        <span className="text-fg-muted">выбрать из запущенных</span>
        <span aria-hidden="true" className="text-fg-muted text-[11px]">{otkryt ? "▴" : "▾"}</span>
      </button>
      {otkryt && (
        <div
          role="listbox"
          aria-label="запущенные процессы"
          className="bg-surface border-border absolute left-0 right-0 z-10 mt-1 max-h-64 overflow-y-auto rounded-lg border shadow-lg"
        >
          {zapushchennye.map((z) => (
            <div
              key={z.put}
              role="option"
              aria-selected={false}
              tabIndex={0}
              data-testid={`zapushchennyy-${z.imya}`}
              onClick={() => { naVybor(z.put); zadatOtkryt(false); }}
              onKeyDown={(e) => { if (e.key === "Enter") { naVybor(z.put); zadatOtkryt(false); } }}
              className="border-border hover:bg-fill-subtle flex h-9 cursor-pointer items-center gap-2.5 border-t px-3 text-[13px] first:border-t-0"
            >
              <span className="text-foreground shrink-0 font-medium">{z.imya}</span>
              <span className="text-fg-muted truncate text-xs">{z.put}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
