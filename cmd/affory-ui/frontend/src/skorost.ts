import { useCallback, useEffect, useRef, useState } from "react";
import type { Kadr } from "./most";

export interface SpeedSnapshot {
  id: number;
  phase: "idle" | "download" | "upload" | "complete" | "cancelled" | "error";
  path: "vpn" | "system";
  provider: string;
  name: string;
  attempt: number;
  reason?: string;
  result?: {
    name: string;
    download_mbps?: number;
    upload_mbps?: number;
    error?: string;
    attempts: { name: string; error?: string }[];
  };
}
const phases = new Set(["idle", "download", "upload", "complete", "cancelled", "error"]);
export function readSpeed(value: unknown): SpeedSnapshot | null {
  if (!value || typeof value !== "object") return null;
  const v = value as Record<string, unknown>;
  if (typeof v.phase !== "string" || !phases.has(v.phase)) return null;
  const text = (x: unknown) => typeof x === "string" ? x : "";
  const positive = (x: unknown) => typeof x === "number" && Number.isFinite(x) && x > 0 ? x : undefined;
  const snapshot: SpeedSnapshot = {
    id: positive(v.id) ?? 0, phase: v.phase as SpeedSnapshot["phase"], path: v.path === "vpn" ? "vpn" : "system",
    provider: text(v.provider), name: text(v.name), attempt: positive(v.attempt) ?? 0, reason: text(v.reason),
  };
  if (v.result && typeof v.result === "object") {
    const r = v.result as Record<string, unknown>;
    snapshot.result = {
      name: text(r.name), download_mbps: positive(r.download_mbps), upload_mbps: positive(r.upload_mbps), error: text(r.error),
      attempts: Array.isArray(r.attempts) ? r.attempts.flatMap(a => {
        if (!a || typeof a !== "object") return [];
        const attempt = a as Record<string, unknown>;
        return typeof attempt.name === "string" ? [{ name: attempt.name, error: text(attempt.error) }] : [];
      }) : [],
    };
  }
  // Partial numbers never graduate into a successful pair by wishful thinking.
  if (snapshot.phase === "complete" && (!snapshot.result?.download_mbps || !snapshot.result.upload_mbps)) {
    snapshot.phase = "error";
    snapshot.reason = "Сервис не подтвердил оба направления. Повторите замер.";
    snapshot.result = undefined;
  }
  return snapshot;
}

type Request = (name: string, body: Record<string, unknown>) => Promise<Kadr>;

// Пока замер идёт, состояние должно быть свежим: полоса на экране движется.
// В покое хватает и десяти секунд, потому что меняться там нечему, кроме
// замера, запущенного мимо этого окна.
const PERIOD_ZAMERA = 1000;
const PERIOD_POKOYA = 10_000;
export function useSkorost(request: Request, available: boolean) {
  const [snapshot, setSnapshot] = useState<SpeedSnapshot | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const epoch = useRef(0);
  const commandRunning = useRef(false);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; epoch.current++; }; }, []);
  // Период опроса СЛЕДУЕТ фазе, а не тикает всегда.
  //
  // Раньше здесь стоял setInterval на секунду, работавший всё время, пока живо
  // окно. Журнал команд живой машины за двое суток: 25 155 строк, из них
  // 18 338 это speedTestStatus, при двадцати запусках замера за то же время.
  // Диск от этого не страдает, ротация работает; страдает диагностика, потому
  // что настоящие команды уезжают за горизонт ротации вчетверо быстрее.
  //
  // В покое опрос всё равно НУЖЕН, просто редкий: замер можно запустить из
  // второго окна или из affory-cli, и узнать об этом больше неоткуда.
  const idyot = useRef(false);
  useEffect(() => {
    if (!available) { epoch.current++; idyot.current = false; setSnapshot(null); return; }
    let live = true, busy = false;
    let timer = 0;
    const poll = async () => {
      if (busy || commandRunning.current) return;
      busy = true;
      const turn = epoch.current;
      try {
        const reply = await request("speedTestStatus", {});
        if (!live || turn !== epoch.current || !alive.current) return;
        if (reply.oshibka) return;
        const next = readSpeed(reply.telo);
        if (next) { idyot.current = next.phase === "download" || next.phase === "upload"; setSnapshot(next); }
      } catch (e: unknown) {
        if (live && turn === epoch.current) {
          setSnapshot(null);
          setError(e instanceof Error ? e.message : "Не удалось получить состояние замера");
        }
      } finally { busy = false; }
    };
    const zavesti = () => {
      timer = window.setTimeout(async () => {
        await poll();
        if (live) zavesti();
      }, idyot.current || commandRunning.current ? PERIOD_ZAMERA : PERIOD_POKOYA);
    };
    // Первый такт заводится ПОСЛЕ первого ответа, а не рядом с ним: фаза до
    // ответа неизвестна, и таймер, заведённый вслепую, встал бы на редкий
    // период посреди идущего замера.
    void poll().then(() => { if (live) zavesti(); });
    return () => { live = false; window.clearTimeout(timer); };
  }, [request, available]);
  const command = useCallback(async (name: string, body: Record<string, unknown>) => {
    if (commandRunning.current) return;
    commandRunning.current = true; epoch.current++; setPending(true); setError("");
    if (name === "startSpeedTest") { setSnapshot(null); idyot.current = true; }
    try {
      const reply = await request(name, body);
      if (!alive.current) return;
      if (reply.oshibka) throw new Error(reply.oshibka.tekst || "Не удалось выполнить замер");
      const next = readSpeed(reply.telo);
      if (!next) throw new Error("Служба не поддерживает этот замер. Обновите приложение.");
      setSnapshot(next);
    } catch (e: unknown) {
      if (alive.current) setError(e instanceof Error ? e.message : "Не удалось выполнить замер");
    } finally { commandRunning.current = false; if (alive.current) setPending(false); }
  }, [request]);
  return { snapshot, error, pending, start: (provider: string) => void command("startSpeedTest", { provider }), cancel: () => void command("cancelSpeedTest", {}) };
}
