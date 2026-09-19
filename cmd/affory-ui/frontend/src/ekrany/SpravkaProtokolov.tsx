import { useEffect, useRef } from "react";

// Справка о протоколах, вызываемая значком рядом с заголовком «Серверы».
//
// Зачем. В подписке восемь ключей, и человек видит восемь строк с чужими
// словами: anytls, reality, hysteria2. Выбрать из них нечем, а выбор есть:
// замер 19.09.2026 показал, что ключи расходятся в разы и по-разному в разных
// занятиях. Список без объяснения предлагает гадать.
//
// Как написано. Без слов «UDP», «шифрование», «рукопожатие», «порт» и прочего
// из мира машин: человек, открывший эту справку, как раз и не знает этих слов,
// иначе она была бы ему не нужна. Каждому протоколу одна строка про то, что он
// даёт, и одна про то, чем платит. Обещаний без замера здесь нет.

/** Одно описание: чем хорош и чем плох. Ключи совпадают с TRANSPORT в Servery. */
export const OPISANIYA: Record<string, { horosho: string; ceny: string }> = {
  anytls: {
    horosho: "Ровнее всех в обычных делах: страницы открываются быстро, видеозвонки не рассыпаются.",
    ceny: "Просыпается после долгой паузы чуть медленнее, чем hysteria2 и tuic.",
  },
  trojan: {
    horosho: "Спокойный и надёжный. Со стороны похож на обычный защищённый сайт, поэтому проходит почти везде.",
    ceny: "Ничем особенно не выделяется: ни самый быстрый, ни самый незаметный.",
  },
  "reality-tcp": {
    horosho: "Притворяется чужим известным сайтом. Бери, когда остальные вообще не подключаются.",
    ceny: "На плохой связи отвечает заметно дольше других.",
  },
  hy2: {
    horosho: "Быстрее всех качает большое и мгновенно оживает после паузы.",
    ceny: "На голосовых звонках голос дрожит заметнее, чем на anytls и trojan.",
  },
  tuic: {
    horosho: "Быстро откликается после простоя, хорош на мобильном интернете.",
    ceny: "На видео и трансляциях спотыкается чаще остальных.",
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
 *  Четвёртая строка про Happ появилась 19.09.2026, когда в нём недосчитались
 *  двух ключей из восьми. Причина не в наших ссылках: Happ читает только
 *  шесть видов ссылок, и anytls с tuic в них не входят. Совет «бери anytls»
 *  без этой оговорки отправлял бы человека искать строку, которой у него на
 *  телефоне нет и не будет. */
export const SOVET = [
  "Не знаешь, что взять - бери anytls: он ровнее всех в обычных делах.",
  "Не подключается ни один - пробуй reality, он лучше всех прячется.",
  "Сидишь с телефона и связь скачет - hysteria2 или tuic отвечают быстрее после пауз.",
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

/** Имена протоколов в том же виде, в каком они стоят в строке сервера. */
export const NAZVANIYA: Record<string, string> = {
  anytls: "anytls", trojan: "trojan", "reality-tcp": "reality", hy2: "hysteria2",
  tuic: "tuic", httpupgrade: "httpupgrade", ws: "websocket", grpc: "grpc",
  ss: "shadowsocks", vmess: "vmess", xhttp: "xhttp",
};

/** Порядок показа: сперва то, что человек встретит в своей подписке, и в том
 *  порядке, в каком его стоит пробовать. Остальное ниже, ради чужих ключей. */
const PORYADOK = ["anytls", "trojan", "reality-tcp", "hy2", "tuic", "httpupgrade",
                  "ws", "grpc", "ss", "vmess", "xhttp"];

export function SpravkaProtokolov({ zakryt }: { zakryt: () => void }) {
  const okno = useRef<HTMLDivElement>(null);

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

        <dl className="flex flex-col gap-3">
          {PORYADOK.filter((t) => OPISANIYA[t]).map((t) => (
            <div key={t} className="border-border border-t pt-3 first:border-t-0 first:pt-0">
              <dt className="text-foreground text-[13px] font-medium">{NAZVANIYA[t] ?? t}</dt>
              <dd className="text-fg-secondary mt-0.5 text-[13px] leading-relaxed">
                {OPISANIYA[t].horosho}
                <span className="text-fg-muted"> {OPISANIYA[t].ceny}</span>
              </dd>
            </div>
          ))}
        </dl>
      </div>
    </div>
  );
}
