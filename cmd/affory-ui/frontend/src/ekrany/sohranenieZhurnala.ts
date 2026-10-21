import { useState } from "react";
import { ponyatno, TEKST_NICHEGO_NE_OSTALOS } from "../ponyatno";

// «Сохранить журнал» (1.10.0, просьба владельца 21.10.2026) стоит везде, где
// человек видит ошибку: под плашкой отказа и под «служба не отвечает» на
// главном экране. Второе важнее первого: молчащая служба это ровно тот
// случай, когда журнал нужен, а окно читает его без службы само.

/** Сохранение журнала со строкой итога. Пустой итог от оболочки значит
 *  «передумал в диалоге», и строка тогда не появляется. */
export function useSohranenieZhurnala(naSohranit?: () => Promise<string>) {
  const [itog, zadatItog] = useState("");
  const [idet, zadatIdet] = useState(false);
  const sohranit = naSohranit && (async () => {
    zadatIdet(true);
    zadatItog("");
    try {
      zadatItog(await naSohranit());
    } catch (e) {
      zadatItog(`журнал не сохранён: ${ponyatno(e instanceof Error ? e.message : String(e)) || TEKST_NICHEGO_NE_OSTALOS}`);
    } finally {
      zadatIdet(false);
    }
  });
  return { sohranit, itog, zadatItog, idet };
}
