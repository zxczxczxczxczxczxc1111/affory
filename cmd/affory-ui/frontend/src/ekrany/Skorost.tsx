import { useState } from "react";
import { Vybor } from "./Vybor";
import { Knopka, Svorachivaemyy, Vertushka } from "./ui";
import type { SpeedSnapshot } from "../skorost";

// Замер скорости под списком серверов. Занимает одну строку, пока его не
// трогают, и разворачивается числами только после нажатия: постоянная плашка
// с прочерками на главном экране это место, потраченное на «ещё не мерили».

export function Skorost({ snapshot, error, pending, disabled, start, cancel }: {
  snapshot: SpeedSnapshot | null; error: string; pending: boolean; disabled: boolean;
  start: (provider: string) => void; cancel: () => void;
}) {
  const [provider, setProvider] = useState("");
  const running = snapshot?.phase === "download" || snapshot?.phase === "upload";
  const result = snapshot?.phase === "complete" ? snapshot.result : undefined;
  const message = error || snapshot?.result?.error || snapshot?.reason;
  const format = (value?: number) => value === undefined ? "…" : new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 1 }).format(value);
  return (
    <section className="border-border bg-rail shrink-0 border-t px-8 py-4" aria-label="Скорость подключения">
      <div className="flex items-center justify-between gap-4">
        <div className="min-w-0">
          <h3 className="text-foreground text-sm font-medium">Скорость подключения</h3>
          <p className="text-fg-muted mt-0.5 truncate text-[13px]">
            {snapshot && snapshot.phase !== "idle"
              ? snapshot.path === "vpn" ? "Через Affory" : "Текущее подключение"
              : "Приём и отдача одним нажатием"}
          </p>
        </div>
        <Knopka
          rang="vtoraya"
          bolshaya
          zhdyot={pending}
          aktiven={!(pending || (!running && disabled))}
          onClick={() => running ? cancel() : start(provider)}
        >
          {pending ? "Подготовка…" : running ? "Остановить" : "Проверить"}
        </Knopka>
      </div>

      {running && (
        <div className="text-accent-ink mt-3 flex items-center gap-2 text-[13px]" role="status">
          <Vertushka className="h-3.5 w-3.5" />
          {snapshot.phase === "download" ? "Замер приёма" : "Замер отдачи"} · {snapshot.name || "Выбор сервиса"}
          {snapshot.attempt > 1 ? ` · попытка ${snapshot.attempt} из 4` : ""}
        </div>
      )}

      {result && (
        <div className="mt-3 grid grid-cols-2 gap-3" role="status">
          <Storona podpis="↓ Замер приёма" chislo={format(result.download_mbps)} />
          <Storona podpis="↑ Замер отдачи" chislo={format(result.upload_mbps)} />
          <p className="text-fg-muted col-span-2 text-[13px]">{result.name}</p>
        </div>
      )}

      {message && <p className="text-fg-muted mt-2 text-[13px] leading-relaxed" role="status">{message}</p>}

      <Svorachivaemyy
        zagolovok="Сервис замера"
        deti={
          <div className="flex flex-col gap-2">
            <div className="max-w-[280px]">
              <Vybor label="Первый сервис замера" value={provider} disabled={running || pending} onChange={setProvider}
                options={[{value: "", label: "Автоматически"}, {value: "librespeed", label: "LibreSpeed · Helsinki"}, {value: "clouvider", label: "Clouvider · Amsterdam"}, {value: "openspeedtest", label: "OpenSpeedTest"}, {value: "cloudflare", label: "Cloudflare"}]} />
            </div>
            <p>
              Если сервис недоступен, Affory попробует следующий. До минуты и примерно 550 МБ при
              четырёх попытках: замер меряет скорость до тестового сервиса, а не до сервера подписки
            </p>
            {snapshot?.result?.attempts.map((attempt, i) => (
              <p key={i}>{attempt.name}: {attempt.error || "Оба направления измерены"}</p>
            ))}
          </div>
        }
      />
    </section>
  );
}

function Storona({ podpis, chislo }: { podpis: string; chislo: string }) {
  return (
    <span className="flex flex-col gap-1">
      <small className="text-fg-muted text-[13px]">{podpis}</small>
      <b className="text-foreground text-[21px] font-semibold leading-none">
        {chislo} <em className="text-fg-muted text-[13px] font-normal not-italic">Мбит/с</em>
      </b>
    </span>
  );
}
