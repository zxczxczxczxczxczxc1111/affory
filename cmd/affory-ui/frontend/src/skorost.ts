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
export function useSkorost(request: Request, available: boolean) {
  const [snapshot, setSnapshot] = useState<SpeedSnapshot | null>(null);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const epoch = useRef(0);
  const commandRunning = useRef(false);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; epoch.current++; }; }, []);
  useEffect(() => {
    if (!available) { epoch.current++; setSnapshot(null); return; }
    let live = true, busy = false;
    const poll = async () => {
      if (busy || commandRunning.current) return;
      busy = true;
      const turn = epoch.current;
      try {
        const reply = await request("speedTestStatus", {});
        if (!live || turn !== epoch.current || !alive.current) return;
        if (reply.oshibka) return;
        const next = readSpeed(reply.telo);
        if (next) setSnapshot(next);
      } catch (e: unknown) {
        if (live && turn === epoch.current) {
          setSnapshot(null);
          setError(e instanceof Error ? e.message : "Не удалось получить состояние замера");
        }
      } finally { busy = false; }
    };
    void poll();
    const timer = window.setInterval(() => void poll(), 1000);
    return () => { live = false; window.clearInterval(timer); };
  }, [request, available]);
  const command = useCallback(async (name: string, body: Record<string, unknown>) => {
    if (commandRunning.current) return;
    commandRunning.current = true; epoch.current++; setPending(true); setError("");
    if (name === "startSpeedTest") setSnapshot(null);
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
