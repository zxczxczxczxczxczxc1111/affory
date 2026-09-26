import { useEffect, useState } from "react";
import { slovoPosleChisla } from "../chisla";
import { Karta, Knopka, PoleTeksta, Segment } from "./ui";

// Выгрузка серверов (26.09.2026): ссылки из exportServers, чтобы перенести
// ключи на другой ПК или телефон строкой или QR.
//
// Текст и код по умолчанию закрыты. Панель открывают и при демонстрации
// экрана, а строка на экране это ключ в трансляции. «Скопировать» работает
// сразу и ничего не показывает.

/** Ответ exportServers, см. cmd/affory-svc/komandy_pachki.go. */
export interface VygruzkaOtvet {
  tekst: string;
  base64: string;
  vsego: number;
  propushcheny?: { imya: string; prichina: string }[];
}

export function Vygruzka({ vygruzka, zakryt, skopirovat, kodyQr }: {
  vygruzka: VygruzkaOtvet;
  zakryt: () => void;
  skopirovat?: (tekst: string) => Promise<void>;
  kodyQr?: (tekst: string) => Promise<string[]>;
}) {
  const [vid, zadatVid] = useState<"spisok" | "base64">("spisok");
  const [tekstViden, zadatTekstViden] = useState(false);
  const [kody, zadatKody] = useState<string[] | null>(null);
  const [soobshchenie, zadatSoobshchenie] = useState<string | null>(null);
  const tekst = vid === "spisok" ? vygruzka.tekst : vygruzka.base64;

  // Новая выгрузка закрывает прежний код: картинка старого списка рядом со
  // свежим текстом врала бы про один из них.
  useEffect(() => { zadatKody(null); zadatSoobshchenie(null); }, [vygruzka]);

  const kopirovat = async () => {
    if (!skopirovat) return;
    try {
      await skopirovat(tekst);
      zadatSoobshchenie("скопировано");
    } catch (e: unknown) {
      zadatSoobshchenie(`не скопировалось: ${e instanceof Error ? e.message : String(e)}`);
    }
  };
  const pokazatQr = async () => {
    if (kody) { zadatKody(null); return; }
    if (!kodyQr) return;
    try {
      // В код всегда идёт список, а не base64: он на треть короче, и чужие
      // клиенты читают оба.
      zadatKody(await kodyQr(vygruzka.tekst));
      zadatSoobshchenie(null);
    } catch (e: unknown) {
      zadatSoobshchenie(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <Karta testId="vygruzka">
      <div className="flex flex-col gap-3 px-4 py-3">
        <div className="flex items-center gap-3">
          <span className="text-foreground flex-1 text-sm font-medium" data-testid="vygruzka-itog">
            {`Выгрузка: ${vygruzka.vsego} ${slovoPosleChisla(vygruzka.vsego, "сервер", "сервера", "серверов")}`}
          </span>
          <Knopka rang="tekst" testId="vygruzka-zakryt" onClick={zakryt}>Закрыть</Knopka>
        </div>
        <p className="text-warn text-xs">
          в этих строках ключи: кто их получит, тот подключится к твоим серверам
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Segment<"spisok" | "base64">
            aria-label="вид выгрузки"
            znacheniya={[{ z: "spisok", podpis: "списком" }, { z: "base64", podpis: "одной строкой base64" }]}
            vybrano={vid}
            naVybor={(z) => { zadatVid(z); zadatSoobshchenie(null); }}
          />
          <Knopka rang="glavnaya" testId="vygruzka-kopirovat" aktiven={!!skopirovat} onClick={() => void kopirovat()}>
            Скопировать
          </Knopka>
          <Knopka rang="vtoraya" testId="vygruzka-tekst" onClick={() => zadatTekstViden(!tekstViden)}>
            {tekstViden ? "Скрыть текст" : "Показать текст"}
          </Knopka>
          {kodyQr && (
            <Knopka rang="vtoraya" testId="vygruzka-qr" onClick={() => void pokazatQr()}>
              {kody ? "Скрыть QR" : "Показать QR"}
            </Knopka>
          )}
        </div>
        {soobshchenie && (
          <p className="text-fg-secondary text-xs" data-testid="vygruzka-soobshchenie">{soobshchenie}</p>
        )}
        {tekstViden && (
          <PoleTeksta testId="vygruzka-pole" aria-label="ссылки на серверы" znachenie={tekst} tolkoChtenie strok={6} />
        )}
        {kody && (
          <div className="flex flex-wrap gap-4" data-testid="vygruzka-kody">
            {kody.map((kod, i) => (
              <figure key={kod} className="flex w-full max-w-[360px] flex-col items-center gap-1.5">
                {/* Белое поле обязательно: камера ищет тёмные модули на
                    светлом, и код на тёмной подложке окна не читается. */}
                <img src={kod} alt={`QR ${i + 1} из ${kody.length}`} className="aspect-square w-full rounded-md bg-white" />
                {kody.length > 1 && (
                  <figcaption className="text-fg-muted text-xs">{`код ${i + 1} из ${kody.length}`}</figcaption>
                )}
              </figure>
            ))}
          </div>
        )}
        {(vygruzka.propushcheny?.length ?? 0) > 0 && (
          <ul className="text-fg-muted text-xs" data-testid="vygruzka-propushcheny">
            {vygruzka.propushcheny!.map((p) => (
              <li key={p.imya}>{`не выгружен ${p.imya}: ${p.prichina}`}</li>
            ))}
          </ul>
        )}
      </div>
    </Karta>
  );
}
