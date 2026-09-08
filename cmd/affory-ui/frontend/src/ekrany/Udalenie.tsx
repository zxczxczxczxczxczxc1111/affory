import { useState } from "react";

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
      className="border-border bg-surface flex max-w-xl flex-col gap-4 rounded-lg border p-6"
    >
      <h3 className="text-foreground text-lg font-semibold">удалить программу</h3>
      <p className="text-fg-secondary text-sm">
        снимутся правила брандмауэра, туннель, служба и автозапуск, потом каталог программы.
        ключи серверов лежат отдельно и переживают снятие службы, поэтому вопрос
      </p>

      <fieldset className="flex flex-col gap-2">
        <legend className="text-foreground text-sm font-medium">что делать с ключами</legend>
        <label className="text-foreground flex items-start gap-2 text-sm">
          <input type="radio" name="klyuchi" checked={vybor === "ostavit"} onChange={() => zadatVybor("ostavit")} />
          <span>
            оставить <span className="text-fg-muted" data-testid="umolchanie">(оставить это безопасный ответ: переустановка найдёт серверы на месте)</span>
          </span>
        </label>
        <label className="text-foreground flex items-start gap-2 text-sm">
          <input type="radio" name="klyuchi" checked={vybor === "steret"} onChange={() => zadatVybor("steret")} />
          <span>
            стереть <span className="text-danger">(насовсем, восстановить будет нечем)</span>
          </span>
        </label>
      </fieldset>

      <div className="flex gap-3">
        <button
          type="button"
          data-testid="podtverdit"
          disabled={vybor === null}
          onClick={() => vybor !== null && naUdalenie(vybor === "steret")}
          className="bg-danger text-background disabled:bg-fill disabled:text-fg-faint rounded-md px-4 py-2 text-sm font-medium"
        >
          Удалить
        </button>
        <button
          type="button"
          data-testid="otmena"
          onClick={naOtmenu}
          className="text-fg-secondary hover:text-foreground rounded-md px-4 py-2 text-sm"
        >
          Отмена
        </button>
      </div>
    </section>
  );
}
