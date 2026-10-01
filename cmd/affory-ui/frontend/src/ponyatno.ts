// Текст ошибки для человека (01.10.2026). Зеркало sboi.ObrezatTehniku в
// службе: наша часть сообщения русская и идёт первой, чужая (Go, Windows,
// WebView2) английская и идёт хвостом после двоеточия. Хвост отрезается.
//
// Служба режет свои тексты сама и знает тип ошибки, окно видит только строку.
// Здесь ловится то, что пришло мимо службы: диалоги, буфер обмена, запуск
// Проводника, оборванный канал.

/** Что сказать, когда от текста не осталось ни одного нашего слова. */
export const TEKST_NICHEGO_NE_OSTALOS = "что-то пошло не так";

type Chey = "nashe" | "chuzhoe" | "neytralno";

function chey(zveno: string): Chey {
  let kir = 0;
  let lat = 0;
  for (const slovo of zveno.split(/\s+/)) {
    // Пути, имена файлов и адреса не голосуют: длинный путь к чужой программе
    // иначе перевесил бы русскую фразу вокруг него.
    const goloe = slovo.replace(/^[.,;:!?()«»"']+|[.,;:!?()«»"']+$/g, "");
    if (/[\\/.]/.test(goloe)) continue;
    for (const b of goloe) {
      if (/[а-яё]/i.test(b)) kir++;
      else if (/[a-z]/i.test(b)) lat++;
    }
  }
  if (kir > 0 && kir >= lat) return "nashe";
  if (kir === 0 && lat === 0) return "neytralno";
  return "chuzhoe";
}

/** Наша часть текста. Пустая строка значит «нашего тут ничего нет». */
export function ponyatno(tekst: string): string {
  const nashi: string[] = [];
  for (const syroe of tekst.split(": ")) {
    const z = syroe.trim();
    const c = chey(z);
    if (c === "nashe" || (c === "neytralno" && nashi.length > 0)) {
      nashi.push(z);
      continue;
    }
    if (c === "chuzhoe" && nashi.length > 0) break;
  }
  return nashi.join(": ").replace(/[\s.:;,]+$/, "");
}

/** Текст пойманного исключения для экрана: никогда не пустой. */
export function tekstOshibki(e: unknown): string {
  const syroe = e instanceof Error ? e.message : String(e);
  return ponyatno(syroe) || TEKST_NICHEGO_NE_OSTALOS;
}
