import { useEffect, useRef, useState } from "react";
import type { Statistika } from "../protokol";

interface Skorosti { priem?: number; otdacha?: number }
interface Snimok { dannye: Statistika; vremya: number }
const SROK = 5000;

function raznica(novoe: number | undefined, staroe: number | undefined, ms: number): number | undefined {
  if (novoe === undefined || staroe === undefined || !Number.isFinite(novoe) ||
      !Number.isFinite(staroe) || novoe < 0 || staroe < 0 || novoe < staroe || ms <= 0 || ms > SROK) return undefined;
  return (novoe - staroe) * 8 / (ms * 1000);
}

export function formatSkorosti(value: number | undefined): string {
  return value === undefined ? "-" : `${value.toFixed(1).replace(".", ",")} Мбит/с`;
}

/** Скорость между полученными снимками, а не накопленный объём за время подключения. */
export function useSkorostTrafika(statistika: Statistika | null, podnyat: boolean, sessiya: string): Skorosti {
  const predydushchiy = useRef<Snimok | null>(null);
  const posledniy = useRef<Statistika | null>(null);
  const [skorosti, zadatSkorosti] = useState<Skorosti>({});
  useEffect(() => {
    predydushchiy.current = null;
    zadatSkorosti({});
  }, [podnyat, sessiya]);
  useEffect(() => {
    // Смена состояния не превращает оставшийся в App старый снимок в новый замер.
    if (!podnyat || !statistika) {
      posledniy.current = statistika;
      predydushchiy.current = null;
      zadatSkorosti({});
      return;
    }
    if (posledniy.current === statistika) return;
    posledniy.current = statistika;
    const vremya = performance.now();
    const pred = predydushchiy.current;
    zadatSkorosti(pred ? {
      priem: raznica(statistika.prinyato, pred.dannye.prinyato, vremya - pred.vremya),
      otdacha: raznica(statistika.otdano, pred.dannye.otdano, vremya - pred.vremya),
    } : {});
    predydushchiy.current = { dannye: statistika, vremya };
    const timer = setTimeout(() => {
      predydushchiy.current = null;
      zadatSkorosti({});
    }, SROK);
    return () => clearTimeout(timer);
  }, [statistika, podnyat, sessiya]);
  return podnyat ? skorosti : {};
}
