import { Polosa } from "./ui";
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
    // Единственное содержимое окна на этом шаге: прижатый в угол блок
    // оставлял три четверти чёрного поля и читался как недогрузившийся
    // экран. Заголовок с прописной, как у всех остальных экранов.
    <section className="flex h-full flex-col items-center justify-center gap-4 p-8" aria-label="Первый запуск">
      <div className="flex w-full max-w-prose flex-col gap-4">
      <h2 className="text-foreground text-2xl font-semibold">Служба не установлена</h2>
      <p className="text-fg-secondary max-w-prose text-sm">
        VPN поднимает служба Windows, и ей нужны права администратора один раз,
        при установке; дальше программа работает без запросов
      </p>
      {sostoyanie === "otkaz" && prichina && (
        <p className="text-danger text-sm" data-testid="prichina">{prichina}</p>
      )}
      {idyot ? (
        <div className="flex flex-col gap-2">
          <p className="text-fg-muted text-sm" data-testid="hod">
            установка идёт, ответь на запрос Windows и подожди
          </p>
          {/* Запрос UAC может оказаться за окном, а служба поднимается
              несколько секунд: без движения на экране это неотличимо от
              зависшей программы. */}
          <Polosa podpis="ход установки службы" />
        </div>
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
      </div>
    </section>
  );
}
