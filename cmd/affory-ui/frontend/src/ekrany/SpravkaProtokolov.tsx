import { useEffect, useRef } from "react";

// Справка о протоколах, вызываемая значком рядом с заголовком «Серверы».
//
// Зачем. В подписке шесть ключей, и человек видит шесть строк с чужими
// словами: anytls, reality, hysteria2. Список без объяснения предлагает гадать.
//
// С 02.10.2026 справка описывает устройство, а не советует (решение
// владельца). Прежняя редакция от 21.09 ставила tuic первым и звала «бери
// tuic» по замерам одной машины на одном провайдере, а у другого человека тот
// же tuic может резать провайдер, и совет отправлял бы его к худшему ключу.
// Устройство протокола одинаково у всех, от провайдера зависит только то,
// как сеть с этим устройством обходится. Поэтому сверху два вида протоколов и
// что каждому мешает в любой сети, а у протокола то, как он устроен и что из
// этого следует.
//
// Как написано. Без слов «UDP», «шифрование», «рукопожатие», «порт» и прочего
// из мира машин: человек, открывший эту справку, как раз и не знает этих слов,
// иначе она была бы ему не нужна. «https» оставлен сознательно: его человек
// видит в браузере. Ни «бери», ни «самый быстрый»: сравнение без замера на
// своей сети это совет под видом описания.

/** Как устроен протокол и что из этого следует. Ключи совпадают с TRANSPORT
 *  в Servery. */
export const OPISANIYA: Record<string, { kak: string; sledstvie: string }> = {
  tuic: {
    kak: "Каждое подключение идёт отдельно от остальных, и новое начинается без лишнего обмена с сервером.",
    sledstvie: "Звонки и игры идут своими пакетами, как без VPN. Есть не во всех приложениях.",
  },
  hy2: {
    kak: "Со стороны похож на подключение к современному сайту.",
    sledstvie: "Держит скорость, даже когда связь теряет часть данных.",
  },
  "reality-tcp": {
    kak: "Представляется известным сторонним сайтом: со стороны подключение выглядит как заход на тот сайт.",
    sledstvie: "Каждое подключение отдельное, как у браузера. Когда их открывается много сразу, некоторые провайдеры начинают их притормаживать.",
  },
  // Добавлено 26.09.2026, когда reality в подписке стал только поверх grpc:
  // 64 подключения разом через него прошли все, а reality без grpc после
  // такого шквала провайдер глушил на минуты.
  "reality-grpc": {
    kak: "Та же маскировка под сторонний сайт, но все подключения идут внутри одного общего канала.",
    sledstvie: "Много вкладок разом не создают много подключений. Зато затор в общем канале тормозит всё, что по нему идёт.",
  },
  trojan: {
    kak: "Выглядит как обычное подключение к сайту по https.",
    sledstvie: "Устроен просто и есть почти во всех приложениях.",
  },
  anytls: {
    kak: "Выглядит как подключение к сайту по https, как trojan, но меняет размеры передаваемых кусков, чтобы его труднее было узнать.",
    sledstvie: "Держит готовые подключения про запас, поэтому новые страницы открываются без ожидания. Есть не во всех приложениях.",
  },
  httpupgrade: {
    kak: "Начинается как обычный запрос к сайту, а дальше данные идут тем же подключением.",
    sledstvie: "Может идти через посредников, которые раздают сайты, тогда сам сервер со стороны не виден, а скорость зависит от посредника.",
  },
  ws: {
    kak: "Подключение как у онлайн-чатов в браузере: открывается запросом к сайту и остаётся открытым.",
    sledstvie: "Может идти через посредников, которые раздают сайты. Каждый кусок данных несёт небольшую служебную добавку.",
  },
  grpc: {
    kak: "Все подключения идут внутри одного общего канала, как у reality поверх grpc, только без маскировки под сторонний сайт.",
    sledstvie: "Затор в общем канале тормозит всё, что по нему идёт. Может идти через посредников, которые раздают сайты.",
  },
  ss: {
    kak: "Превращает данные в поток, похожий на случайный шум, без маскировки под сайт.",
    sledstvie: "Устроен просто, но такой шум системы блокировки узнают легче, чем подключения под видом сайта.",
  },
  vmess: {
    kak: "Старый формат из других приложений: сам прячет данные и сверяет время отправки с часами сервера.",
    sledstvie: "Не подключается, если часы компьютера и сервера расходятся больше чем на минуту-другую.",
  },
  xhttp: {
    kak: "Делит подключение на обычные запросы к сайту.",
    sledstvie: "Affory такие ключи больше не подключает. Они остаются в списке, чтобы не выглядеть сломанными.",
  },
};

/** Два вида протоколов сверху. От вида зависит, что протоколу мешает, и это
 *  верно в любой сети, в отличие от «какой быстрее»: это уже свойство
 *  провайдера, а не протокола. */
export const VIDY: { imya: string; kto: string; tekst: string }[] = [
  {
    imya: "Пакетами",
    kto: "tuic, hy2",
    tekst: "Потеря одного куска данных не задерживает остальные. Некоторые провайдеры и сети такие подключения режут или замедляют.",
  },
  {
    imya: "Потоком",
    kto: "остальные",
    tekst: "Такие подключения пропускает почти любая сеть. Если связь теряет данные, поток ждёт, пока потерянное придёт заново.",
  },
];

export function Krestik() {
  return (
    <svg viewBox="0 0 16 16" width="14" height="14" fill="none" aria-hidden="true">
      <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
    </svg>
  );
}

/** Значок у заголовка. Знак вопроса, а не «i»: человек сюда идёт с вопросом. */
export function IkonkaSpravka() {
  return (
    <svg viewBox="0 0 16 16" width="15" height="15" fill="none" aria-hidden="true">
      <circle cx="8" cy="8" r="6.25" stroke="currentColor" strokeWidth="1.3" />
      <path d="M6.4 6.2a1.6 1.6 0 1 1 2.1 1.5c-.4.15-.6.45-.6.85v.3" stroke="currentColor"
            strokeWidth="1.3" strokeLinecap="round" />
      <circle cx="8" cy="11.3" r="0.75" fill="currentColor" />
    </svg>
  );
}

/** Кнопка со значком: открывает справку. Отдельно от окна, потому что живёт в
 *  шапке экрана, а окно поверх всего. */
export function KnopkaSpravki({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      data-testid="spravka-protokolov-otkryt"
      aria-label="Чем отличаются протоколы"
      title="Чем отличаются протоколы"
      onClick={onClick}
      className="text-fg-muted hover:text-foreground hover:bg-surface-hover focus-visible:ring-accent-ink inline-flex h-6 w-6 items-center justify-center rounded-md transition-colors focus-visible:outline-none focus-visible:ring-2"
    >
      <IkonkaSpravka />
    </button>
  );
}

/** Второе имя протокола, под которым он попадается в чужих программах.
 *
 *  Заголовком идёт САМ транспорт, слово в слово как в строке сервера: 19.09.2026
 *  осмотр показал, что в списке стоит «hy2», а справка звала его «hysteria2», и
 *  связать одно с другим человеку было нечем. Второе имя ушло в скобки, где оно
 *  помогает узнать протокол в чужом клиенте и ничего не подменяет. */
export const NAZVANIYA: Record<string, string> = {
  "reality-tcp": "reality", "reality-grpc": "reality", hy2: "hysteria2", ws: "websocket", ss: "shadowsocks",
};

/** Порядок показа по виду, как в пояснении сверху: сперва пакетами, потом
 *  потоком, внутри потока от маскировки под сайт к старым форматам. Это не
 *  рейтинг: с 02.10.2026 справка ничего не советует, а порядок только держит
 *  похожие протоколы рядом, чтобы их было с чем сравнить. */
export const PORYADOK = ["tuic", "hy2", "reality-grpc", "reality-tcp", "trojan", "anytls", "httpupgrade",
                         "ws", "grpc", "ss", "vmess", "xhttp"];

/** Заголовки разделов. Константами, потому что по ним ищет границу тест: с
 *  формулировкой в двух местах он ломался при каждой правке текста. */
export const ZAGOLOVOK_SVOI = "Протоколы Affory";
export const ZAGOLOVOK_CHUZHIE = "Протоколы с других подписок";

/** Справка. `svoi` это транспорты ключей, которые у человека на руках.
 *
 *  Разделение появилось 19.09.2026 при первом осмотре глазами: в окне
 *  одиннадцать протоколов, а в подписке шесть, и четыре хвостовых блока
 *  человек листает мимо того, чего у него нет. Без списка (или с пустым)
 *  показывается всё подряд, как раньше.
 *
 *  Заголовки разделов названы владельцем 21.09.2026. Имя программы вместо
 *  лица: ключи и правда исходят отсюда, а от «мы» окно не говорит нигде
 *  (golos.test.ts). */
export function SpravkaProtokolov({ zakryt, svoi }: { zakryt: () => void; svoi?: string[] }) {
  const okno = useRef<HTMLDivElement>(null);
  // trojan-ws и vmess-ws это те же протоколы поверх соединения с сайтом, и
  // отдельного описания у них нет: без сведения к основному человек с таким
  // ключом не нашёл бы в своём разделе ничего.
  const nabor = new Set((svoi ?? []).map((t) => t.replace(/-ws$/, "")));
  const est = PORYADOK.filter((t) => OPISANIYA[t] && nabor.has(t));
  const ostalnye = PORYADOK.filter((t) => OPISANIYA[t] && !nabor.has(t));
  const razdely: { zagolovok?: string; transporty: string[] }[] = est.length
    ? [{ zagolovok: ZAGOLOVOK_SVOI, transporty: est },
       { zagolovok: ZAGOLOVOK_CHUZHIE, transporty: ostalnye }]
    : [{ transporty: ostalnye }];

  // Esc закрывает, и фокус уезжает внутрь окна: без этого человек, пришедший
  // с клавиатуры, остаётся стоять на кнопке под затемнением и не понимает,
  // почему нажатия ничего не делают.
  useEffect(() => {
    const naKlavishu = (e: KeyboardEvent) => {
      if (e.key === "Escape") zakryt();
    };
    document.addEventListener("keydown", naKlavishu);
    okno.current?.focus();
    return () => document.removeEventListener("keydown", naKlavishu);
  }, [zakryt]);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) zakryt();
      }}
    >
      <div
        ref={okno}
        tabIndex={-1}
        role="dialog"
        aria-modal="true"
        aria-label="Чем отличаются протоколы"
        data-testid="spravka-protokolov"
        className="border-border bg-surface flex max-h-[85vh] w-full max-w-xl flex-col gap-4 overflow-y-auto rounded-xl border p-5 focus-visible:outline-none"
      >
        <header className="flex items-start justify-between gap-4">
          <div>
            <h3 className="text-foreground text-base font-semibold">Чем отличаются протоколы</h3>
            <p className="text-fg-muted mt-0.5 text-[13px]">
              Как устроен каждый протокол. Какой работает лучше у тебя, зависит от провайдера и сети
            </p>
          </div>
          <button
            type="button"
            data-testid="spravka-protokolov-zakryt"
            aria-label="Закрыть"
            onClick={zakryt}
            className="text-fg-muted hover:text-foreground hover:bg-surface-hover inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-md transition-colors"
          >
            <Krestik />
          </button>
        </header>

        <ul className="text-fg-secondary flex list-none flex-col gap-2 text-[13px] leading-relaxed" data-testid="spravka-vidy">
          {VIDY.map((v) => (
            <li key={v.imya}>
              <span className="text-foreground font-medium">{v.imya}</span>
              <span className="text-fg-muted">{` (${v.kto}). `}</span>
              {v.tekst}
            </li>
          ))}
        </ul>

        {razdely.filter((r) => r.transporty.length > 0).map((r) => (
          <section key={r.zagolovok ?? "svoi"} className="flex flex-col gap-3">
            {r.zagolovok && (
              <h4 className="text-fg-muted border-border border-t pt-3 text-[12px] uppercase tracking-wide">
                {r.zagolovok}
              </h4>
            )}
            <dl className="flex flex-col gap-3">
              {r.transporty.map((t) => (
                <div key={t} className="border-border border-t pt-3 first:border-t-0 first:pt-0">
                  <dt className="text-foreground text-[13px] font-medium">
                    {t}
                    {NAZVANIYA[t] && <span className="text-fg-muted font-normal"> ({NAZVANIYA[t]})</span>}
                  </dt>
                  <dd className="text-fg-secondary mt-0.5 text-[13px] leading-relaxed">
                    {OPISANIYA[t].kak}
                    <span className="text-fg-muted"> {OPISANIYA[t].sledstvie}</span>
                  </dd>
                </div>
              ))}
            </dl>
          </section>
        ))}
      </div>
    </div>
  );
}
