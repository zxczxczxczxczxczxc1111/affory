import type { Zapushchennyy } from "./most";

// Частые приложения: Discord, Telegram, игровые лаунчеры.
//
// Зачем отдельно от каталога сервисов. Карточка сервиса это НАБОР ДОМЕНОВ, и
// нативный клиент она не покрывает: программа ходит к адресам без имени, а
// браузерный DoH или ECH прячет имя даже там, где оно есть. Поэтому «Discord»
// в сервисах и «Discord» здесь это разные вещи, и обещать, что одна карточка
// заменяет другую, нельзя.
//
// Путь НЕ угадывается. У Discord он содержит номер сборки
// (`app-1.0.9xxx\Discord.exe`), у лаунчеров зависит от диска установки, и
// прибитый шаблон дал бы правило на несуществующий файл - то есть выдуманный
// охват. Вместо этого имя файла ищется среди СЕЙЧАС ЗАПУЩЕННЫХ программ, и в
// правило уезжает фактический путь. Не запущено - окно так и говорит.

export interface Preset {
  id: string;
  imya: string;
  /** Имена файлов в нижнем регистре. Несколько - это ветки выпуска (PTB,
   *  Canary) и разные редакции одной программы, а не догадки. */
  fayly: string[];
  /** Чем это приложение отличается от одноимённой карточки сервиса. */
  poyasnenie: string;
}

export const PRESETY: Preset[] = [
  { id: "discord", imya: "Discord", fayly: ["discord.exe", "discordptb.exe", "discordcanary.exe"],
    poyasnenie: "Голос ходит к адресам без имени: доменная карточка их не накрывает" },
  { id: "telegram", imya: "Telegram", fayly: ["telegram.exe", "telegram desktop.exe"],
    poyasnenie: "Клиент соединяется со своими адресами напрямую, минуя имена сайтов" },
  { id: "steam", imya: "Steam", fayly: ["steam.exe"],
    poyasnenie: "Лаунчер и запущенные им игры" },
  { id: "epic", imya: "Epic Games", fayly: ["epicgameslauncher.exe"],
    poyasnenie: "Лаунчер и запущенные им игры" },
  { id: "battlenet", imya: "Battle.net", fayly: ["battle.net.exe"],
    poyasnenie: "Лаунчер и запущенные им игры" },
  { id: "riot", imya: "Riot Client", fayly: ["riotclientservices.exe"],
    poyasnenie: "Лаунчер и запущенные им игры" },
  { id: "gog", imya: "GOG Galaxy", fayly: ["galaxyclient.exe"],
    poyasnenie: "Лаунчер и запущенные им игры" },
  { id: "ea", imya: "EA app", fayly: ["eadesktop.exe", "eabackgroundservice.exe"],
    poyasnenie: "Лаунчер и запущенные им игры" },
  { id: "ubisoft", imya: "Ubisoft Connect", fayly: ["upc.exe", "ubisoftconnect.exe"],
    poyasnenie: "Лаунчер и запущенные им игры" },
];

/** Имя файла из полного пути, в нижнем регистре. */
export function imyaFayla(put: string): string {
  return (put.split(/[/\\]/).pop() ?? "").toLowerCase();
}

/** Пути этого пресета среди запущенных программ. Пусто значит «не запущено»,
 *  а не «не установлено»: окно не заглядывает в диск и не должно делать вид,
 *  что заглянуло. Две копии одной программы дают два пути, и обе настоящие. */
export function naydennyePuti(preset: Preset, zapushchennye: Zapushchennyy[] | null | undefined): string[] {
  if (!zapushchennye) return [];
  const iskomye = new Set(preset.fayly);
  const puti: string[] = [];
  for (const p of zapushchennye) {
    if (iskomye.has(imyaFayla(p.put)) && !puti.includes(p.put)) puti.push(p.put);
  }
  return puti;
}
