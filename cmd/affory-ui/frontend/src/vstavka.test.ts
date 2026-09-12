import { afterEach, describe, expect, it, vi } from "vitest";
import { naladitVstavku, SROK_RODNOY_VSTAVKI_MS } from "./vstavka";

// Ctrl+V в поле подписки не вставлял ничего: помогало только контекстное меню.
// Wails ставит WebView2 настройку AreBrowserAcceleratorKeysEnabled(false)
// безусловно (webview_window_windows.go), опции окна её не переопределяют, и
// на какой машине акселератор доживёт до страницы, предсказать нельзя. Поэтому
// запасной путь: не пришло родное событие paste, читаем буфер сами.

function pole(): HTMLInputElement {
  const el = document.createElement("input");
  document.body.appendChild(el);
  el.focus();
  return el;
}

function nazhatCtrlV(el: HTMLElement) {
  // code, а не key: в русской раскладке key приходит 'м', и проверка по 'v'
  // молча не совпадает. Клавиша физически та же.
  el.dispatchEvent(new KeyboardEvent("keydown", { code: "KeyV", ctrlKey: true, bubbles: true }));
}

let snyat: (() => void) | null = null;
function naladit(chitatBufer: () => Promise<string>) {
  snyat = naladitVstavku(chitatBufer);
}

// Отписка в afterEach, а не в конце теста: упавший тест до своей строки не
// доходит, слушатель остаётся жить и вставляет уже в следующий тест. Один раз
// так и вышло, и красным выглядел не тот тест, который сломан.
afterEach(() => {
  snyat?.();
  snyat = null;
  document.body.innerHTML = "";
  vi.useRealTimers();
});

describe("запасная вставка", () => {
  it("вставляет из буфера, когда родного paste не случилось", async () => {
    naladit(async () => "https://panel.example.net/sub");
    const el = pole();
    const vvod = vi.fn();
    el.addEventListener("input", vvod);

    nazhatCtrlV(el);
    await new Promise((r) => setTimeout(r, SROK_RODNOY_VSTAVKI_MS + 20));

    expect(el.value).toBe("https://panel.example.net/sub");
    expect(vvod).toHaveBeenCalled();
  });

  it("молчит, когда родная вставка сработала", async () => {
    const chitat = vi.fn(async () => "https://panel.example.net/sub");
    naladit(chitat);
    const el = pole();

    nazhatCtrlV(el);
    el.dispatchEvent(new Event("paste", { bubbles: true }));
    await new Promise((r) => setTimeout(r, SROK_RODNOY_VSTAVKI_MS + 20));

    expect(chitat).not.toHaveBeenCalled();
    expect(el.value).toBe("");
  });

  it("вставляет в позицию курсора, а не затирает строку", async () => {
    naladit(async () => "СРЕДИНА");
    const el = pole();
    el.value = "начало-конец";
    el.setSelectionRange(7, 7);

    nazhatCtrlV(el);
    await new Promise((r) => setTimeout(r, SROK_RODNOY_VSTAVKI_MS + 20));

    expect(el.value).toBe("начало-СРЕДИНАконец");
    expect(el.selectionStart).toBe(7 + "СРЕДИНА".length);
  });

  it("выделенный кусок заменяется, а не дополняется", async () => {
    naladit(async () => "новое");
    const el = pole();
    el.value = "старое значение";
    el.setSelectionRange(0, 6);

    nazhatCtrlV(el);
    await new Promise((r) => setTimeout(r, SROK_RODNOY_VSTAVKI_MS + 20));

    expect(el.value).toBe("новое значение");
  });

  it("не трогает ничего, когда фокус не в поле ввода", async () => {
    const chitat = vi.fn(async () => "текст");
    naladit(chitat);
    const div = document.createElement("div");
    document.body.appendChild(div);

    nazhatCtrlV(div);
    await new Promise((r) => setTimeout(r, SROK_RODNOY_VSTAVKI_MS + 20));

    expect(chitat).not.toHaveBeenCalled();
  });

  it("пустой буфер не стирает то, что человек уже набрал", async () => {
    naladit(async () => "");
    const el = pole();
    el.value = "уже набрано";
    el.setSelectionRange(0, el.value.length);

    nazhatCtrlV(el);
    await new Promise((r) => setTimeout(r, SROK_RODNOY_VSTAVKI_MS + 20));

    expect(el.value).toBe("уже набрано");
  });
});
