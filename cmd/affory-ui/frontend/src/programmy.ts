import type { Zapushchennyy } from "./most";

// Путь к клиенту сервиса.
//
// Имена файлов знает каталог (`programmy` у сервиса), путь - только машина.
// Путь НЕ угадывается: у Discord он содержит номер сборки
// (`app-1.0.9xxx\Discord.exe`), у лаунчеров зависит от диска установки, и
// прибитый шаблон дал бы правило на несуществующий файл, то есть выдуманный
// охват. Имя файла ищется среди СЕЙЧАС ЗАПУЩЕННЫХ программ, и в правило уезжает
// фактический путь. Не запущено - карточка так и говорит.

/** Имя файла из полного пути, в нижнем регистре. */
export function imyaFayla(put: string): string {
  return (put.split(/[/\\]/).pop() ?? "").toLowerCase();
}

/** Пути этих программ среди запущенных. Пусто значит «не запущено», а не «не
 *  установлено»: окно не заглядывает в диск и не должно делать вид, что
 *  заглянуло. Две копии одной программы дают два пути, и обе настоящие. */
export function naydennyePuti(imena: string[] | undefined, zapushchennye: Zapushchennyy[] | null | undefined): string[] {
  if (!imena?.length || !zapushchennye) return [];
  const iskomye = new Set(imena.map((i) => i.toLowerCase()));
  const puti: string[] = [];
  for (const p of zapushchennye) {
    if (iskomye.has(imyaFayla(p.put)) && !puti.includes(p.put)) puti.push(p.put);
  }
  return puti;
}
