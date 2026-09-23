import { useState } from "react";
import type { StatusOtvet } from "../protokol";
import { Knopka, Tumbler } from "./ui";

export function Zhurnaly({ status, disabled, naKomandu, naPapku, zanyatyeKomandy = {} }: {
  status: StatusOtvet;
  disabled: boolean;
  naKomandu: (komanda: string, telo: unknown) => void;
  naPapku?: () => Promise<void>;
  zanyatyeKomandy?: Record<string, boolean>;
}) {
  const [opening, setOpening] = useState(false);
  const [error, setError] = useState("");
  const open = async () => {
    if (!naPapku || opening) return;
    setOpening(true); setError("");
    try { await naPapku(); }
    catch (e: unknown) { setError(e instanceof Error ? e.message : String(e)); }
    finally { setOpening(false); }
  };
  return <div className="border-border flex flex-col gap-4 border-t p-4" data-testid="zhurnaly">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h3 className="text-foreground text-sm font-medium">Журналы</h3>
      <Knopka rang="vtoraya" aktiven={!!naPapku && !opening} zhdyot={opening} onClick={() => void open()}>
        {opening ? "Открываю…" : "Открыть папку с журналами"}
      </Knopka>
    </div>
    {error && <p role="alert" className="text-danger text-[13px]">{error}</p>}
    <div className="flex items-center justify-between gap-4">
      <span className="flex min-w-0 flex-col gap-1">
        <span className="text-fg-secondary text-sm">Журнал соединений</span>
        <span className="text-fg-muted text-[13px]">Приложение, адрес и маршрут каждого соединения. Помогает проверить работу правил.</span>
      </span>
      <Tumbler testId="zhurnal" podpis="Журнал соединений" aktiven={!disabled && !zanyatyeKomandy.setJournal}
        vkl={status.zhurnal ?? false} naSmenu={vkl => naKomandu("setJournal", {vkl})}/>
    </div>
    <div className="flex items-center justify-between gap-4">
      <span className="flex min-w-0 flex-col gap-1">
        <span className="text-fg-secondary text-sm">Подробный журнал для отладки</span>
        <span className="text-fg-muted text-[13px]">Раз в секунду: порты, соединения, скорость и задержка. Нужен при нестабильной связи.</span>
      </span>
      <Tumbler testId="diagnostika" podpis="Подробный журнал для отладки" aktiven={!disabled && !zanyatyeKomandy.setDiagnostics}
        vkl={status.diagnostika ?? false} naSmenu={vkl => naKomandu("setDiagnostics", {vkl})}/>
    </div>
    <div><Knopka rang="vtoraya" testId="ochistit-zhurnal" aktiven={!disabled && !zanyatyeKomandy.clearJournal}
      zhdyot={!!zanyatyeKomandy.clearJournal} onClick={() => naKomandu("clearJournal", {})}>
      {zanyatyeKomandy.clearJournal ? "Стираю" : "Очистить журнал"}
    </Knopka></div>
  </div>;
}
