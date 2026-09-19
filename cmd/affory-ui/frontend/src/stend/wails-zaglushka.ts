// Подставной рантайм Wails для съёмки снимков README.
//
// Подменяется именно @wailsio/runtime, а не most.ts: мост тогда работает ровно
// тем кодом, что уезжает в выпуск, и снимок показывает настоящий экран, а не
// его копию. Alias живёт в vite.stend.config.ts.
//
// Данные подставные и намеренно узнаваемые: адреса из RFC 5737, ключей нет
// нигде. Настоящие адреса в кадр не попадают никогда.

const VERSIYA = "1.2.0";

const servery = [
  { id: "nl", imya: "Нидерланды · Амстердам", transport: "reality-tcp", host: "203.0.113.11", port: 443, iz_podpiski: true },
  // Транспорты пишутся ровно теми словами, какими их называет служба
  // (`izvestnyeTransporty` в internal/genkonfig/vhod.go). До 19.09.2026 здесь
  // стояли «hysteria2», «vless-ws» и «vless-tcp», которых служба не отдаёт
  // никогда, и стенд показывал экран, какого в жизни не бывает.
  { id: "de", imya: "Германия · Франкфурт", transport: "hy2", host: "203.0.113.12", port: 443, iz_podpiski: true, s_pinom: true },
  { id: "fi", imya: "Финляндия · Хельсинки", transport: "ws", host: "203.0.113.13", port: 443, iz_podpiski: true },
  { id: "se", imya: "Швеция · Стокгольм", transport: "anytls", host: "203.0.113.14", port: 443, iz_podpiski: true },
  { id: "svoy", imya: "Свой сервер", transport: "trojan", host: "203.0.113.15", port: 443, iz_podpiski: false },
];

const status = {
  sostoyanie: "podnyat",
  vybran_id: "nl",
  nesushchiy_id: "nl",
  nesushchiy_imya: "Нидерланды · Амстердам",
  rezhim_marshruta: "ruchnoy",
  trafik_po_umolchaniyu: "vpn",
  kill_switch: false,
  avtozapusk: true,
  podklyuchat_pri_starte: false,
  zhurnal: false,
  diagnostika: false,
  port_proksi: 10809,
  versiya_programmy: VERSIYA,
  versiya_sluzhby: VERSIYA,
  podnyat_s: new Date(Date.now() - 82 * 60 * 1000).toISOString(),
};

const pravila = {
  // protsessy и domeny верхнего уровня это ПРЕЖНИЙ формат (списки строк), а
  // нынешние правила живут в trafik. Перепутать их значит уронить экран.
  protsessy: [] as string[],
  domeny: [] as string[],
  trafik: {
    po_umolchaniyu: "vpn",
    prilozheniya: [
      { put: "C:\\Program Files\\Mozilla Firefox\\firefox.exe", imya: "firefox.exe", potomki: true, marshrut: "direct" },
      { put: "C:\\Users\\home\\AppData\\Local\\Steam\\steam.exe", imya: "steam.exe", potomki: true, marshrut: "direct" },
      { put: "C:\\Program Files\\qBittorrent\\qbittorrent.exe", imya: "qbittorrent.exe", potomki: false, marshrut: "vpn" },
    ],
    domeny: [
      { domen: "gosuslugi.ru", marshrut: "direct" },
      { domen: "rutracker.org", marshrut: "vpn" },
    ],
    // Явные маршруты сервисов: два ведут мимо туннеля, счётчик вкладки
    // показывает шесть из восьми.
    servisy: [
      { id: "telegram", marshrut: "direct" },
      { id: "spotify", marshrut: "direct" },
    ],
  },
  // Каталог повторяет встроенный (internal/katalog/servisy.json): те же
  // восемь сервисов и те же домены, иначе снимок врёт о содержимом выпуска.
  katalog: {
    versiya: "2026.09.08",
    istochnik: "https://iplist.opencck.org/ru/",
    servisy: [
      { id: "youtube", imya: "YouTube", domeny: ["youtube.com", "youtu.be", "googlevideo.com"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "discord", imya: "Discord", domeny: ["discord.com", "discord.gg", "discord.media"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "chatgpt", imya: "ChatGPT", domeny: ["chatgpt.com", "openai.com", "oaistatic.com"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "instagram", imya: "Instagram", domeny: ["instagram.com", "cdninstagram.com"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "claude", imya: "Claude", domeny: ["claude.ai", "claude.com", "anthropic.com"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "telegram", imya: "Telegram", domeny: ["telegram.org", "telegram.me", "t.me"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "spotify", imya: "Spotify", domeny: ["spotify.com", "scdn.co", "spotifycdn.com"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "soundcloud", imya: "SoundCloud", domeny: ["soundcloud.com", "sndcdn.com", "snd.sc"], istochnik: "https://github.com/rekryt/iplist/" },
    ],
  },
};

// Замеры приходят ответом на measureDelays, то есть на нажатие «Проверить
// серверы»: съёмка нажимает её сама. Два числа про разное, см. ZamerZaderzhki.
const zamery = {
  zamery: [
    { id: "nl", realping_ms: 38, tcping_ms: 28 },
    { id: "de", realping_ms: 57, tcping_ms: 41 },
    { id: "fi", realping_ms: 49, tcping_ms: 36 },
  ],
};

function telo(imya: string): unknown {
  switch (imya) {
    case "status":
    case "connect":
    case "disconnect":
    case "setServer":
    case "setRouteMode":
      return status;
    case "hello":
      return { pozzhe: {} };
    case "listServers":
      return { servery, vybran: "nl", podpiska_zadana: true, podpiska_uzel: "panel.example" };
    case "listRules":
      return pravila;
    case "listSubscriptions":
      return { podpiski: [{ id: "osn", imya: "panel.example", aktivnaya: true, serverov: 4 }] };
    case "measureDelays":
      return zamery;
    default:
      return {};
  }
}

type Obrabotchik = (sobytie: { data: unknown }) => void;
const podpischiki = new Map<string, Obrabotchik[]>();

function izvestit(imya: string, data: unknown): void {
  for (const o of podpischiki.get(imya) ?? []) o({ data });
}

export const Call = {
  async ByName(imya: string, ...args: unknown[]): Promise<unknown> {
    if (imya === "main.most.SluzhbaUstanovlena") return true;
    if (imya === "main.most.Zvat") {
      const komanda = String(args[0]);
      return JSON.stringify({ tip: "otvet", id: 1, imya: komanda, telo: telo(komanda) });
    }
    return null;
  },
};

export const Events = {
  On(imya: string, o: Obrabotchik): () => void {
    const bylo = podpischiki.get(imya) ?? [];
    podpischiki.set(imya, [...bylo, o]);
    return () => podpischiki.set(imya, (podpischiki.get(imya) ?? []).filter((x) => x !== o));
  },
};

export const Window = { Minimise: async () => {}, Close: async () => {} };
export const Browser = { OpenURL: async () => {} };
export const Clipboard = { Text: async () => "" };

// Экран выбирается параметром адреса, вкладку окно берёт из события трея.
// Цифры под сферой приходят тем же путём, что и в жизни: событием kanal, где
// data это СТРОКА JSON, а не объект (см. naSobytie в most.ts).
const kadrStats = JSON.stringify({
  tip: "sobytie", id: 0, imya: "stats",
  telo: { adres_vyhoda: "203.0.113.24", zaderzhka_ms: 38, prinyato: 2_362_232_012, otdano: 184_090_624 },
});

function podat(): void {
  const ekran = new URLSearchParams(location.search).get("ekran") ?? "podklyuchenie";
  izvestit("okno", true);
  izvestit("vkladka", ekran);
  izvestit("kanal", kadrStats);
}

// Несколько раз подряд: подписки окна встают в useEffect, и одна подача в
// заранее выбранный момент попадала мимо них.
for (const kogda of [80, 250, 600, 1200]) setTimeout(podat, kogda);
setInterval(() => izvestit("kanal", kadrStats), 1000);
