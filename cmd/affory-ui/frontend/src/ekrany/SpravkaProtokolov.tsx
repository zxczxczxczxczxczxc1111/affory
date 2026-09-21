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

/** Одно описание: чем хорош и чем плох. Ключи совпадают с TRANSPORT в Servery. */
export const OPISANIYA: Record<string, { horosho: string; ceny: string }> = {
  anytls: {
    horosho: "Ровный в обычных делах: страницы открываются быстро, большое качается споро.",
    ceny: "Когда связь занята чем-то тяжёлым, голос спотыкается чаще других. После долгой паузы просыпается медленнее, чем hysteria2 и tuic.",
  },
  trojan: {
    horosho: "Спокойный и надёжный. Со стороны похож на обычный защищённый сайт, поэтому проходит почти везде.",
    ceny: "Первые полминуты после переключения работает заметно хуже, чем потом. Под тяжёлой нагрузкой голос иногда подрагивает.",
  },
  "reality-tcp": {
    horosho: "Притворяется чужим известным сайтом. Бери, когда остальные вообще не подключаются. Под нагрузкой держится почти как лучшие.",
    ceny: "Дольше остальных отвечает на плохой связи и первые полминуты после переключения.",
  },
  hy2: {
    horosho: "Быстрее всех качает большое, мгновенно оживает после паузы и ровно держит голос.",
    ceny: "Если связь оборвалась, возвращается примерно через полминуты - остальные возвращаются сразу.",
  },
  tuic: {
    horosho: "Ровнее всех, когда связь занята: голос не рассыпается, даже если рядом идёт трансляция или качается большое. Быстро откликается после простоя.",
    ceny: "Не всякая чужая программа его читает: в Happ такой строки не будет.",
  },
  httpupgrade: {
    horosho: "Выглядит как самый обычный сайт и пролезает там, где закрыто остальное.",
    ceny: "Самый медленный в наборе и дольше всех отвечает после паузы.",
  },
  ws: {
    horosho: "Обычное соединение с сайтом. Живучий вариант, когда сеть придирчива.",
    ceny: "Медленнее остальных, отклик хуже.",
  },
  grpc: {
    horosho: "Хорош, когда открыта одна страница или идёт одна загрузка.",
    ceny: "Слабеет, когда браузер тянет много всего разом, а это обычное дело.",
  },
  ss: {
    horosho: "Простой и давно известный способ.",
    ceny: "Ничем не притворяется, поэтому его проще всего распознать и закрыть. Такие ключи больше не выдаются.",
  },
  vmess: {
    horosho: "Старый формат из других программ, поддержан ради чужих ключей.",
    ceny: "Ничего нового не даёт, выбирать его незачем.",
  },
  xhttp: {
    horosho: "Оставлен ради старых ключей, чтобы они не выглядели сломанными.",
    ceny: "На сервере его больше нет.",
  },
};

/** Подпись под заголовком: с чего начать, если выбирать не хочется.
 *
 *  Строка про Happ появилась 19.09.2026, когда в нём недосчитались двух ключей
 *  из восьми, и подтвердилась 21.09 на живом экране: из шести ключей подписки
 *  Happ показал четыре, без tuic и anytls. Причина не в наших ссылках, их
 *  форма верна. Совет «бери tuic» без этой оговорки отправлял бы человека
 *  искать строку, которой у него на телефоне нет и не будет.
 *
 *  Строка про полминуты после смены сервера - из прогона 20.09: пять замеров
 *  подряд на reality дали 133, 85, 60, 68, 60 мс. У ключей, которые не на TCP,
 *  такого разогрева нет, но человеку в списке это ничем не видно, и он судит о
 *  новом сервере по худшей его минуте. */
export const SOVET = [
  "Не знаешь, что взять - бери tuic: он ровнее всех, когда связь чем-то занята.",
  "Не подключается ни один - пробуй reality, он лучше всех прячется.",
  "Сидишь с телефона и связь скачет - tuic оживает после пауз быстрее всех.",
  "Только что сменил сервер - дай полминуты: первые секунды связь честно хуже, чем будет.",
  "В приложении Happ видно не всё: anytls и tuic оно читать не умеет, там бери trojan.",
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
  "reality-tcp": "reality", hy2: "hysteria2", ws: "websocket", ss: "shadowsocks",
};

/** Порядок показа: сперва то, что человек встретит в своей подписке, и в том
 *  порядке, в каком его стоит пробовать. Остальное ниже, ради чужих ключей.
 *
 *  Пересчитан 21.09.2026 по замеру под нагрузкой: раньше первым стоял anytls, а
 *  он оказался последним из живых ключей (101.5 мс против 64.4 у tuic), и
 *  человек, бравший верхнюю строку не глядя, брал худшее. Теперь порядок тот
 *  же, что в замере: tuic, hy2, reality, trojan, anytls, httpupgrade. */
const PORYADOK = ["tuic", "hy2", "reality-tcp", "trojan", "anytls", "httpupgrade",
                  "ws", "grpc", "ss", "vmess", "xhttp"];

/** Справка. `svoi` это транспорты ключей, которые у человека на руках.
 *
 *  Разделение появилось 19.09.2026 при первом осмотре глазами: в окне
 *  одиннадцать протоколов, а в подписке шесть, и четыре хвостовых блока
 *  человек листает мимо того, чего у него нет. Без списка (или с пустым)
 *  показывается всё подряд, как раньше. */
export function SpravkaProtokolov({ zakryt, svoi }: { zakryt: () => void; svoi?: string[] }) {
  const okno = useRef<HTMLDivElement>(null);
  // trojan-ws и vmess-ws это те же протоколы поверх соединения с сайтом, и
  // отдельного описания у них нет: без сведения к основному человек с таким
  // ключом не нашёл бы в своём разделе ничего.
  const nabor = new Set((svoi ?? []).map((t) => t.replace(/-ws$/, "")));
  const est = PORYADOK.filter((t) => OPISANIYA[t] && nabor.has(t));
  const ostalnye = PORYADOK.filter((t) => OPISANIYA[t] && !nabor.has(t));
  const razdely: { zagolovok?: string; transporty: string[] }[] = est.length
    ? [{ transporty: est },
       { zagolovok: "Остальные - если ключ пришёл со стороны", transporty: ostalnye }]
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
              Все они везут одно и то же, но по-разному. Разница видна на замерах
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
