import { describe, expect, it } from "vitest";
import { TEKST_NICHEGO_NE_OSTALOS, ponyatno, tekstOshibki } from "./ponyatno";

// Тексты из разбора 01.10.2026: ровно то, что окно показывало человеку.
describe("ponyatno", () => {
  it.each([
    ["Новое окно не открылось: новый файл окна не найден: CreateFile C:\\x\\affory-ui.exe: The system cannot find the file specified.",
      "Новое окно не открылось: новый файл окна не найден"],
    ["exportProfile: файл не записан: open D:\\profil.affory: Access is denied.", "файл не записан"],
    ["Не удалось прочитать правила: канал не открылся: open \\\\.\\pipe\\affory-v1: The system cannot find the file specified.",
      "Не удалось прочитать правила: канал не открылся"],
    ["no-admin: служба не запущена", "служба не запущена"],
    ["папка журналов недоступна: CreateFile C:\\ProgramData\\Affory\\log: Access is denied.", "папка журналов недоступна"],
    // Путь не перевешивает русскую фразу.
    ["порт 9090 занят процессом C:\\Program Files\\NekoBox Portable Edition\\nekobox_core.exe, а не ядром sing-box.exe",
      "порт 9090 занят процессом C:\\Program Files\\NekoBox Portable Edition\\nekobox_core.exe, а не ядром sing-box.exe"],
    // Адреса после нашей фразы остаются.
    ["имя не разрешилось: a.example.com, b.example.net", "имя не разрешилось: a.example.com, b.example.net"],
    ["Cannot read properties of null (reading 'x')", ""],
    ["VPN не понёс трафик за 2 попытки по 15s", "VPN не понёс трафик за 2 попытки по 15s"],
  ])("%s", (vhod, zhdyom) => {
    expect(ponyatno(vhod)).toBe(zhdyom);
  });
});

describe("tekstOshibki", () => {
  it("не бывает пустым", () => {
    expect(tekstOshibki(new Error("unexpected end of JSON input"))).toBe(TEKST_NICHEGO_NE_OSTALOS);
    expect(tekstOshibki("строка не разобрана: invalid character")).toBe("строка не разобрана");
  });
});
