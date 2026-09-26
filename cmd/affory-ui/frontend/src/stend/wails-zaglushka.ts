// Подставной рантайм Wails: съёмка снимков README и обход интерфейса.
//
// Подменяется именно @wailsio/runtime, а не most.ts: мост тогда работает ровно
// тем кодом, что уезжает в выпуск, и снимок показывает настоящий экран, а не
// его копию. Alias живёт в vite.stend.config.ts.
//
// Данные подставные и намеренно узнаваемые: адреса из RFC 5737, ключей нет
// нигде. Настоящие адреса в кадр не попадают никогда.
//
// 19.09.2026 заглушка перестала быть статичной. До этого `connect` и
// `disconnect` возвращали один и тот же `podnyat`, то есть окно нельзя было
// провести ни по одному переходу: нажатие ничего не меняло. Теперь состояние
// живёт здесь и команды его двигают, а СЛУЧАЙ задаётся параметром адреса:
//
//     ?sluchay=podnyat      туннель поднят (по умолчанию)
//     ?sluchay=vyklyuchen   выключен, ждёт нажатия
//     ?sluchay=molchit      служба не отвечает
//     ?sluchay=otkaz        подъём не удался, окно показывает объяснение
//     ?sluchay=pusto        подписки нет, список серверов пуст
//     ?sluchay=novyy        службы нет вовсе: экран первого запуска
//     ?sluchay=novyy-otkaz  тот же экран, но установку отклонили
//
// Три последних в госте не воспроизводятся без нарочной поломки продукта, и
// именно поэтому экраны отказов не проверял никто.

// Подставляется конфигом стенда из файла VERSIYA в корне дерева. Числом здесь
// он уже стоял и уже соврал: снимки README показывали 1.2.0 в дни 1.3.2.
declare const __VERSIYA_STENDA__: string;
const VERSIYA = typeof __VERSIYA_STENDA__ === "string" ? __VERSIYA_STENDA__ : "dev";

function parametr(imya: string, poumolchaniyu: string): string {
  try {
    return new URLSearchParams(location.search).get(imya) ?? poumolchaniyu;
  } catch {
    return poumolchaniyu;
  }
}

import type { Server } from "../protokol";

const SLUCHAY = parametr("sluchay", "podnyat");

/** Первый запуск: службы в системе нет, канал молчит, и единственное, что
 *  делает окно, это кнопка установки. Флаг живой - подставная установка его
 *  снимает, и окно само уходит на обычный экран, как в жизни после UAC. */
let sluzhbaEst = SLUCHAY !== "novyy" && SLUCHAY !== "novyy-otkaz";

/** Номер выпуска, который находит подставная проверка: следующий минор от своей
 *  версии. Числом здесь стоять нельзя - оно разошлось бы с VERSIYA ровно так,
 *  как уже разошлись снимки README, и стенд предлагал бы обновиться назад. */
function sleduyushchayaVersiya(): string {
  const ch = VERSIYA.split(".").map((n) => Number.parseInt(n, 10));
  if (ch.length !== 3 || ch.some((n) => !Number.isFinite(n))) return "1.0.0";
  return `${ch[0]}.${ch[1] + 1}.0`;
}

// Тип берётся у протокола, а не выводится: без него поле, которого нет ни у
// одной записи образца (vne_avto), не присвоить, а образец врал бы формой
// ответа службы.
const osnovnyeServery: Server[] = [
  { id: "nl", imya: "Нидерланды · Амстердам", transport: "reality-tcp", host: "203.0.113.11", port: 443, iz_podpiski: true },
  // Транспорты пишутся ровно теми словами, какими их называет служба
  // (`izvestnyeTransporty` в internal/genkonfig/vhod.go). До 19.09.2026 здесь
  // стояли «hysteria2», «vless-ws» и «vless-tcp», которых служба не отдаёт
  // никогда, и стенд показывал экран, какого в жизни не бывает.
  { id: "de", imya: "Германия · Франкфурт", transport: "hy2", host: "203.0.113.12", port: 443, iz_podpiski: true, s_pinom: true },
  { id: "fi", imya: "Финляндия · Хельсинки", transport: "ws", host: "203.0.113.13", port: 443, iz_podpiski: true },
  { id: "se", imya: "Швеция · Стокгольм", transport: "anytls", host: "203.0.113.14", port: 443, iz_podpiski: true },
  // tuic стоит в подписке первым и первым же советует справка, поэтому он
  // обязан быть и здесь: без него снимок справки советовал бы протокол,
  // которого на соседнем снимке нет (21.09.2026).
  { id: "pl", imya: "Польша · Варшава", transport: "tuic", host: "203.0.113.16", port: 10443, iz_podpiski: true },
  { id: "svoy", imya: "Свой сервер", transport: "trojan", host: "203.0.113.15", port: 443, iz_podpiski: false },
];

const zapasnyeServery: Server[] = [{ id: "reserve-1", imya: "Запасной сервер", host: "203.0.113.20", port: 443, transport: "trojan", iz_podpiski: true }];
let aktivnayaPodpiska = "osn";
let servery = [...osnovnyeServery];

// Отказ подъёма берётся настоящим кодом и настоящим текстом службы: экран
// разбирает код и рисует по нему свой разговор, и выдуманный код провёл бы
// обход мимо той ветки, которую он же и проверяет.
const OTKAZ = {
  kod: "server-auth-failed",
  tekst: "VPN включился, но данные через него не пошли. Попыток: 2, по 15 секунд каждая. Причина: сервер не принял ключ (код ответа 502)",
};

const status: Record<string, unknown> = {
  sostoyanie: SLUCHAY === "podnyat" ? "podnyat"
    : SLUCHAY === "molchit" ? "sluzhba-molchit"
    : SLUCHAY === "otkaz" ? "ne-neset"
    : "vyklyuchen",
  vybran_id: "nl",
  // Пустой строки тут быть не может: оба поля в протоколе с omitempty, то
  // есть служба их просто НЕ ШЛЁТ, пока никто ничего не несёт. Пустая строка
  // стояла здесь до 23.09.2026 и показывала на выключенном окне «Сервер не
  // выбран» при выбранном сервере - экран, которого в жизни не бывает.
  nesushchiy_id: SLUCHAY === "podnyat" ? "nl" : undefined,
  nesushchiy_imya: SLUCHAY === "podnyat" ? "Нидерланды · Амстердам" : undefined,
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
  ...(SLUCHAY === "otkaz" ? { oshibka: OTKAZ } : {}),
};

/** Переходы состояния. Подъём идёт ЧЕРЕЗ promezhutochnoe «podnimaetsya»:
 *  окно рисует его отдельной подписью, и переход сразу в «podnyat» оставил
 *  бы эту ветку непройденной. Задержка маленькая, обходу хватает. */
function perevesti(komanda: string): void {
  if (SLUCHAY === "molchit") return;
  if (komanda === "connect") {
    status.sostoyanie = "podnimaetsya";
    status.oshibka = undefined;
    setTimeout(() => {
      if (SLUCHAY === "otkaz") {
        status.sostoyanie = "ne-neset";
        status.oshibka = OTKAZ;
      } else {
        status.sostoyanie = "podnyat";
        status.nesushchiy_id = String(status.vybran_id ?? "nl");
        status.nesushchiy_imya = servery.find((s) => s.id === status.vybran_id)?.imya ?? "";
      }
      izvestit("kanal", JSON.stringify({ tip: "sobytie", id: 0, imya: "status", telo: status }));
    }, 300);
  }
  if (komanda === "disconnect") {
    status.sostoyanie = "vyklyuchen";
    status.nesushchiy_id = undefined;
    status.nesushchiy_imya = undefined;
    status.oshibka = undefined;
  }
}

const pravila = {
  // protsessy и domeny верхнего уровня это ПРЕЖНИЙ формат (списки строк), а
  // нынешние правила живут в trafik. Перепутать их значит уронить экран.
  protsessy: [] as string[],
  domeny: [] as string[],
  // Три поля из teloPravil, которых у заглушки не было: экран читает их и
  // решает, показывать ли «правила ждут переподъёма» и выключатель ru-списка.
  trebuet_podyoma: false,
  spisok_izmenyon: false,
  bez_ru_spiska: false,
  trafik: {
    po_umolchaniyu: "vpn",
    prilozheniya: [
      { put: "C:\\Program Files\\Mozilla Firefox\\firefox.exe", imya: "firefox.exe", potomki: true, marshrut: "direct" },
      // Свои программы, а не программы сервисов: правило на steam.exe служба
      // снимает при обновлении, этой программой теперь управляет карточка.
      { put: "C:\\Program Files\\Epic Games\\Fortnite\\FortniteClient-Win64-Shipping.exe", imya: "FortniteClient-Win64-Shipping.exe", potomki: true, marshrut: "direct" },
      { put: "C:\\Program Files\\qBittorrent\\qbittorrent.exe", imya: "qbittorrent.exe", potomki: false, marshrut: "vpn" },
    ],
    domeny: [
      { domen: "gosuslugi.ru", marshrut: "direct" },
      { domen: "reddit.com", marshrut: "vpn" },
    ],
    // Явные маршруты сервисов: два ведут мимо туннеля, счётчик вкладки
    // показывает шесть из восьми.
    servisy: [
      { id: "telegram", marshrut: "direct" },
      { id: "spotify", marshrut: "direct" },
    ],
  },
  // Каталог повторяет встроенный (internal/katalog/servisy.json): те же
  // сервисы, те же домены и те же имена клиентов, иначе снимок врёт о
  // содержимом выпуска. Пять последних живут без доменов: набора в iplist для
  // них нет, и маршрут держится на программе.
  katalog: {
    versiya: "2026.09.08",
    istochnik: "https://iplist.opencck.org/ru/",
    servisy: [
      { id: "youtube", imya: "YouTube", domeny: ["youtube.com", "youtu.be", "googlevideo.com"], programmy: [], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "discord", imya: "Discord", domeny: ["discord.com", "discord.gg", "discord.media"], programmy: ["Discord.exe", "DiscordPTB.exe", "DiscordCanary.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "chatgpt", imya: "ChatGPT", domeny: ["chatgpt.com", "openai.com", "oaistatic.com"], programmy: ["ChatGPT.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "instagram", imya: "Instagram", domeny: ["instagram.com", "cdninstagram.com"], programmy: [], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "claude", imya: "Claude", domeny: ["claude.ai", "claude.com", "anthropic.com"], programmy: ["Claude.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "telegram", imya: "Telegram", domeny: ["telegram.org", "telegram.me", "t.me"], programmy: ["Telegram.exe", "Telegram Desktop.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "spotify", imya: "Spotify", domeny: ["spotify.com", "scdn.co", "spotifycdn.com"], programmy: ["Spotify.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "soundcloud", imya: "SoundCloud", domeny: ["soundcloud.com", "sndcdn.com", "snd.sc"], programmy: ["SoundCloud.exe"], istochnik: "https://github.com/rekryt/iplist/" },
      { id: "steam", imya: "Steam", domeny: [], programmy: ["steam.exe"] },
      { id: "epicgames", imya: "Epic Games", domeny: [], programmy: ["EpicGamesLauncher.exe"] },
      { id: "battlenet", imya: "Battle.net", domeny: [], programmy: ["Battle.net.exe"] },
      { id: "riot", imya: "Riot Client", domeny: [], programmy: ["RiotClientServices.exe"] },
      { id: "ubisoft", imya: "Ubisoft Connect", domeny: [], programmy: ["UbisoftConnect.exe", "upc.exe"] },
    ],
  },
};

// Замеры приходят ответом на measureDelays, то есть на нажатие «Проверить
// серверы»: съёмка нажимает её сама. Два числа про разное, см. ZamerZaderzhki.
const zamery = {
  zamery: [
    // realping БОЛЬШЕ задержки главного экрана, и это не описка: там один круг
    // по готовому соединению, здесь весь запрос вместе с рукопожатием. Числа
    // взяты того же порядка, что живые замеры 12.09.2026.
    { id: "nl", realping_ms: 231, tcping_ms: 28 },
    { id: "de", realping_ms: 104, tcping_ms: 41 },
    { id: "fi", realping_ms: 382, tcping_ms: 36 },
    { id: "pl", realping_ms: 96, tcping_ms: 30 },
  ],
};

/** Снимок замера скорости. Живёт переменной, потому что экран ОПРАШИВАЕТ его
 *  раз в такт: одно и то же тело на все опросы оставило бы замер вечно
 *  идущим, а кнопку «Отмена» вечно нажатой. */
let skorost: Record<string, unknown> = { id: 0, phase: "idle", path: "vpn", provider: "", name: "", attempt: 0 };

function stroka(x: unknown): string {
  return typeof x === "string" ? x : "";
}

/** Тумблер настроек. Ответ это статус, и поле в нём обязано смениться: иначе
 *  обход прочитает неподвижный тумблер как дефект окна. */
function tumbler(pole: string): (vhod: Record<string, unknown>) => unknown {
  return (vhod) => {
    status[pole] = vhod.vkl === true;
    return status;
  };
}

function vybrat(vhod: Record<string, unknown>): unknown {
  const id = stroka(vhod.id) || stroka(vhod.server);
  if (id) {
    status.vybran_id = id;
    if (status.sostoyanie === "podnyat") {
      status.nesushchiy_id = id;
      status.nesushchiy_imya = servery.find((s) => s.id === id)?.imya ?? "";
    }
  }
  return status;
}

// Ответы команд, по одному на каждую команду службы.
//
// Форма ответа повторяет службу поле в поле. Окно разбирает тело и падает
// ЦЕЛИКОМ, если поля нет: первый же прогон обхода 19.09.2026 уронил раздел
// «Настройки» нажатием «Проверить», потому что прежний `default` возвращал
// пустое тело, а экран читает `proverka.punkty.map`. Служба на checkLeaks
// отдаёт четыре пункта ВСЕГДА (internal/set/utechki.go), то есть виноват был
// стенд. Незнакомых команд с тех пор нет молча, см. probely ниже.
const OTVETY: Record<string, (vhod: Record<string, unknown>) => unknown> = {
  status: () => status,
  // Имя поля у connect это `server`, а не `id` (dispetcher.go). Заглушка
  // читала `id`, и выбор сервера при подключении не доезжал никуда.
  connect: (v) => {
    const id = stroka(v.server) || stroka(v.id);
    if (id) { status.vybran_id = id; status.rezhim_marshruta = "ruchnoy"; }
    perevesti("connect");
    return status;
  },
  disconnect: () => {
    perevesti("disconnect");
    return status;
  },
  setServer: vybrat,
  switch: vybrat,
  // Статус ВЛОЖЕН, см. statusIz в App.tsx: плоский статус тут читался бы
  // окном, но разошёлся бы с ответом службы.
  setRouteMode: (v) => {
    if (stroka(v.rezhim)) status.rezhim_marshruta = stroka(v.rezhim);
    return { status, trebuet_podyoma: false };
  },
  hello: () => ({ pozzhe: {} }),

  setKillSwitch: tumbler("kill_switch"),
  setAutostart: tumbler("avtozapusk"),
  setConnectOnStart: tumbler("podklyuchat_pri_starte"),
  setJournal: tumbler("zhurnal"),
  setDiagnostics: tumbler("diagnostika"),
  clearJournal: () => status,
  // Проверка НАХОДИТ выпуск: до 23.09.2026 заглушка отдавала свой же статус, и
  // ни плашку в подвале главного, ни карточку с кнопкой «Установить» обход
  // увидеть не мог - их рисует только находка.
  checkUpdate: () => {
    status.obnovlenie = { versiya: sleduyushchayaVersiya(), razmer: 26934390, provereno: new Date().toISOString() };
    status.obnovlenie_provereno = new Date().toISOString();
    return status;
  },
  downloadUpdate: () => ({ zapushchena: true, srok_s: 90 }),
  installUpdate: () => ({ zapushchena: true, srok_s: 90 }),
  setBandwidth: () => status,
  subscribeStats: (v) => ({ vkl: v.vkl === true }),

  listServers: () => {
    // Пустой набор это не «список из нуля строк», а другой экран целиком:
    // окно предлагает завести подписку. Ветка не гонялась ни разу, потому
    // что в госте подписка есть всегда.
    if (SLUCHAY === "pusto") {
      return { servery: [], vybran: "", podpiska_zadana: false, podpiska_uzel: "" };
    }
    return { servery, versii: Object.fromEntries(servery.map(s => [s.id, `demo-v1-${s.id}`])), vybran: status.vybran_id, podpiska_zadana: true, podpiska_uzel: aktivnayaPodpiska === "osn" ? "panel.example" : "reserve.example" };
  },
  // Область автовыбора (A5). Заглушка правит СВОЙ список и отдаёт его целиком,
  // как служба: без этого меню правой кнопки на стенде нажималось бы вхолостую.
  setAutoMember: (v) => {
    const id = stroka(v.id);
    const srv = servery.find((s) => s.id === id);
    if (srv) srv.vne_avto = v.uchastvuet !== true;
    return { servery, versii: Object.fromEntries(servery.map(s => [s.id, `demo-v1-${s.id}`])), vybran: status.vybran_id, podpiska_zadana: true, podpiska_uzel: aktivnayaPodpiska === "osn" ? "panel.example" : "reserve.example" };
  },
  addServer: () => ({ server: servery[servery.length - 1] }),
  // Пачка (26.09.2026): стенд считает строки, а не разбирает их. Строка без
  // «://» идёт в отказ, чтобы итог с пропущенными было видно на снимке.
  addServers: (v) => {
    const stroki = stroka(v.tekst).split(/\r\n|\r|\n/);
    const otkazy = stroki
      .map((s, i) => ({ s: s.trim(), stroka: i + 1 }))
      .filter(({ s }) => s !== "" && !s.toLowerCase().startsWith("http") && !s.includes("://"))
      .map(({ stroka: n }) => ({ stroka: n, prichina: "ссылка не разобрана: нет схемы" }));
    const dobavleno = stroki.filter((s) => s.includes("://") && !s.trim().toLowerCase().startsWith("http")).length;
    return { dobavleno, obnovleno: 0, uzhe_bylo: 0, otkazy };
  },
  exportServers: () => {
    const tekst = servery.map((s) => `hy2://demo-parol@${s.host}:${s.port}?sni=demo.example#${encodeURIComponent(s.imya)}`).join("\n");
    return { tekst, base64: btoa(String.fromCharCode(...new TextEncoder().encode(tekst))), vsego: servery.length, propushcheny: [] };
  },
  removeServer: () => ({ ostalos: servery.length }),
  listRules: () => pravila,
  inspectApplication: (v) => {
    const root=pravila.trafik.prilozheniya.find(a=>a.put.toLowerCase()===stroka(v.put).toLowerCase());
    if(!root)throw new Error("Правило приложения изменилось. Обнови список правил");
    const launcher=root.put.toLowerCase().includes("steam");
    const child=launcher?"C:\\Games\\Demo\\game.exe":root.put;
    const winner=pravila.trafik.prilozheniya.find(a=>a.put.toLowerCase()===child.toLowerCase()) || root;
    const included=!launcher || root.potomki;
    return {pravilo:root,reviziya_pravil:"demo",trebuet_podyoma:false,vremya:new Date().toISOString(),fayl:root.put.includes("Missing")?"net":"est",samo:launcher?0:1,vsego:included?1:0,
      zapushchennye:included?[{pid:24680,created:"134000000000000001",put:child,imya:child.split("\\").pop(),cherez:launcher?[root.put,child]:[root.put],marshrut:winner.marshrut,pravilo_put:winner.put,pravilo_imya:winner.imya,pereopredelen:winner.put!==root.put}]:[],
      neizvestno:1,neizvestnye:["C:\\Apps\\Separate.exe"],ogranichen:false};
  },
  // Пустой список стоял здесь до 23.09.2026, и проверка охвата всегда падала
  // в ветку «подходящих соединений не обнаружено»: сам список соединений в
  // окне не видел никто. Фильтр учитывается, иначе проба любого домена
  // показывала бы чужие строки.
  listConnections: (v) => {
    const domen = stroka(v.domen).toLowerCase();
    const put = stroka(v.put).toLowerCase();
    // Значения ровно той формы, какую отдаёт clash_api: выход это первое
    // звено цепочки («srv-nl», «direct»), правило это строка sing-box вида
    // «domain_suffix=x => srv-nl», путь процесса полный. Своя форма показала
    // бы «не удалось определить» на каждой строке.
    const hrom = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe";
    const vse = [
      { Id: "1", Host: "youtube.com", Adres: "203.0.113.60", Port: 443, Protsess: hrom, Vyhod: "srv-nl", Pravilo: "rule_set=service-youtube => srv-nl", Nachalo: new Date(Date.now() - 42 * 1000).toISOString() },
      { Id: "2", Host: "rr3---sn-5hne6nsy.googlevideo.com", Adres: "203.0.113.61", Port: 443, Protsess: hrom, Vyhod: "srv-nl", Pravilo: "domain_suffix=googlevideo.com => srv-nl", Nachalo: new Date(Date.now() - 9 * 1000).toISOString() },
      { Id: "3", Host: "gosuslugi.ru", Adres: "198.51.100.7", Port: 443, Protsess: hrom, Vyhod: "direct", Pravilo: "domain_suffix=gosuslugi.ru => direct", Nachalo: new Date(Date.now() - 4 * 60 * 1000).toISOString() },
      { Id: "4", Host: "", Adres: "203.0.113.62", Port: 443, Protsess: "C:\\Users\\home\\AppData\\Local\\Discord\\app-1.0.9187\\Discord.exe", Vyhod: "srv-nl", Pravilo: "final => vybor", Nachalo: new Date(Date.now() - 77 * 1000).toISOString() },
    ];
    const podhodit = (s: typeof vse[number]) => {
      if (domen) return s.Host === domen || s.Host.endsWith(`.${domen}`);
      if (put) return put.endsWith(`\\${s.Protsess.toLowerCase()}`);
      return true;
    };
    return { yadro: status.sostoyanie === "podnyat", vremya: new Date().toISOString(), ogranichen: false, soedineniya: vse.filter(podhodit) };
  },
  setRules: (v) => {
    if (v.trafik && typeof v.trafik === "object") {
      pravila.trafik = { ...pravila.trafik, ...(v.trafik as Record<string, unknown>) } as typeof pravila.trafik;
    }
    return pravila;
  },
  listSubscriptions: () => ({ podpiski: SLUCHAY === "pusto" ? [] : [
    { id: "osn", uzel: "panel.example", aktivnaya: aktivnayaPodpiska === "osn", serverov: 5, servery: osnovnyeServery.filter(s => s.iz_podpiski), obnovlena: new Date(Date.now()-50*60000).toISOString() },
    { id: "reserve", uzel: "reserve.example", aktivnaya: aktivnayaPodpiska === "reserve", serverov: 1, servery: zapasnyeServery },
  ] }),
  addSubscription: (v) => ({ id: stroka(v.id) || "osn", aktivnaya: true, serverov: servery.length, otkazy: [] }),
  removeSubscription: (v) => ({ udalena: stroka(v.id) }),
  setActiveSubscription: (v) => {
    aktivnayaPodpiska = stroka(v.id) || "osn";
    servery = aktivnayaPodpiska === "osn" ? [...osnovnyeServery] : [...zapasnyeServery, ...osnovnyeServery.filter(s => !s.iz_podpiski)];
    return { aktivnaya: aktivnayaPodpiska, serverov: servery.length, otkazy: [] };
  },
  setSubscription: () => ({ zadana: true, serverov: servery.length, otkazy: [] }),
  refreshSubscription: () => ({ serverov: servery.length, otkazy: [] }),

  measureDelays: () => ({ zamery: zamery.zamery.filter(z => servery.some(s => s.id === z.id)).map(z => ({ ...z, versiya: `demo-v1-${z.id}` })) }),
  measureBandwidth: () => ({
    mbit_vniz: 318.4, bayt_vniz: 2_140_000_000, potokov: 4, sovet_vniz: 286,
    cherez_tunnel: true,
    mbit_vverh: 96.2, bayt_vverh: 620_000_000, sovet_vverh: 86,
  }),
  getServerHealth: () => ({
    vremya: Math.floor(Date.now() / 1000) - 240, vozrast_s: 240, ustarel: false, trevogi: [],
    dney_do_konca_sertifikata_maski: 74, dney_s_obnovleniya_xray: 12,
    hy2_aktiven: true, dney_do_konca_sertifikata_hy2: 74,
  }),
  checkExitIp: () => ({ adres: "203.0.113.24", cherez: "tunnel" }),
  // Четыре пункта теми же словами и теми же итогами, что у службы: экран
  // разбирает `itog` и рисует по нему три разных строки.
  checkLeaks: () => ({
    adres_vyhoda: "203.0.113.24",
    punkty: [
      { imya: "адрес выхода", itog: "ok", tekst: "через VPN виден 203.0.113.24, это адрес сервера" },
      { imya: "IPv6", itog: "ok", tekst: "исходящий IPv6 закрыт правилом брандмауэра, мимо VPN по нему не уйти" },
      { imya: "поиск сайтов по имени", itog: "ne_vidim", tekst: "эти запросы забирает на себя VPN. Куда они уходят дальше, программа со своей стороны увидеть не может: это проверяется отдельно, на испытательной машине" },
      { imya: "браузер со своим поиском адресов (DoH)", itog: "ne_vidim", tekst: "такой браузер ищет адреса сам, своим DoH, и общую настройку обходит. Правила по сайтам на него не действуют, и эта проверка его тоже не видит" },
    ],
  }),
  // Пять слоёв теми же видами и подписями, что у службы (proverka_seti.go).
  // Один слой СЛОМАН намеренно: список, где всё зелено, не показывает, как
  // окно рисует отказ, и обход проходил бы мимо этой половины разметки.
  checkNetwork: () => ({
    vremya: new Date().toISOString(),
    sloi: [
      { vid: "tunnel", podpis: "VPN на этом компьютере", proshlo: true, podrobno: "Affory, адрес 172.19.0.1", ms: 2 },
      { vid: "yadro", podpis: "Связь с сервером", proshlo: true, podrobno: "ответ за 41 мс", ms: 41 },
      { vid: "imya", podpis: "Имена сайтов через VPN", proshlo: true, podrobno: "cp.cloudflare.com это 104.16.132.229", ms: 28 },
      { vid: "mestnyy-dns", podpis: "Имена сайтов мимо VPN", proshlo: false, podrobno: "yandex.ru не находится по имени: нет ответа за отведённый срок; через 192.168.0.1 открываются российские сайты и всё, что идёт мимо VPN", ms: 3001 },
      { vid: "udp", podpis: "Голос и видео", proshlo: true, podrobno: "получено 10 из 10, среднее 24 мс, разброс 6 мс", ms: 1140 },
    ],
  }),
  startSpeedTest: (v) => {
    skorost = { id: Date.now(), phase: "download", path: "vpn", provider: stroka(v.provider) || "librespeed", name: "LibreSpeed · Helsinki", attempt: 1 };
    setTimeout(() => {
      skorost = {
        ...skorost, phase: "complete",
        result: { name: "LibreSpeed · Helsinki", download_mbps: 284.6, upload_mbps: 92.1, attempts: [{ name: "LibreSpeed · Helsinki" }] },
      };
    }, 600);
    return skorost;
  },
  speedTestStatus: () => skorost,
  cancelSpeedTest: () => {
    skorost = { ...skorost, phase: "cancelled", reason: "замер отменён" };
    return skorost;
  },

  exportProfile: () => ({ profil: "QUZGT1JZLVBST0ZJTA==" }),
  importProfile: () => ({ prinyato: true }),
};

/** Команды, которых заглушка не знает.
 *
 *  Пустой ответ на такую команду однажды уронил окно и выглядел дефектом
 *  продукта. Теперь незнакомая команда получает ОТКАЗ - окно рисует его
 *  штатно, тем же путём, что отказ службы, - а её имя остаётся здесь. Обход
 *  читает список в конце и объявляет НЕГОДЕН стенда, а не провал продукта:
 *  ровно то разделение, ради которого затевался переход с UIA. */
const probely: string[] = [];
(globalThis as unknown as Record<string, unknown>).__probelyStenda = probely;

type Obrabotchik = (sobytie: { data: unknown }) => void;
const podpischiki = new Map<string, Obrabotchik[]>();

function izvestit(imya: string, data: unknown): void {
  for (const o of podpischiki.get(imya) ?? []) o({ data });
}

// Пути настоящего вида: у Discord в нём номер сборки, у Steam диск установки.
// Ровно поэтому путь и не угадывается в самом окне.
const ZAPUSHCHENNYE = [
  { imya: "Discord", put: "C:\\Users\\home\\AppData\\Local\\Discord\\app-1.0.9187\\Discord.exe" },
  { imya: "Steam", put: "C:\\Program Files (x86)\\Steam\\steam.exe" },
  { imya: "Telegram", put: "C:\\Users\\home\\AppData\\Roaming\\Telegram Desktop\\Telegram.exe" },
  { imya: "Браузер", put: "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe" },
];

export const Call = {
  async ByName(imya: string, ...args: unknown[]): Promise<unknown> {
    if (imya === "main.most.VybratPrilozhenie") throw new Error("В браузерном стенде выбор файла недоступен. В установленном Affory откроется окно выбора приложения.");
    if (imya === "main.most.OtkrytPapkuZhurnalov") throw new Error("В браузерном стенде Проводник недоступен. В установленном Affory кнопка открывает папку журналов.");
    if (imya === "main.most.SluzhbaUstanovlena") return sluzhbaEst;
    if (imya === "main.most.UstanovitSluzhbu") {
      if (SLUCHAY === "novyy-otkaz") throw new Error("права не выданы: запрос отклонён");
      // Служба появляется не мгновенно: окно должно успеть показать полосу
      // хода, иначе эта ветка так и останется непройденной.
      setTimeout(() => { sluzhbaEst = true; }, 4000);
      return null;
    }
    // Запущенные программы стенда. Без них карточки сервисов с клиентом
    // выглядели бы «не запущено» на всех снимках, а у Steam маршрут вовсе
    // нельзя было бы выбрать: путь берётся среди запущенных.
    if (imya === "main.most.SpisokProtsessov") return ZAPUSHCHENNYE;
    // Настоящий код рисует оболочка на Go (qr_vyvod.go), в браузере её нет.
    // Стенду нужен только размер и место картинки, поэтому узор, а не QR.
    if (imya === "main.most.KodyQr") {
      const uzor = "<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 29 29'><rect width='29' height='29' fill='white'/><path d='M4 4h7v7H4zM18 4h7v7h-7zM4 18h7v7H4zM13 13h3v3h-3zM18 18h2v2h-2zM22 20h3v3h-3z' fill='black'/></svg>";
      return ["data:image/svg+xml;utf8," + encodeURIComponent(uzor)];
    }
    if (imya === "main.most.Zvat") {
      // Канала нет, пока нет службы: окно ловит это исключением и спрашивает
      // диспетчер, установлена ли она вообще.
      if (!sluzhbaEst) throw new Error("канал недоступен: нет службы");
      const komanda = String(args[0]);
      // Тело запроса приезжает вторым доводом строкой JSON (см. most.ts).
      // Раньше заглушка его выбрасывала, и выбор сервера не доезжал никуда:
      // окно щёлкало по списку, а стенд показывал прежний.
      let vhod: Record<string, unknown> = {};
      try { vhod = JSON.parse(String(args[1] ?? "{}")) as Record<string, unknown>; } catch { vhod = {}; }
      const otvetchik = OTVETY[komanda];
      if (!otvetchik) {
        if (!probely.includes(komanda)) probely.push(komanda);
        return JSON.stringify({
          tip: "otvet", id: 1, imya: komanda,
          oshibka: { kod: "stend-ne-znaet-komandu", tekst: `подставной мост не знает команду ${komanda}` },
        });
      }
      return JSON.stringify({ tip: "otvet", id: 1, imya: komanda, telo: otvetchik(vhod) });
    }
    return null;
  },
};

export const Events = {
  On(imya: string, o: Obrabotchik): () => void {
    const bylo = podpischiki.get(imya) ?? [];
    podpischiki.set(imya, [...bylo, o]);
    // Стартовое значение подаётся В МОМЕНТ ПОДПИСКИ, а не по таймеру.
    //
    // Раньше подача шла четырежды - на 80, 250, 600 и 1200 мс, чтобы точно
    // попасть после useEffect окна. Для снимков это работало, а обход ломало
    // насмерть: человек жмёт «Правила» на 400-й миллисекунде, а на 600-й
    // заглушка возвращает экран на стартовый, и вкладка «не открывается».
    // Разбор 19.09.2026 занял бы куда больше, если бы Playwright не показал
    // дерево на момент отказа: там стояло «tab Подключение [selected]».
    //
    // Подписчик виден прямо здесь, поэтому гадать про сроки незачем.
    if (!bylo.length) queueMicrotask(() => podat(imya, o));
    return () => podpischiki.set(imya, (podpischiki.get(imya) ?? []).filter((x) => x !== o));
  },
};

// ToggleMaximise зовёт most.ts при нажатии «Развернуть». Без него нажатие
// роняло бы окно исключением, а не разворачивало его.
export const Window = { Minimise: async () => {}, ToggleMaximise: async () => {}, Close: async () => {} };
export const Browser = { OpenURL: async () => {} };
export const Clipboard = { Text: async () => "", SetText: async (_tekst: string) => {} };

// Экран выбирается параметром адреса, вкладку окно берёт из события трея.
// Цифры под сферой приходят тем же путём, что и в жизни: событием kanal, где
// data это СТРОКА JSON, а не объект (см. naSobytie в most.ts).
const kadrStats = JSON.stringify({
  tip: "sobytie", id: 0, imya: "stats",
  telo: { adres_vyhoda: "203.0.113.24", zaderzhka_ms: 38, prinyato: 2_362_232_012, otdano: 184_090_624 },
});

/** Что подать подписчику события `imya` сразу при подписке. */
function podat(imya: string, o: Obrabotchik): void {
  if (imya === "okno") o({ data: true });
  if (imya === "vkladka") o({ data: parametr("ekran", "podklyuchenie") });
  if (imya === "kanal") o({ data: kadrStats });
}

// Цифры под сферой идут потоком, как в жизни. Вкладка и признак окна
// потоком НЕ идут: они подаются один раз при подписке, см. Events.On.
setInterval(() => izvestit("kanal", kadrStats), 1000);
