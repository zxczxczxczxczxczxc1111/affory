import { useState } from "react";
import { Knopka } from "./ui";

// Uninstall dialog. The one irreversible screen in the program, so it asks
// the one question that matters out loud: keep the keys or erase them. The
// confirm button stays disabled until the human has picked, and "keep" is
// labelled as the safe default rather than pre-selected on their behalf.

export interface UdalenieProps {
  naUdalenie: (steretKlyuchi: boolean) => void;
  naOtmenu: () => void;
}

export function Udalenie({ naUdalenie, naOtmenu }: UdalenieProps) {
  const [vybor, zadatVybor] = useState<"ostavit" | "steret" | null>(null);

  return (
    <section
      role="dialog"
      aria-label="Удаление программы"
      className="border-border bg-surface flex max-w-xl flex-col gap-4 rounded-xl border p-6"
    >
      {/* Экран писался до редизайна и остался со строчными заголовками,
          своими кнопками и чужим радиусом. Приведён к остальному окну
          23.09.2026; вопрос и порядок ответов те же. */}
      <h3 className="text-foreground text-lg font-semibold">Удалить программу</h3>
      <p className="text-fg-secondary text-sm leading-relaxed">
        Снимутся защита сети, VPN, служба и автозапуск, потом удалится папка программы.
        Ключи серверов лежат отдельно и переживают удаление службы, поэтому о них отдельный вопрос.
      </p>

      <fieldset className="flex flex-col gap-2">
        <legend className="text-foreground mb-1 text-sm font-medium">Что делать с ключами</legend>
        <label className="text-foreground flex items-start gap-2 text-sm leading-relaxed">
          <input type="radio" name="klyuchi" className="mt-0.5 accent-[var(--color-accent)]" checked={vybor === "ostavit"} onChange={() => zadatVybor("ostavit")} />
          <span>
            <span data-testid="umolchanie">Оставить <span className="text-fg-muted">- безопасный ответ: после переустановки серверы окажутся на месте</span></span>
          </span>
        </label>
        <label className="text-foreground flex items-start gap-2 text-sm leading-relaxed">
          <input type="radio" name="klyuchi" className="mt-0.5 accent-[var(--color-danger)]" checked={vybor === "steret"} onChange={() => zadatVybor("steret")} />
          <span>
            Стереть <span className="text-danger">- насовсем, восстановить будет нечем</span>
          </span>
        </label>
      </fieldset>

      <div className="flex gap-2">
        <Knopka
          rang="opasnaya"
          bolshaya
          testId="podtverdit"
          aktiven={vybor !== null}
          onClick={() => vybor !== null && naUdalenie(vybor === "steret")}
        >
          Удалить
        </Knopka>
        <Knopka rang="tekst" bolshaya testId="otmena" onClick={naOtmenu}>
          Отмена
        </Knopka>
      </div>
    </section>
  );
}
