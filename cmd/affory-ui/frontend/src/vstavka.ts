// Запасная вставка по Ctrl+V.
//
// Wails безусловно выключает у WebView2 браузерные акселераторы
// (`settings.PutAreBrowserAcceleratorKeysEnabled(false)` в
// webview_window_windows.go beta.16), опции окна этого не переопределяют.
// Правки при этом обязаны были остаться, и на стенде Ctrl+V действительно
// вставляет, а на рабочей машине с тем же рантаймом WebView2 152.0.4191.66 не
// вставляет вовсе: помогает только «Вставить» из контекстного меню. Причина
// расхождения снаружи страницы, а поле ввода нужно человеку на обеих.
//
// Поэтому не замена родной вставке, а её подстраховка: ждём событие `paste`, и
// только если оно не пришло, читаем буфер сами. Там, где акселератор доживает
// до страницы, этот код не делает ничего и вставки не задваивает.

/** Сколько ждать родного `paste` после нажатия. Порядок величины взят с
 *  запасом: событие приходит тем же тиком, что и клавиша, а лишние миллисекунды
 *  человек не замечает. */
export const SROK_RODNOY_VSTAVKI_MS = 80;

type Vvod = HTMLInputElement | HTMLTextAreaElement;

function podhodit(el: EventTarget | null): el is Vvod {
  if (!(el instanceof HTMLInputElement) && !(el instanceof HTMLTextAreaElement)) return false;
  if (el.disabled || el.readOnly) return false;
  // Поля, где вставка не текст: числовые и им подобные не имеют выделения, и
  // setSelectionRange на них бросает.
  return !(el instanceof HTMLInputElement) || ["text", "search", "url", "tel", "password", "email", ""].includes(el.type);
}

/** Пишет значение так, чтобы React увидел. Прямое `el.value = x` он
 *  пропускает: его слушатель висит на событии, а сеттер переопределён. */
function zapisat(el: Vvod, tekst: string) {
  const proto = el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLTextAreaElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, "value")?.set;
  if (setter) setter.call(el, tekst);
  else el.value = tekst;
  el.dispatchEvent(new Event("input", { bubbles: true }));
}

/** Вешает подстраховку на документ и отдаёт способ её снять. */
export function naladitVstavku(chitatBufer: () => Promise<string>): () => void {
  let zhdyom: Vvod | null = null;
  let taymer = 0;

  const zabyt = () => {
    zhdyom = null;
    if (taymer) {
      window.clearTimeout(taymer);
      taymer = 0;
    }
  };

  const naPaste = () => zabyt();

  const naKlavishu = (e: KeyboardEvent) => {
    // code, а не key: в русской раскладке key приходит 'м', и проверка по 'v'
    // молча не совпадает при физически той же клавише.
    if (e.code !== "KeyV" || !(e.ctrlKey || e.metaKey) || e.altKey) return;
    const el = e.target;
    if (!podhodit(el)) return;

    zabyt();
    zhdyom = el;
    taymer = window.setTimeout(() => {
      const cel = zhdyom;
      zabyt();
      if (!cel || !cel.isConnected) return;
      void chitatBufer().then((tekst) => {
        // Пустой буфер это не команда стереть набранное.
        if (!tekst) return;
        const nachalo = cel.selectionStart ?? cel.value.length;
        const konec = cel.selectionEnd ?? nachalo;
        zapisat(cel, cel.value.slice(0, nachalo) + tekst + cel.value.slice(konec));
        const posle = nachalo + tekst.length;
        try {
          cel.setSelectionRange(posle, posle);
        } catch {
          // Поле без выделения: текст уже на месте, курсор не наша забота.
        }
      });
    }, SROK_RODNOY_VSTAVKI_MS);
  };

  document.addEventListener("keydown", naKlavishu, true);
  document.addEventListener("paste", naPaste, true);
  return () => {
    zabyt();
    document.removeEventListener("keydown", naKlavishu, true);
    document.removeEventListener("paste", naPaste, true);
  };
}
