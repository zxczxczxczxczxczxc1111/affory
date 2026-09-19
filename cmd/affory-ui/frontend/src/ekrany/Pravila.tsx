import type { KatalogServisov, PravilaTrafika } from "../trafik";
import { Marshruty, RazdelZhurnala } from "./Marshruty";
import type { OtkazNaEkrane, StatusOtvet } from "../protokol";
import type { Zapushchennyy } from "../most";
import { Knopka, Neudacha } from "./ui";

// Раздел правил. Сам экран живёт в Marshruty; здесь только разбор случая,
// когда правил на руках НЕТ, и честный вид для него.
//
// Служба отдаёт набор маршрутов (`trafik`) с 1.0.0 и сама переносит в него
// прежние списки процессов и доменов, поэтому «правил нет» означает ровно
// одно: ответа нет. Служба молчит, hello ещё не пришёл, listRules отложена
// или отказала. Редактировать в этом случае нечего, и экран говорит, чего
// именно он ждёт, вместо пустых таблиц.

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
  /** Команды в полёте, для вертушки на своей кнопке. `zanyato` выше это про
   *  другое: оно гасит экран целиком на время команды, меняющей туннель. */
  zanyatyeKomandy?: Record<string, boolean>;
}

/** "появится в волне N" for a deferred command, or undefined once it exists. */
export function otlozhenaDo(otlozheno: Record<string, number> | null, komanda: string): string | undefined {
  const volna = otlozheno?.[komanda];
  return volna === undefined ? undefined : `команда ${komanda} появится в волне ${volna}`;
}

export function Pravila(props: PravilaProps) {
  return props.pravila?.trafik ? (
    <Marshruty {...props} trafik={props.pravila.trafik} />
  ) : (
    <PravilaBezDannyh {...props} />
  );
}

/** Правила не прочитаны. Раскладка та же, что у рабочего экрана: боковая
 *  колонка слева, содержимое справа, журнал под ним. Так переход из этого
 *  состояния в рабочее ничего на экране не двигает.
 *
 *  Чего здесь нет намеренно: счёта правил и переключателя режима. Ноль правил
 *  это ИЗМЕРЕННОЕ значение, а мы ничего не мерили, и положение переключателя
 *  без ответа службы было бы выдумкой. */
function PravilaBezDannyh({
  status,
  otlozheno,
  pravilaOtkaz = null,
  obnovitPravila,
  naKomandu,
  zanyatyeKomandy = {},
}: PravilaProps) {
  const molchit = status.sostoyanie === "sluzhba-molchit";
  const zhdyomHello = otlozheno === null;
  const spisokOtlozhen = otlozhenaDo(otlozheno, "listRules");
  // Почему правил нет, в случаях без собственной строки рядом. Молчание тут
  // однажды оставило экран, который выглядит сломанным без объяснений
  // (03.09.2026, служба лежала).
  const prichina = molchit
    ? "Служба не отвечает"
    : zhdyomHello || spisokOtlozhen || pravilaOtkaz
      ? undefined
      : "Список правил не получен";

  return (
    <section className="flex min-h-0 flex-1" aria-label="Правила">
      <aside className="border-border flex w-[288px] shrink-0 flex-col gap-4 overflow-y-auto border-r px-6 py-6">
        <h2 className="text-foreground text-[22px] font-semibold leading-tight">Куда идёт трафик</h2>
        <p className="text-fg-muted text-sm leading-relaxed">
          Режим и правила появятся здесь, как только служба ответит
        </p>
        {zhdyomHello && (
          <p className="text-fg-secondary text-[13px]" data-testid="zagruzka" role="status">
            Жду ответа службы
          </p>
        )}
        {spisokOtlozhen && (
          <p className="text-fg-muted text-[13px] leading-relaxed" data-testid="pravila-otlozheny">
            {spisokOtlozhen}
          </p>
        )}
        {prichina && (
          <p className="text-fg-muted text-[13px] leading-relaxed" data-testid="pravka-nedostupna">
            {prichina}
          </p>
        )}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
        <div className="flex min-w-0 flex-1 flex-col px-8 py-6">
          {pravilaOtkaz ? (
            <Neudacha
              testId="otkaz-pravil"
              kod={pravilaOtkaz.kod}
              zagolovok="Правила не прочитались"
              tekst={pravilaOtkaz.tekst}
              deystvie={obnovitPravila}
              podpisDeystviya="Повторить"
            />
          ) : (
            <div
              className="border-border flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-12 text-center"
              data-testid="net-pravil"
            >
              <p className="text-foreground text-sm font-medium">Правила ещё не прочитаны</p>
              <p className="text-fg-muted max-w-[440px] text-[13px] leading-relaxed">
                {molchit
                  ? "Служба не отвечает. Маршруты продолжают работать так, как их сохранили в прошлый раз"
                  : spisokOtlozhen ?? "Список придёт от службы вместе с каталогом сервисов"}
              </p>
              {obnovitPravila && (
                <Knopka rang="vtoraya" testId="povtorit-pravila" onClick={obnovitPravila}>
                  Повторить
                </Knopka>
              )}
            </div>
          )}

          <RazdelZhurnala status={status} disabled={molchit || zhdyomHello} naKomandu={naKomandu} zanyatyeKomandy={zanyatyeKomandy} />
        </div>
      </div>
    </section>
  );
}
