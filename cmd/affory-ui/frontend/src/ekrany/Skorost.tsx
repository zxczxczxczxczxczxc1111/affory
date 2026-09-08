import { useState } from "react";
import { Vybor } from "./Vybor";
import type { SpeedSnapshot } from "../skorost";

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
    <section className="af-speed" aria-label="Скорость подключения">
      <div className="af-speed-head">
        <div><h3>Скорость подключения</h3><p>{snapshot && snapshot.phase !== "idle" ? snapshot.path === "vpn" ? "Через Affory" : "Текущее подключение" : "Приём и отдача одним нажатием"}</p></div>
        <button type="button" className="af-speed-button" disabled={pending || (!running && disabled)} onClick={() => running ? cancel() : start(provider)}>
          {pending ? "Подождите…" : running ? "Остановить" : "Проверить"}
        </button>
      </div>
      {running && <div className="af-speed-progress" role="status"><span className="af-speed-spinner" aria-hidden="true" />{snapshot.phase === "download" ? "Измеряем приём" : "Измеряем отдачу"} · {snapshot.name || "Выбираем сервис"}{snapshot.attempt > 1 ? ` · попытка ${snapshot.attempt} из 4` : ""}</div>}
      {result && <div className="af-speed-result" role="status"><span><small>↓ Приём</small><b>{format(result.download_mbps)} <em>Мбит/с</em></b></span><span><small>↑ Отдача</small><b>{format(result.upload_mbps)} <em>Мбит/с</em></b></span><p>{result.name}</p></div>}
      {message && <p className="af-note" role="status">{message}</p>}
      <details className="af-speed-options"><summary>Сервис замера</summary>
        <Vybor label="Первый сервис замера" value={provider} disabled={running || pending} onChange={setProvider}
          options={[{value: "", label: "Автоматически"}, {value: "librespeed", label: "LibreSpeed · Helsinki"}, {value: "clouvider", label: "Clouvider · Amsterdam"}, {value: "openspeedtest", label: "OpenSpeedTest"}, {value: "cloudflare", label: "Cloudflare"}]} />
        <p className="af-note">Если сервис недоступен, Affory попробует следующий. До минуты и примерно 550 МБ при четырёх попытках. Измеряется скорость передачи данных до тестового сервиса.</p>
        {snapshot?.result?.attempts.map((attempt, i) => <p className="af-note" key={i}>{attempt.name}: {attempt.error || "Оба направления измерены"}</p>)}
      </details>
    </section>
  );
}
