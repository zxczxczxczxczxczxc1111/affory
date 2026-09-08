// Spec §9.1, one entry per refusal code. The action column is a contract:
// internal/kachestvo/otkazy_ui_test.go parses this file (one entry per line,
// `deystvie` first) and compares it with the spec both ways. Text is checked
// by Otkaz.test.tsx: dry, no "we", no trailing dot.

/** Verbatim the ten-word vocabulary of internal/kachestvo/deystviya_test.go. */
export type Deystvie =
  | "povtorit"
  | "perepodklyuchitsya"
  | "otkryt-servery"
  | "obnovit-podpisku"
  | "obnovit-programmu"
  | "postavit-sluzhbu"
  | "zaprosit-prava"
  | "proverit-set"
  | "pokazat-vinovnika"
  | "nichego";

export interface ZapisOtkaza {
  deystvie: Deystvie;
  tekst: string;
}

export const tekstOtkaza: Record<string, ZapisOtkaza> = {
  "dns-resolve-failed": { deystvie: "povtorit", tekst: "не удалось найти сервер по имени" },
  "server-auth-failed": { deystvie: "obnovit-podpisku", tekst: "сервер не принял ключ, проверь подписку" },
  "subscription-unreachable": { deystvie: "obnovit-podpisku", tekst: "подписка недоступна, работают серверы из прошлого обновления" },
  "subscription-malformed": { deystvie: "obnovit-podpisku", tekst: "подписка отдала непонятное" },
  "subscription-expired": { deystvie: "obnovit-podpisku", tekst: "подписка отдала сообщение вместо серверов" },
  "selected-server-gone": { deystvie: "otkryt-servery", tekst: "выбранного сервера больше нет в подписке" },
  "tun-create-failed": { deystvie: "povtorit", tekst: "не удалось создать адаптер" },
  "core-not-responding": { deystvie: "povtorit", tekst: "ядро не отвечает по управляющему порту" },
  "wintun-missing": { deystvie: "postavit-sluzhbu", tekst: "драйвер адаптера не установился" },
  "no-admin": { deystvie: "postavit-sluzhbu", tekst: "служба не установлена или не отвечает" },
  "admin-required": { deystvie: "zaprosit-prava", tekst: "эта команда только для администратора машины" },
  "firewall-disabled": { deystvie: "nichego", tekst: "режим «весь трафик» недоступен: брандмауэр выключен" },
  "firewall-failed": { deystvie: "povtorit", tekst: "не удалось поставить защиту" },
  "killswitch-orphan": { deystvie: "nichego", tekst: "остались правила защиты без туннеля, сняты" },
  "health-snapshot-stale": { deystvie: "nichego", tekst: "снимку состояния сервера больше суток" },
  "health-snapshot-missing": { deystvie: "nichego", tekst: "снимка состояния сервера нет" },
  "update-rollback": { deystvie: "nichego", tekst: "обновление откатилось, работает прежняя версия" },
  "pipe-squatted": { deystvie: "pokazat-vinovnika", tekst: "канал управления занят другой программой" },
  "protocol-mismatch": { deystvie: "obnovit-programmu", tekst: "служба и программа разных версий, обнови программу" },
  "foreign-registry-hijack": { deystvie: "pokazat-vinovnika", tekst: "настройки прокси меняет другая программа" },
  "secrets-unreadable": { deystvie: "otkryt-servery", tekst: "прежние серверы не читаются на этой машине" },
  "foreign-proxy-hijack": { deystvie: "pokazat-vinovnika", tekst: "мешает другая программа" },
  "all-servers-down": { deystvie: "proverit-set", tekst: "ни один сервер не отвечает" },
  "name-resolve-failed": { deystvie: "otkryt-servery", tekst: "эти адреса не разрешились и исключены" },
  "tunnel-not-carrying": { deystvie: "povtorit", tekst: "туннель перестал нести трафик" },
  "server-rejected-by-core": { deystvie: "otkryt-servery", tekst: "ядро не приняло сервер" },
  "candidate-in-use": { deystvie: "nichego", tekst: "сервер занят подключением, сначала отключись" },
  "rule-invalid": { deystvie: "nichego", tekst: "правило не принято: путь не ведёт к файлу или домен не похож на имя" },
  "switch-needs-reconnect": { deystvie: "perepodklyuchitsya", tekst: "переподключись, чтобы выбрать этот сервер" },
  "switch-target-not-carrying": { deystvie: "otkryt-servery", tekst: "этот сервер не отвечает, выбери другой" },
  "switch-failed": { deystvie: "povtorit", tekst: "переключиться не удалось, попробуй ещё раз" },
  "journal-clear-failed": { deystvie: "povtorit", tekst: "файл журнала не стёрся, закрой программу, которая его читает" },
  "exit-ip-unmeasured": { deystvie: "povtorit", tekst: "адрес выхода не измерен: эндпоинт не ответил или локальный прокси не поднят" },
  "bandwidth-unmeasured": { deystvie: "povtorit", tekst: "полоса не измерена: мишень не задана, не отвечает или замер прерван" },
  "request-invalid": { deystvie: "povtorit", tekst: "команда пришла в негодном виде: повтори действие" },
  "update-archive-invalid": { deystvie: "nichego", tekst: "архив сборки не принят: хеш не совпал или состав не тот" },
  "update-rollback-failed": { deystvie: "nichego", tekst: "обновление откатилось, но служба не отвечает: переустановите из установщика" },
  "update-check-failed": { deystvie: "povtorit", tekst: "сервер обновлений не ответил, проверить позже" },
  "update-download-failed": { deystvie: "povtorit", tekst: "обновление не скачалось или хеш не совпал, работает прежняя версия" },
  "internal-error": { deystvie: "povtorit", tekst: "команда не выполнена: внутренняя ошибка службы" },
};

/** Button label per action; null means the screen only informs. */
export const podpisDeystviya: Record<Deystvie, string | null> = {
  povtorit: "Повторить",
  perepodklyuchitsya: "Переподключиться",
  "otkryt-servery": "Открыть серверы",
  "obnovit-podpisku": "Обновить подписку",
  "obnovit-programmu": "Обновить программу",
  "postavit-sluzhbu": "Установить службу",
  "zaprosit-prava": "Повторить от администратора",
  "proverit-set": "Проверить сеть",
  "pokazat-vinovnika": null,
  nichego: null,
};

/** Codes whose server-side text replaces ours verbatim (§9.1: the panel's
 *  message, not our wording on top). */
export const TEKST_SLUZHBY_DOSLOVNO = new Set(["subscription-expired"]);
