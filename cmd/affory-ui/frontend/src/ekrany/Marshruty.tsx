import { useEffect, useRef, useState } from "react";
import type { PravilaProps } from "./Pravila";
import { imyaMarshruta, snimokPravil, zadatMarshrutServisa, type VyborServisa, type ChernovikPravil, type Marshrut, type PravilaTrafika } from "../trafik";
import { perekrytiyaServisa } from "../ohvat";
import { Flazhok, Knopka, Poisk, Pole, SegmentStolbik, Svorachivaemyy, Tumbler } from "./ui";
import { IkPlyus, IkSayt, IkSsylka, IkTreugolnik } from "../ikonki";
import { IkonkaServisa } from "./IkonkaServisa";
import { Vybor } from "./Vybor";
import { slovoPosleChisla } from "../chisla";
import { OhvatPravil } from "./OhvatPravil";

// Раздел правил: слева режим по умолчанию и счёт правил, справа три вкладки.
// Боковая область и вкладки стоят на одном месте во всех трёх видах, поэтому
// переключение вкладки ничего на экране не двигает.

export function VyborTrafika({
  value,
  disabled,
  onChange,
}: {
  value: Marshrut;
  disabled?: boolean;
  onChange: (r: Marshrut) => void;
}) {
  return (
    <SegmentStolbik
      aria-label="Трафик по умолчанию"
      aktiven={!disabled}
      vybrano={value}
      naVybor={onChange}
      znacheniya={[
        { z: "vpn", podpis: "Всё через VPN" },
        { z: "direct", podpis: "Только выбранное" },
      ]}
    />
  );
}

function RouteSelect({
  value,
  label,
  disabled,
  onChange,
}: {
  value: Marshrut;
  label: string;
  disabled?: boolean;
  onChange: (v: Marshrut) => void;
}) {
  return (
    <Vybor
      label={label}
      value={value}
      disabled={disabled}
      options={[{ value: "vpn", label: "Через VPN" }, { value: "direct", label: "Напрямую" }]}
      onChange={onChange}
    />
  );
}

/** Путь под ширину колонки: начало, многоточие, хвост с именем файла. Предел
 *  в знаках, а не в пикселях: мерить ширину в каждой строке таблицы значит
 *  мерить её на каждый кадр, а колонка тут фиксированная. */
export function sokratitPut(put: string, predel = 58): string {
  if (put.length <= predel) return put;
  return `${put.slice(0, 13)}...${put.slice(-(predel - 16))}`;
}

// Колонки таблиц задаются ОДНОЙ строкой на вид и переиспользуются шапкой и
// строками: две раскладки рядом это два места, где колонки разъезжаются.
const SETKA_PRILOZHENIY = "grid grid-cols-[minmax(0,1fr)_150px_80px] min-[1100px]:grid-cols-[minmax(0,1fr)_190px_150px_110px] items-center gap-x-4 gap-y-2";
const SETKA_SAYTOV = "grid grid-cols-[minmax(0,1fr)_150px_110px] items-center gap-x-4";

export function Marshruty({
  proveritPrilozhenie,
  proveritSoedineniya,
  status,
  pravila,
  trafik: sohranennyyTrafik,
  chernovik,
  naChernovik,
  zapushchennye,
  obnovitProtsessy,
  naVyborPrilozheniya,
  naKomandu,
  zanyato = false,
}: PravilaProps & { trafik: PravilaTrafika }) {
  const [localDraft, setLocalDraft] = useState<ChernovikPravil | null>(null);
  const draft = chernovik === undefined ? localDraft : chernovik;
  const setDraft = naChernovik ?? setLocalDraft;
  const trafik = draft?.trafik ?? sohranennyyTrafik;
  const bezRu = draft?.bezRu ?? (pravila?.bez_ru_spiska === true);
  const base = snimokPravil(sohranennyyTrafik, pravila?.bez_ru_spiska === true);
  const dirty = snimokPravil(trafik, bezRu) !== base;
  const conflict = !!draft && dirty && draft.baza !== base;
  const [applying, setApplying] = useState(false);
  const inFlight = useRef(false);
  const [notice, setNotice] = useState("");
  const [applyError, setApplyError] = useState("");
  const [tab, setTab] = useState<"services" | "apps" | "sites">("services");
  const [adding, setAdding] = useState(false);
  useEffect(() => {
    if (adding && tab === "apps") obnovitProtsessy?.();
  }, [adding, tab, obnovitProtsessy]);
  const [query, setQuery] = useState("");
  const [path, setPath] = useState("");
  const [picking, setPicking] = useState(false);
  const [pickerError, setPickerError] = useState("");
  const pickerEpoch = useRef(0);
  useEffect(() => () => { pickerEpoch.current++; }, []);
  const [domain, setDomain] = useState("");
  const [descendants, setDescendants] = useState(true);
  const [route, setRoute] = useState<Marshrut>(
    trafik.po_umolchaniyu === "vpn" ? "direct" : "vpn",
  );
  const [remove, setRemove] = useState<string | null>(null);
  const [raskryto, setRaskryto] = useState<Record<string, boolean>>({});
  const [vesSpisok, setVesSpisok] = useState(false);
  const disabled =
    applying ||
    zanyato ||
    picking ||
    status.sostoyanie === "sluzhba-molchit" ||
    status.sostoyanie === "podnimaetsya" ||
    status.sostoyanie === "vosstanavlivaetsya";
  const save = (next: PravilaTrafika, nextBezRu = bezRu) => {
    if (disabled || snimokPravil(next, nextBezRu) === snimokPravil(trafik, bezRu)) return false;
    setApplyError("");
    setNotice("");
    setDraft(snimokPravil(next, nextBezRu) === base ? null : {trafik:next,bezRu:nextBezRu,baza:draft?.baza ?? base,reviziya:draft?.reviziya ?? pravila?.reviziya_pravil});
    return true;
  };
  const apply = async () => {
    if (disabled || inFlight.current || !dirty || conflict) return;
    inFlight.current = true;
    setApplying(true); setApplyError(""); setNotice("");
    try {
      const revision = pravila?.reviziya_pravil ?? draft?.reviziya;
      const body = {trafik,bez_ru_spiska:bezRu,...(revision ? {reviziya_pravil:revision} : {})};
      const ok = await naKomandu("setRules",body);
      if (ok !== true) { setApplyError("Не удалось применить правила. Черновик сохранён; причина указана в сообщении службы."); return; }
      setDraft(null); setPath(""); setDomain(""); setAdding(false);
      setNotice(status.sostoyanie === "vyklyuchen" ? "Правила сохранены" : "Правила применены");
    } catch (error: unknown) {
      setApplyError(error instanceof Error ? error.message : String(error));
    } finally {
      inFlight.current = false; setApplying(false);
    }
  };
  const apps = trafik.prilozheniya ?? [],
    domains = trafik.domeny ?? [],
    services = trafik.servisy ?? [];
  const katalog = pravila?.katalog?.servisy ?? [];
  const ruleCount = apps.length + domains.length + services.length;
  const candidates = (zapushchennye ?? []).filter((p) =>
    `${p.imya} ${p.put}`.toLowerCase().includes(query.toLowerCase()),
  );
  const changeTab = (next: typeof tab) => {
    pickerEpoch.current++;
    setPicking(false);
    setPickerError("");
    setTab(next);
    setAdding(false);
    setRemove(null);
  };
  const browse = async () => {
    if (disabled || !naVyborPrilozheniya) return;
    const epoch = ++pickerEpoch.current;
    setPicking(true);
    setPickerError("");
    try {
      const chosen = await naVyborPrilozheniya();
      if (epoch === pickerEpoch.current && chosen) setPath(chosen);
    } catch (error: unknown) {
      if (epoch === pickerEpoch.current)
        setPickerError(error instanceof Error ? error.message : String(error));
    } finally {
      if (epoch === pickerEpoch.current) setPicking(false);
    }
  };
  const add = () => {
    if (disabled) return;
    let changed = false;
    if (tab === "apps") {
      const app = {
        put: path.trim(),
        imya: path.trim().split(/[/\\]/).pop() ?? "Приложение",
        potomki: descendants,
        marshrut: route,
      };
      const existing = apps.findIndex(p => p.put.toLowerCase() === app.put.toLowerCase());
      changed = save({
        ...trafik,
        prilozheniya: existing < 0 ? [...apps,app] : apps.map((previous,i) => i===existing ? app : previous),
      });
    } else {
      const normalized = domain.trim().toLowerCase().replace(/^\.+|\.+$/g, "");
      changed = save({
        ...trafik,
        domeny: [
          ...domains.filter((d) => d.domen !== normalized),
          { domen: normalized, marshrut: route },
        ],
      });
    }
    setAdding(false);
    setNotice(changed ? "Правило добавлено в черновик. Нажми «Применить изменения», когда закончишь редактирование." : "Такое правило уже есть в списке.");
  };

  // Что включено на вкладке, видно НЕ ЗАХОДЯ на неё. Прежде счёт был только у
  // приложений и сайтов, а «российские сайты напрямую» не показывал вообще
  // никто: чтобы узнать про них, надо было догадаться открыть «Сайты».
  const ruSpisokVkl = !bezRu;
  // Tabs count visible entries; the service summary separately counts configured
  // routes and explicit overrides. Neither counter claims a live network result.
  const servisovCherezVPN = katalog.filter(
    (s) => (services.find((r) => r.id === s.id)?.marshrut ?? trafik.po_umolchaniyu) === "vpn",
  ).length;
  const servisovYavno=katalog.filter(s=>services.some(r=>r.id===s.id)).length;
  const pereopredeleniya=new Map(katalog.map(s=>{
    const route=services.find(r=>r.id===s.id)?.marshrut ?? trafik.po_umolchaniyu;
    return [s.id,perekrytiyaServisa(s.domeny,domains,route)];
  }));
  const perekryvayushchihDomenov=new Set([...pereopredeleniya.values()].flat().map(d=>d.domen)).size;
  const VKLADKI: { v: typeof tab; podpis: string; schyot: number; poyasnenie?: string; vklyucheno?: string }[] = [
    {
      v: "services",
      podpis: "Сервисы",
      schyot: katalog.length,
      poyasnenie: `${katalog.length} сервисов в каталоге`,
    },
    { v: "apps", podpis: "Приложения", schyot: apps.length, poyasnenie: `${apps.length} правил приложений` },
    {
      v: "sites",
      podpis: "Сайты",
      schyot: domains.length,
      poyasnenie: `${domains.length} правил сайтов`,
      vklyucheno: ruSpisokVkl ? "Российские сайты идут напрямую" : undefined,
    },
  ];

  return (
    <section className="flex min-h-0 flex-1" aria-label="Правила">
      <aside className="border-border flex w-[288px] shrink-0 flex-col gap-4 overflow-y-auto border-r px-6 py-6">
        <h2 className="text-foreground text-[22px] font-semibold leading-tight">Куда идёт трафик</h2>
        <VyborTrafika
          value={trafik.po_umolchaniyu}
          disabled={disabled}
          onChange={(value) => save({ ...trafik, po_umolchaniyu: value })}
        />
        <p className="text-fg-muted text-sm leading-relaxed">
          {trafik.po_umolchaniyu === "vpn"
            ? "VPN для всего интернета. Добавь приложения и сайты, которые должны работать напрямую"
            : "Прямой интернет. Через VPN идёт только то, что добавлено в правила"}
        </p>
        <div className="border-border mt-1 border-t pt-4" data-testid="svodka-pravil">
          <p className="flex items-baseline gap-2">
            <span className="text-accent-ink text-[26px] font-semibold leading-none">{ruleCount}</span>
            <span className="text-fg-secondary text-sm">
              {slovoPosleChisla(ruleCount, "отдельное правило", "отдельных правила", "отдельных правил")}
            </span>
          </p>
          <p className="text-fg-muted mt-2 text-[13px] leading-relaxed">
            Явные маршруты сохраняются при смене режима
          </p>
          <p className="text-fg-muted mt-2 flex flex-wrap gap-x-3 text-[13px] leading-relaxed"><span className="whitespace-nowrap">Приложения: {apps.length}</span><span className="whitespace-nowrap">Сайты: {domains.length}</span><span className="whitespace-nowrap">Сервисы: {services.length}</span></p>
        </div>
        {status.sostoyanie === "vyklyuchen" && (
          <p className="text-fg-muted text-[13px] leading-relaxed">
            Правила начнут работать после нажатия на сферу
          </p>
        )}
        {(zanyato || applying) && (
          <p role="status" className="text-fg-secondary text-[13px]">Применяю правила</p>
        )}
        {dirty && <div className="border-border flex flex-col gap-3 border-t pt-4" aria-label="Несохранённые правила">
          <p className="text-fg-secondary text-sm">Есть неприменённые изменения</p>
          {conflict ? <p role="alert" className="text-warn text-[13px]">Сохранённые правила изменились. Отмени черновик и проверь актуальный набор перед редактированием.</p>
            : <p className="text-fg-muted text-[13px]">{status.sostoyanie === "vyklyuchen" ? "Весь набор сохранится одним действием." : "Применение всего набора вызовет одно короткое переподключение VPN."}</p>}
          <Knopka rang="glavnaya" aktiven={!disabled && !conflict} zhdyot={applying} onClick={() => void apply()}>Применить изменения</Knopka>
          <Knopka rang="vtoraya" aktiven={!disabled} onClick={() => {setDraft(null);setPath("");setDomain("");setAdding(false);setApplyError("");setNotice("Черновик отменён");}}>Отменить изменения</Knopka>
        </div>}
        {notice && <p role="status" className="text-fg-secondary text-[13px]">{notice}</p>}
        {applyError && <p role="alert" className="text-danger text-[13px]">{applyError}</p>}
        {pravila?.trebuet_podyoma && (
          <p role="status" className="text-warn text-[13px] leading-relaxed">
            Сохранено. Нужен повторный запуск подключения
          </p>
        )}
        {status.kill_switch && (
          <p className="text-fg-muted text-[13px] leading-relaxed">
            Прямые маршруты не работают с блокировкой сети вне VPN, выключи её в настройках защиты
          </p>
        )}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto">
        <nav role="tablist" aria-label="Вид правил" className="border-border flex shrink-0 items-stretch gap-1 border-b px-8">
          {VKLADKI.map(({ v, podpis, schyot, poyasnenie, vklyucheno }) => {
            const on = v === tab;
            return (
              <button
                key={v}
                type="button"
                role="tab"
                aria-selected={on}
                onClick={() => changeTab(v)}
                className={`relative flex h-12 items-center gap-2 px-3 text-sm font-medium transition-colors ${
                  on ? "text-foreground" : "text-fg-muted hover:text-fg-secondary"
                }`}
              >
                {podpis}
                {/* Число элементов на вкладке, а не число соединений через VPN. */}
                {schyot > 0 && (
                  <span
                    title={poyasnenie}
                    className={`text-[13px] font-normal ${on ? "text-accent-ink" : "text-fg-faint"}`}
                  >
                    {schyot}
                  </span>
                )}
                {/* Точка значит «здесь включено то, чего в счёте правил нет».
                    Она названа словами, а не оставлена загадкой: имя читает и
                    подсказка, и чтение с экрана. */}
                {vklyucheno && (
                  <span
                    className="bg-accent-ink h-1.5 w-1.5 shrink-0 rounded-full"
                    data-testid={`vklyucheno-${v}`}
                    title={vklyucheno}
                    aria-label={vklyucheno}
                    role="img"
                  />
                )}
                {on && <span aria-hidden className="bg-accent-ink absolute inset-x-0 bottom-0 h-[2px] rounded-t" />}
              </button>
            );
          })}
        </nav>

        <div className="min-w-0 flex-1 px-8 py-6">
          {tab === "services" && (
            <div className="flex flex-col gap-5">
              <header>
                <h3 className="text-foreground text-[17px] font-semibold leading-tight">Популярные сервисы</h3>
                <p className="text-fg-muted mt-1 text-[13px]">Маршрут доменов и поддоменов. «По общему режиму» следует настройке слева.</p>
              </header>
              {(katalog.length>0 || services.length>0) && <div className="flex flex-wrap items-center justify-between gap-3">
                {katalog.length>0 && <div className="text-fg-muted text-[13px]" aria-label="Маршруты сервисов">
                  <p>По настройкам: через VPN {servisovCherezVPN}, напрямую {katalog.length-servisovCherezVPN}</p>
                  <p>Заданы отдельно: {servisovYavno} · по общему режиму: {katalog.length-servisovYavno}</p>
                  {perekryvayushchihDomenov>0 && <p className="text-warn">Другой маршрут задан в правилах сайтов: {perekryvayushchihDomenov}</p>}
                </div>}
                <Knopka rang="vtoraya" aktiven={!disabled && services.length>0} onClick={()=>save({...trafik,servisy:[]})}>Сбросить маршруты сервисов</Knopka>
              </div>}

              {katalog.length === 0 && (
                // Пустой каталог рисовал пустоту: заголовок, подпись и полэкрана
                // ничего. Служба старее окна не шлёт каталог вовсе, и человек
                // видел сломанную вкладку вместо объяснения.
                <div className="border-border flex flex-col items-center gap-2 rounded-xl border border-dashed px-6 py-10 text-center" data-testid="net-katalog">
                  <span className="text-foreground text-sm font-medium">Каталог сервисов не пришёл</span>
                  <span className="text-fg-muted max-w-[440px] text-[13px] leading-relaxed">
                    Список готовых наборов доменов обновляется вместе с программой.
                    Правила приложений и сайтов работают и без него
                  </span>
                </div>
              )}

              <div className="grid grid-cols-1 gap-3 min-[1100px]:grid-cols-2">
                {katalog.map((service) => {
                  const explicit = services.find((r) => r.id === service.id);
                  const effective = explicit?.marshrut ?? trafik.po_umolchaniyu;
                  const vkl = effective === "vpn";
                  const otkryt = raskryto[service.id] ?? false;
                  return (
                    <article key={service.id} className="border-border bg-surface flex flex-col overflow-hidden rounded-xl border">
                      <div className="flex flex-wrap items-center gap-3 px-4 py-3.5">
                        <span className={`border-border flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border ${vkl ? "text-foreground" : "text-fg-faint"}`}>
                          <IkonkaServisa id={service.id} imya={service.imya} />
                        </span>
                        <span className="flex min-w-[80px] flex-1 flex-col gap-0.5">
                          <span className="text-foreground truncate text-sm font-medium">{service.imya}</span>
                          <span className="text-fg-muted truncate text-[13px]">
                            {vkl ? "Через VPN" : "Напрямую"}
                          </span>
                        </span>
                        <Vybor<VyborServisa> label={`Маршрут сервиса ${service.imya}`} value={explicit?.marshrut ?? "inherit"} disabled={disabled}
                          options={[{value:"inherit",label:"По общему режиму"},{value:"vpn",label:"Через VPN"},{value:"direct",label:"Напрямую"}]}
                          onChange={value=>save(zadatMarshrutServisa(trafik,service.id,value))}/>
                      </div>
                      {(pereopredeleniya.get(service.id)?.length ?? 0)>0 && <p className="text-warn px-4 pb-2 text-[12px]">Есть другой маршрут в правилах сайтов</p>}

                      <button
                        type="button"
                        aria-expanded={otkryt}
                        onClick={() => setRaskryto({ ...raskryto, [service.id]: !otkryt })}
                        className="group border-border hover:bg-surface-hover flex items-center gap-2 border-t px-4 py-2.5 text-left transition-colors"
                      >
                        <IkTreugolnik className={`text-fg-muted h-3 w-3 shrink-0 transition-transform ${otkryt ? "rotate-90" : ""}`} />
                        <span className="text-fg-muted group-hover:text-fg-secondary text-[13px]">
                          {service.domeny.length} {slovoPosleChisla(service.domeny.length, "домен", "домена", "доменов")} с поддоменами
                        </span>
                      </button>

                      {otkryt && (
                        <ul className="border-border flex flex-col gap-1 border-t px-4 py-3">
                          {service.domeny.map((d) => (
                            <li key={d} className="text-fg-secondary break-all text-[13px]">{d}</li>
                          ))}
                          {pereopredeleniya.get(service.id)?.map(d=><li key={`override-${d.domen}`} className="text-warn break-all text-[12px]">Правило {d.domen}: {imyaMarshruta(d.marshrut).toLowerCase()}</li>)}
                        </ul>
                      )}
                    </article>
                  );
                })}
              </div>

              <p className="text-fg-muted text-[13px]">
                Здесь задан маршрут доменов сервиса. Правило приложения или отдельного сайта может его изменить.
              </p>
              <p className="text-fg-muted text-[13px]">
                Приложение обращается напрямую к IP? Добавь его во вкладке «Приложения»
              </p>

              <div className="flex flex-col">
                <Svorachivaemyy
                  zagolovok="Источник и особенности правил"
                  deti={
                    <div className="flex flex-col gap-2">
                      <p>
                        Каталог OpenCCK, версия {pravila?.katalog?.versiya ?? "неизвестна"}. Обновляется
                        вместе с программой. Правило накрывает домен и все его поддомены.
                      </p>
                      <p>
                        Сначала действует правило приложения, затем отдельного сайта, затем сервиса,
                        затем общий режим. Локальный прокси Affory выбирает VPN раньше пользовательских правил.
                        Проверить приложение вместе с сайтом можно на вкладках «Приложения» и «Сайты».
                      </p>
                      <p>
                        Браузер с защищённым DNS или ECH может скрыть имя сайта. Для такого случая
                        добавь приложение целиком во вкладке «Приложения».
                      </p>
                    </div>
                  }
                />
                <Svorachivaemyy
                  zagolovok="Дополнительно"
                  deti={
                    <div className="flex flex-col gap-3">
                      <div className="flex flex-wrap gap-2">
                        <Knopka rang="vtoraya" onClick={() => setVesSpisok(!vesSpisok)}>
                          {vesSpisok ? "Скрыть весь список доменов" : "Показать весь список доменов"}
                        </Knopka>
                      </div>
                      {vesSpisok && (
                        <ul className="border-border bg-elevated flex flex-col gap-1 rounded-lg border px-4 py-3">
                          {katalog.flatMap((s) => s.domeny.map((d) => ({ d, s }))).map(({ d, s }) => {
                            const cherez = (services.find((r) => r.id === s.id)?.marshrut ?? trafik.po_umolchaniyu) === "vpn";
                            return (
                              <li key={`${s.id}-${d}`} className="flex items-center justify-between gap-4 text-[13px]">
                                <span className="text-fg-secondary">{d}</span>
                                <span className="text-fg-muted">{cherez ? "через VPN" : "напрямую"}</span>
                              </li>
                            );
                          })}
                        </ul>
                      )}
                    </div>
                  }
                />
              </div>
            </div>
          )}

          {tab !== "services" && (
            <div className="flex flex-col gap-5">
              {tab === "sites" && (
                <div className="border-border bg-surface flex items-center gap-4 rounded-xl border px-4 py-3" aria-label="Российские сайты">
                  <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                    <span className="text-foreground text-sm font-medium">Российские сайты напрямую</span>
                    <span className="text-fg-muted text-[13px] leading-relaxed">
                      {/* Строка про «чтобы включить» показывалась и на ВКЛЮЧЁННОМ
                          списке: под блокировкой он не выключен, а не действует,
                          и звать включать уже включённое значит врать. */}
                      {status.kill_switch
                        ? "Список не действует, пока включена блокировка сети вне VPN. Выключить её можно в настройках защиты"
                        : "Сайты из российского списка открываются без VPN, если нет более важного правила"}
                    </span>
                  </span>
                  <Tumbler
                    testId="ru-spisok"
                    podpis="Российские сайты напрямую"
                    aktiven={!disabled && !status.kill_switch}
                    vkl={!bezRu}
                    naSmenu={(v) => save(trafik, !v)}
                  />
                </div>
              )}

              <header className="flex items-start justify-between gap-4">
                <div>
                  <h3 className="text-foreground text-[17px] font-semibold leading-tight">
                    {tab === "apps" ? "Правила приложений" : "Правила сайтов"}
                  </h3>
                  <p className="text-fg-muted mt-1 text-[13px]">
                    {tab === "apps"
                      ? "Приложение и программы, которые оно запускает"
                      : "Домен и все его поддомены"}
                  </p>
                </div>
                <Knopka
                  rang="glavnaya"
                  bolshaya
                  aria-expanded={adding}
                  aktiven={!disabled}
                  onClick={() => {
                    pickerEpoch.current++;
                    setPickerError("");
                    setPicking(false);
                    if (!adding) setRoute(trafik.po_umolchaniyu === "vpn" ? "direct" : "vpn");
                    setAdding(!adding);
                  }}
                >
                  <IkPlyus className="h-4 w-4" />
                  {adding ? "Закрыть" : "Добавить"}
                </Knopka>
              </header>

              {adding && (
                <div className="border-border bg-surface flex flex-col gap-3 rounded-xl border p-4">
                  {tab === "apps" ? (
                    <>
                      <Poisk
                        aria-label="Поиск приложения"
                        znachenie={query}
                        naVvod={setQuery}
                        placeholder="Найти среди запущенных"
                      />
                      <div className="border-border flex max-h-[220px] flex-col overflow-y-auto rounded-lg border" aria-label="Запущенные приложения">
                        {candidates.map((p) => (
                          <button
                            type="button"
                            key={p.put}
                            aria-label={`${p.imya}, ${p.put}`}
                            aria-pressed={path === p.put}
                            onClick={() => setPath(p.put)}
                            className={`flex min-w-0 flex-col gap-0.5 px-3 py-2 text-left transition-colors ${
                              path === p.put ? "bg-accent-soft" : "hover:bg-surface-hover"
                            }`}
                          >
                            <span className="text-foreground truncate text-sm font-medium">{p.imya}</span>
                            <span className="text-fg-muted truncate text-[13px]" title={p.put}>{sokratitPut(p.put)}</span>
                          </button>
                        ))}
                        {candidates.length === 0 && (
                          <p className="text-fg-muted px-3 py-6 text-center text-[13px]">
                            Не найдено. Выбери приложение на ПК или укажи путь ниже
                          </p>
                        )}
                      </div>
                      <div className="flex items-center gap-3">
                        <Pole
                          aria-label="Путь к приложению"
                          znachenie={path}
                          aktiven={!disabled}
                          naVvod={setPath}
                          placeholder="C:\Program Files\...\app.exe"
                          className="min-w-0 flex-1"
                        />
                        <Knopka
                          rang="vtoraya"
                          zhdyot={picking}
                          aktiven={!disabled && !!naVyborPrilozheniya}
                          onClick={() => void browse()}
                        >
                          {picking ? "Выбор…" : "Выбрать на ПК…"}
                        </Knopka>
                      </div>
                      {pickerError && (
                        <p className="text-danger text-[13px]" role="alert">
                          Не удалось выбрать приложение: {pickerError}
                        </p>
                      )}
                      <Flazhok
                        podpis="И запущенные им программы"
                        vkl={descendants}
                        naSmenu={setDescendants}
                      />
                    </>
                  ) : (
                    <Pole
                      aria-label="Домен сайта"
                      znachenie={domain}
                      naVvod={setDomain}
                      placeholder="example.org"
                    />
                  )}
                  <div className="flex items-center justify-end gap-3">
                    <div className="w-[170px]">
                      <RouteSelect
                        label="Маршрут нового правила"
                        value={route}
                        onChange={setRoute}
                      />
                    </div>
                    <Knopka
                      rang="glavnaya"
                      bolshaya
                      aktiven={
                        !disabled &&
                        (tab === "apps" ? path.trim() !== "" : domain.trim() !== "")
                      }
                      onClick={add}
                    >
                      Добавить в черновик
                    </Knopka>
                  </div>
                </div>
              )}

              {(tab === "apps" ? apps.length : domains.length) === 0 ? (
                <div className="border-border flex flex-col items-center gap-3 rounded-xl border border-dashed px-6 py-12 text-center">
                  <IkSsylka className="text-fg-faint h-7 w-7" />
                  <p className="text-foreground text-sm font-medium">Пока нет отдельных правил</p>
                  <p className="text-fg-muted max-w-[440px] text-[13px]">
                    Сейчас используется маршрут по умолчанию: {imyaMarshruta(trafik.po_umolchaniyu)}
                  </p>
                </div>
              ) : tab === "apps" ? (
                <div>
                  <div className={`${SETKA_PRILOZHENIY} max-[1099px]:hidden border-border text-fg-muted border-b px-3 pb-2.5 text-[13px]`}>
                    <span>Приложение</span>
                    <span>Запущенные программы</span>
                    <span>Маршрут</span>
                    <span className="text-right">Действие</span>
                  </div>
                  <ul>
                    {apps.map((app) => (
                      <li key={app.put} className={`${SETKA_PRILOZHENIY} border-border hover:bg-surface-hover border-b px-3 py-2.5 transition-colors`}>
                        <span className="col-span-3 min-[1100px]:col-span-1 flex min-w-0 flex-col gap-0.5">
                          <span className="text-foreground truncate text-sm font-medium">{app.imya}</span>
                          {/* Длинный путь теряет СЕРЕДИНУ, а не хвост: диск говорит, где
                              файл живёт, имя говорит, что это за файл, а папки посередине
                              человек и так не читает. Целиком в подсказке. */}
                          <span className="text-fg-muted truncate text-[13px]" title={app.put}>{sokratitPut(app.put)}</span>
                        </span>
                        <Flazhok
                          podpis="И запущенные им программы"
                          aktiven={!disabled}
                          vkl={app.potomki}
                          naSmenu={(v) =>
                            save({
                              ...trafik,
                              prilozheniya: apps.map((a) => (a.put === app.put ? { ...a, potomki: v } : a)),
                            })
                          }
                        />
                        <RouteSelect
                          label={`Маршрут ${app.imya}`}
                          value={app.marshrut}
                          disabled={disabled}
                          onChange={(r) =>
                            save({
                              ...trafik,
                              prilozheniya: apps.map((a) => (a.put === app.put ? { ...a, marshrut: r } : a)),
                            })
                          }
                        />
                        <div className="flex justify-end">
                          <Knopka
                            rang={remove === app.put ? "opasnaya" : "tekst"}
                            aktiven={!disabled}
                            onClick={() => {
                              if (remove === app.put)
                                save({ ...trafik, prilozheniya: apps.filter((a) => a.put !== app.put) });
                              else setRemove(app.put);
                            }}
                          >
                            {remove === app.put ? "Подтвердить" : "Удалить"}
                          </Knopka>
                        </div>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : (
                <div>
                  <div className={`${SETKA_SAYTOV} border-border text-fg-muted border-b px-3 pb-2.5 text-[13px]`}>
                    <span>Домен</span>
                    <span>Маршрут</span>
                    <span className="text-right">Действие</span>
                  </div>
                  <ul>
                    {domains.map((d) => (
                      <li key={d.domen} className={`${SETKA_SAYTOV} border-border hover:bg-surface-hover border-b px-3 py-2.5 transition-colors`}>
                        <span className="flex min-w-0 items-center gap-3">
                          <span className="border-border bg-elevated text-fg-muted flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border">
                            <IkSayt className="h-4 w-4" />
                          </span>
                          <span className="flex min-w-0 flex-col gap-0.5">
                            {/* Обрезка плюс подсказка: домен длиной в две сотни знаков
                                не должен ни выезжать за колонку, ни пропадать насовсем. */}
                            <span className="text-foreground truncate text-sm font-medium" title={d.domen}>{d.domen}</span>
                            <span className="text-fg-muted text-[13px]">Включая поддомены</span>
                          </span>
                        </span>
                        <RouteSelect
                          label={`Маршрут ${d.domen}`}
                          value={d.marshrut}
                          disabled={disabled}
                          onChange={(r) =>
                            save({
                              ...trafik,
                              domeny: domains.map((v) => (v.domen === d.domen ? { ...v, marshrut: r } : v)),
                            })
                          }
                        />
                        <div className="flex justify-end">
                          <Knopka
                            rang={remove === d.domen ? "opasnaya" : "tekst"}
                            aktiven={!disabled}
                            onClick={() => {
                              if (remove === d.domen)
                                save({ ...trafik, domeny: domains.filter((v) => v.domen !== d.domen) });
                              else setRemove(d.domen);
                            }}
                          >
                            {remove === d.domen ? "Подтвердить" : "Удалить"}
                          </Knopka>
                        </div>
                      </li>
                    ))}
                  </ul>
                </div>
              )}

              <OhvatPravil key={tab} vid={tab} trafik={trafik} katalog={pravila?.katalog} chernovik={dirty} ozhidayut={pravila?.trebuet_podyoma} bezRu={bezRu} killSwitch={status.kill_switch===true} disabled={disabled} proverit={proveritSoedineniya} proveritPrilozhenie={proveritPrilozhenie} vybratFayl={naVyborPrilozheniya} zamenit={(oldPath,newPath)=>{
                const key=newPath.trim().toLowerCase();
                if(apps.some(a=>a.put!==oldPath && a.put.toLowerCase()===key))throw new Error("Для этого файла уже есть правило. Измени его в списке приложений.");
                return save({...trafik,prilozheniya:apps.map(a=>a.put===oldPath?{...a,put:newPath.trim(),imya:newPath.trim().split(/[/\\]/).pop() || a.imya}:a)});
              }} naSbros={()=>save(tab==="apps"?{...trafik,prilozheniya:[]}:{...trafik,domeny:[]})}/>
              <div className="flex flex-col">
                {tab === "apps" && (
                  <Svorachivaemyy
                    zagolovok="Что попадает под правило приложения"
                    deti={
                      <div className="flex flex-col gap-2">
                        <p>
                          Под правило попадает выбранный файл и всё, что он запускает, включая
                          запущенное уже после включения VPN: лаунчер уводит за собой игру, а
                          браузер свои вкладки.
                        </p>
                        <p>
                          Программа, открытая отдельно, под это правило не попадёт. Affory запоминает
                          запуски, пока работает служба: переподключение VPN эти сведения не стирает.
                          Если программа успела закрыться до запуска службы, сведений о её запуске
                          может не быть. В таком случае добавь саму игру или приложение отдельным правилом.
                        </p>
                        <p>
                          Если хотя бы одно приложение направлено в VPN, Affory по умолчанию ищет
                          адреса сайтов через VPN: общий DNS Windows не позволяет надёжно определить приложение.
                          Явные правила сайтов и сервисов, локальные имена и российский список проверяются раньше.
                          Поэтому маршрут соединения и поиск адреса могут различаться.
                        </p>
                      </div>
                    }
                  />
                )}
                <Svorachivaemyy
                  zagolovok="Дополнительно"
                  deti={
                    tab === "apps" ? (
                      <p>
                        Правило по пути, а не по имени: две копии одного файла из разных папок это
                        два разных правила. Переименованный или перенесённый файл под правило
                        перестаёт попадать, путь придётся указать заново.
                      </p>
                    ) : (
                      <p>
                        Правило накрывает домен и все его поддомены: example.com действует и на
                        api.example.com. Программа, которая обращается к адресу без имени, под правило
                        сайта не попадает, для неё есть вкладка «Приложения».
                      </p>
                    )
                  }
                />
              </div>
            </div>
          )}

        </div>
      </div>
    </section>
  );
}
