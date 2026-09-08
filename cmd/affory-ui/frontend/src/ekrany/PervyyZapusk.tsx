// First run (spec §9.2): the service is not installed, and exactly one thing
// is active, the install button. Pure over props; App owns the UAC call and
// the status polling that follows it.

export type SostoyanieUstanovki = "net-sluzhby" | "ustanavlivaetsya" | "otkaz";

export interface PervyyZapuskProps {
  sostoyanie: SostoyanieUstanovki;
  /** System text for the `otkaz` state, e.g. a declined UAC prompt. */
  prichina?: string;
  naUstanovku: () => void;
}

export function PervyyZapusk({ sostoyanie, prichina, naUstanovku }: PervyyZapuskProps) {
  const idyot = sostoyanie === "ustanavlivaetsya";
  return (
    <section className="flex flex-col gap-4 p-8" aria-label="Первый запуск">
      <h2 className="text-foreground text-2xl font-semibold">служба не установлена</h2>
      <p className="text-fg-secondary max-w-prose text-sm">
        туннель поднимает служба Windows, ей нужны права администратора один раз, при установке.
        дальше программа работает без запросов
      </p>
      {sostoyanie === "otkaz" && prichina && (
        <p className="text-danger text-sm" data-testid="prichina">{prichina}</p>
      )}
      {idyot ? (
        <p className="text-fg-muted text-sm" data-testid="hod">
          установка идёт, ответь на запрос Windows и подожди
        </p>
      ) : (
        <button
          type="button"
          data-testid="ustanovit"
          onClick={naUstanovku}
          className="bg-accent text-foreground hover:brightness-110 self-start rounded-md px-5 py-2.5 text-base font-medium"
        >
          Установить службу
        </button>
      )}
    </section>
  );
}
