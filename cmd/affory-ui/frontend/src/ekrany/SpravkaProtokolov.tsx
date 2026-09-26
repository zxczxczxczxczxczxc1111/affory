import { useEffect, useRef } from "react";

// Справка о протоколах, вызываемая значком рядом с заголовком «Серверы».
//
// Зачем. В подписке шесть ключей, и человек видит шесть строк с чужими
// словами: anytls, reality, hysteria2. Выбрать из них нечем, а выбор есть:
// замеры показали, что ключи расходятся в разы и по-разному в разных занятиях.
// Список без объяснения предлагает гадать.
//
// Числа под текстом. Переписано 21.09.2026 по прогону смеси голоса и
// трансляции от 20.09 - до него профили мерили по отдельности, и справка
// говорила ровно наоборот про два ключа из шести. Под нагрузкой (база канала
// 60-65 мс): tuic 64.4 и ни одного залипания за восемь замеров, reality+vision
// 63.8, trojan 86.7, anytls 101.5. После того же разбора у TUN задан MTU 1500,
// и TCP-ключи подтянулись: trojan 61.8, anytls 71.1 - но у anytls залипания
// остались. hy2 тогда же перенесён с занятого порта и стал ровным (60.09,
// 59.97, 60.11), а его цена вскрылась на замере обрыва 21.09: он возвращается
// за полминуты, остальные за треть секунды.
//
// Как написано. Без слов «UDP», «шифрование», «рукопожатие», «порт» и прочего
// из мира машин: человек, открывший эту справку, как раз и не знает этих слов,
// иначе она была бы ему не нужна. Каждому протоколу одна строка про то, что он
// даёт, и одна про то, чем платит. Обещаний без замера здесь нет.
//
// Редакция текстов - владельца, 21.09.2026: строки вдвое короче прежних, и
// названы вещи, которые человек видит сам (звонки, загрузки, вкладки), а не
// описаны обиняками. «HTTPS-сайт» и «фильтрация» оставлены сознательно: их
// человек встречает в браузере, в отличие от рукопожатий и портов. Факты за
// строками прежние, они из замеров ниже.

/** Одно описание: чем хорош и чем плох. Ключи совпадают с TRANSPORT в Servery. */
export const OPISANIYA: Record<string, { horosho: string; ceny: string }> = {
  anytls: {
    horosho: "Быстро открывает страницы и качает файлы.",
    ceny: "Звонки рвутся, если параллельно что-то грузится.",
  },
  trojan: {
    horosho: "Выглядит как обычный HTTPS-сайт, поэтому работает почти везде.",
    ceny: "Под нагрузкой звук в звонках может прерываться.",
  },
  "reality-tcp": {
    horosho: "Маскируется под известный сторонний сайт. Выручает, когда не работает ничего.",
    ceny: "На плохом интернете отвечает медленнее остальных.",
  },
  // Добавлено 26.09.2026, когда reality в подписке стал только поверх grpc.
  // Польза замерена в тот же день: 64 подключения разом через него прошли все,
  // а reality без grpc после такого шквала провайдер глушил на минуты. Цена
  // та же, что у grpc ниже: всё идёт одним каналом.
  "reality-grpc": {
    horosho: "Маскируется под сторонний сайт и не глохнет, когда открыто много вкладок сразу.",
    ceny: "Когда браузер грузит много всего сразу, скорость проседает.",
  },
  hy2: {
    horosho: "Самый быстрый на больших загрузках. Звонки стабильные.",
    ceny: "После обрыва переподключается около 30 секунд.",
  },
  tuic: {
    horosho: "Звонки не проседают, даже если параллельно идёт стрим или большая загрузка. Подключается сразу.",
    ceny: "Не работает в Happ.",
  },
  httpupgrade: {
    horosho: "Похож на обычный сайт. Проходит даже там, где заблокировано почти всё.",
    ceny: "Самый медленный из всех.",
  },
  ws: {
    horosho: "Устойчив в сетях с жёсткой фильтрацией.",
    ceny: "Медленный, задержка выше.",
  },
  grpc: {
    horosho: "Хорош, когда открыта одна вкладка или идёт одна загрузка.",
    ceny: "Проседает, когда браузер грузит много всего сразу.",
  },
  ss: {
    horosho: "Простой и проверенный.",
    ceny: "Не маскируется, блокируется легче всех. Новые такие ключи больше не выдают.",
  },
  vmess: {
    horosho: "Старый формат из других приложений.",
    ceny: "Преимуществ нет, лучше не использовать.",
  },
  xhttp: {
    horosho: "Оставлен, чтобы старые ключи не отображались как сломанные.",
    ceny: "Сервер его больше не принимает.",
  },
};

/** Подпись под заголовком: с чего начать, если выбирать не хочется.
 *
 *  Строка про чужие клиенты появилась 19.09.2026, когда в Happ недосчитались
 *  двух ключей из восьми, и подтвердилась 21.09 на живом экране: из шести
 *  ключей подписки Happ показал четыре, без tuic и anytls. Причина не в наших
 *  ссылках, их форма верна. Совет «бери tuic» без этой оговорки отправлял бы
 *  человека искать строку, которой у него на телефоне нет и не будет. Имя
 *  программы в совете владелец убрал: клиентов много, и Happ не единственный.
 *
 *  Строка про полминуты после смены сервера - из прогона 20.09: пять замеров
 *  подряд на reality дали 133, 85, 60, 68, 60 мс. У ключей, которые не на TCP,
 *  такого разогрева нет, но человеку в списке это ничем не видно, и он судит о
 *  новом сервере по худшей его минуте. */
export const SOVET = [
  "Не знаешь, что выбрать - бери tuic.",
  "Ничего не подключается - пробуй reality, его сложнее всего заблокировать.",
  "После смены сервера подожди полминуты: первые секунды скорость ниже.",
  "Некоторые клиенты могут не поддерживать tuic и anytls. В них бери trojan.",
];

function Krestik() {
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

/** Порядок показа: сперва то, что человек встретит в своей подписке, и в том
 *  порядке, в каком его стоит пробовать. Остальное ниже, ради чужих ключей.
 *
 *  Пересчитан 21.09.2026 по замеру под нагрузкой: раньше первым стоял anytls, а
 *  он оказался последним из живых ключей (101.5 мс против 64.4 у tuic), и
 *  человек, бравший верхнюю строку не глядя, брал худшее. Теперь порядок тот
 *  же, что в замере: tuic, hy2, reality, trojan, anytls, httpupgrade.
 *  reality поверх grpc (26.09.2026) встал на место reality: в подписке он его
 *  и заменил. */
const PORYADOK = ["tuic", "hy2", "reality-grpc", "reality-tcp", "trojan", "anytls", "httpupgrade",
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
              Все протоколы работают, но по-разному. Ниже - что показали замеры
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

        <ul className="text-fg-secondary flex list-none flex-col gap-1.5 text-[13px] leading-relaxed">
          {SOVET.map((s) => (
            <li key={s} className="before:text-fg-muted before:mr-2 before:content-['-']">{s}</li>
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
                    {OPISANIYA[t].horosho}
                    <span className="text-fg-muted"> {OPISANIYA[t].ceny}</span>
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
