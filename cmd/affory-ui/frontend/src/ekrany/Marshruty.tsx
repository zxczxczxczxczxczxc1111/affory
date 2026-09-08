import { useEffect, useRef, useState } from "react";
import type { PravilaProps } from "./Pravila";
import { imyaMarshruta, type Marshrut, type PravilaTrafika } from "../trafik";
import { Knopka, Pole, Tumbler } from "./ui";
import { IkonkaServisa } from "./IkonkaServisa";
import { Vybor } from "./Vybor";
import { slovoPosleChisla } from "../chisla";

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
    <div className="af-segment" role="group" aria-label="Трафик по умолчанию">
      <button
        type="button"
        aria-pressed={value === "vpn"}
        disabled={disabled}
        onClick={() => onChange("vpn")}
      >
        Всё через VPN
      </button>
      <button
        type="button"
        aria-pressed={value === "direct"}
        disabled={disabled}
        onClick={() => onChange("direct")}
      >
        Только выбранное
      </button>
    </div>
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

export function Marshruty({
  status,
  pravila,
  trafik,
  zapushchennye,
  naVyborPrilozheniya,
  naKomandu,
  zanyato = false,
}: PravilaProps & { trafik: PravilaTrafika }) {
  const [tab, setTab] = useState<"services" | "apps" | "sites">("services");
  const [adding, setAdding] = useState(false);
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
  const disabled =
    zanyato ||
    picking ||
    status.sostoyanie === "sluzhba-molchit" ||
    status.sostoyanie === "podnimaetsya" ||
    status.sostoyanie === "vosstanavlivaetsya";
  const save = (next: PravilaTrafika, bezRu = pravila?.bez_ru_spiska) => {
    if (!disabled)
      naKomandu("setRules", { trafik: next, bez_ru_spiska: bezRu });
  };
  const apps = trafik.prilozheniya ?? [],
    domains = trafik.domeny ?? [],
    services = trafik.servisy ?? [];
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
    if (tab === "apps")
      save({
        ...trafik,
        prilozheniya: [
          ...apps.filter(
            (p) => p.put.toLowerCase() !== path.trim().toLowerCase(),
          ),
          {
            put: path.trim(),
            imya: path.trim().split(/[/\\]/).pop() ?? "Приложение",
            potomki: descendants,
            marshrut: route,
          },
        ],
      });
    else
      save({
        ...trafik,
        domeny: [
          ...domains.filter((d) => d.domen !== domain.trim().toLowerCase()),
          { domen: domain.trim(), marshrut: route },
        ],
      });
    // Keep typed values available when validation fails; a rejected form is not amnesia.
  };
  return (
    <section className="af-routes" aria-label="Правила">
      <aside className="af-route-aside">
        <h2>Куда идёт трафик</h2>
        <VyborTrafika
          value={trafik.po_umolchaniyu}
          disabled={disabled}
          onChange={(value) => save({ ...trafik, po_umolchaniyu: value })}
        />
        <p className="af-note">
          {trafik.po_umolchaniyu === "vpn"
            ? "VPN для всего интернета. Добавьте приложения и сайты, которые должны работать напрямую."
            : "Обычное подключение для всего интернета. Выберите, что направить через VPN."}
        </p>
        <div className="af-route-summary">
          <b>{ruleCount}</b> {slovoPosleChisla(ruleCount, "правило", "правила", "правил")}
          <br />
          Явные маршруты сохраняются при смене режима.
        </div>
        {status.sostoyanie === "vyklyuchen" && (
          <p className="af-note">
            Правила начнут работать после нажатия на сферу.
          </p>
        )}
        {zanyato && (
          <p role="status" className="af-note">
            Применяем правила…
          </p>
        )}
        {pravila?.trebuet_podyoma && (
          <p role="status" className="af-note">
            Сохранено. Нужен повторный запуск подключения.
          </p>
        )}
        {status.kill_switch && (
          <p className="af-note">
            Прямые маршруты требуют отключить блокировку сети вне VPN в
            настройках защиты.
          </p>
        )}
      </aside>
      <div className="af-route-content">
        <nav className="af-route-tabs" aria-label="Виды правил">
          <button
            type="button"
            aria-pressed={tab === "services"}
            onClick={() => changeTab("services")}
          >
            Сервисы
          </button>
          <button
            type="button"
            aria-pressed={tab === "apps"}
            onClick={() => changeTab("apps")}
          >
            Приложения <small>{apps.length || ""}</small>
          </button>
          <button
            type="button"
            aria-pressed={tab === "sites"}
            onClick={() => changeTab("sites")}
          >
            Сайты <small>{domains.length || ""}</small>
          </button>
        </nav>
        {tab === "sites" && (
          <section className="af-ru-card" aria-label="Российские сайты">
            <div>
              <h3>Российские сайты напрямую</h3>
              <p>{status.kill_switch
                ? "Чтобы включить, отключите блокировку сети вне VPN в настройках защиты."
                : "Сайты из российского списка открываются без VPN. Ваши отдельные правила имеют приоритет."}</p>
            </div>
            <Tumbler testId="ru-spisok" podpis="Российские сайты напрямую"
              aktiven={!disabled && !status.kill_switch}
              vkl={pravila?.bez_ru_spiska !== true}
              naSmenu={(v) => save(trafik, !v)} />
          </section>
        )}
        {tab === "services" ? (
          <>
            <div className="af-pane-head">
              <div>
                <h3>Популярные сервисы</h3>
                <p>Домены и поддомены одним переключателем</p>
              </div>
            </div>
            <div className="af-services">
              {(pravila?.katalog?.servisy ?? []).map((service) => {
                const explicit = services.find((r) => r.id === service.id);
                const effective = explicit?.marshrut ?? trafik.po_umolchaniyu;
                return (
                  <article
                    className="af-service"
                    data-vpn={effective === "vpn"}
                    key={service.id}
                  >
                    <div className="af-service-head">
                      <IkonkaServisa id={service.id} imya={service.imya} />
                      <span className="af-service-name">
                        <b>{service.imya}</b>
                        <small>
                          {effective === "vpn" ? "Через VPN" : "Напрямую"}
                          {!explicit ? " по умолчанию" : ""}
                        </small>
                      </span>
                      <Tumbler
                        testId={`service-${service.id}`}
                        podpis={`${service.imya} через VPN`}
                        vkl={effective === "vpn"}
                        aktiven={!disabled}
                        naSmenu={(on) =>
                          save({
                            ...trafik,
                            servisy: [
                              ...services.filter((s) => s.id !== service.id),
                              {
                                id: service.id,
                                marshrut: on ? "vpn" : "direct",
                              },
                            ],
                          })
                        }
                      />
                    </div>
                    <details className="af-service-details">
                      <summary>
                        {service.domeny.length} {slovoPosleChisla(service.domeny.length, "домен", "домена", "доменов")} с поддоменами
                      </summary>
                      <ul>
                        {service.domeny.map((d) => (
                          <li key={d}>{d}</li>
                        ))}
                      </ul>
                      {explicit && (
                        <button
                          className="af-link"
                          type="button"
                          disabled={disabled}
                          onClick={() =>
                            save({
                              ...trafik,
                              servisy: services.filter(
                                (s) => s.id !== service.id,
                              ),
                            })
                          }
                        >
                          Вернуть маршрут по умолчанию
                        </button>
                      )}
                    </details>
                  </article>
                );
              })}
            </div>
            <p className="af-note">
              Приложение обращается напрямую к IP? Добавьте его во вкладке
              «Приложения».
            </p>
            <details className="af-details">
              <summary>Источник и особенности правил</summary>
              <p className="af-note">
                Каталог OpenCCK, версия{" "}
                {pravila?.katalog?.versiya ?? "неизвестна"}. Обновляется вместе
                с приложением. Отдельный домен имеет приоритет над набором
                сервиса. Браузер с защищённым DNS или ECH может скрыть имя
                сайта; для такого случая добавьте приложение целиком.
              </p>
            </details>
          </>
        ) : (
          <>
            <div className="af-pane-head">
              <div>
                <h3>
                  {tab === "apps" ? "Правила приложений" : "Правила сайтов"}
                </h3>
                <p>
                  {tab === "apps"
                    ? "Приложение вместе с дочерними процессами"
                    : "Домен и все его поддомены"}
                </p>
              </div>
              <Knopka
                rang="glavnaya"
                aktiven={!disabled}
                onClick={() => { pickerEpoch.current++; setPickerError(""); setPicking(false); setAdding(!adding); }}
              >
                {adding ? "Закрыть" : "+ Добавить"}
              </Knopka>
            </div>
            {adding && (
              <div className="af-rule-form">
                {tab === "apps" ? (
                  <>
                    <Pole
                      aria-label="Поиск приложения"
                      znachenie={query}
                      naVvod={setQuery}
                      placeholder="Найти среди запущенных…"
                    />
                    <div
                      className="af-process-list"
                      aria-label="Запущенные приложения"
                    >
                      {candidates.map((p) => (
                        <button
                          type="button"
                          key={p.put}
                          aria-label={`${p.imya}, ${p.put}`}
                          aria-pressed={path === p.put}
                          onClick={() => setPath(p.put)}
                        >
                          <b>{p.imya}</b>
                          <small>{p.put}</small>
                        </button>
                      ))}
                      {candidates.length === 0 && (
                        <p className="af-note">
                          Не найдено. Выберите приложение на ПК или укажите путь ниже.
                        </p>
                      )}
                    </div>
                    <div className="af-file-picker">
                      <Pole
                        aria-label="Путь к приложению"
                        znachenie={path}
                        aktiven={!disabled}
                        naVvod={setPath}
                        placeholder="C:\Program Files\…\app.exe"
                      />
                      <Knopka rang="vtoraya" aktiven={!disabled && !!naVyborPrilozheniya} onClick={() => void browse()}>
                        {picking ? "Выбираем…" : "Выбрать на ПК…"}
                      </Knopka>
                    </div>
                    {pickerError && <p className="af-picker-error" role="alert">Не удалось выбрать приложение: {pickerError}</p>}
                    <label className="af-check">
                      <input
                        type="checkbox"
                        checked={descendants}
                        onChange={(e) => setDescendants(e.target.checked)}
                      />
                      Включая дочерние процессы
                    </label>
                  </>
                ) : (
                  <Pole
                    aria-label="Домен сайта"
                    znachenie={domain}
                    naVvod={setDomain}
                    placeholder="example.org"
                  />
                )}
                <div className="af-form-actions">
                  <RouteSelect
                    label="Маршрут нового правила"
                    value={route}
                    onChange={setRoute}
                  />
                  <Knopka
                    rang="glavnaya"
                    aktiven={
                      !disabled &&
                      (tab === "apps"
                        ? path.trim() !== ""
                        : domain.trim() !== "")
                    }
                    onClick={add}
                  >
                    Сохранить правило
                  </Knopka>
                </div>
              </div>
            )}
            {(tab === "apps" ? apps.length : domains.length) === 0 && (
              <div className="af-empty">
                <b>Пока нет отдельных правил</b>
                <p>
                  Сейчас используется маршрут по умолчанию:{" "}
                  {imyaMarshruta(trafik.po_umolchaniyu)}.
                </p>
              </div>
            )}
            <div className="af-rules-list">
              {tab === "apps"
                ? apps.map((app) => (
                    <article className="af-app-rule" key={app.put}>
                      <div className="af-app-head">
                        <div className="af-app-copy">
                          <b>{app.imya}</b>
                          <small>{app.put}</small>
                        </div>
                        <RouteSelect
                          label={`Маршрут ${app.imya}`}
                          value={app.marshrut}
                          disabled={disabled}
                          onChange={(r) =>
                            save({
                              ...trafik,
                              prilozheniya: apps.map((a) =>
                                a.put === app.put ? { ...a, marshrut: r } : a,
                              ),
                            })
                          }
                        />
                      </div>
                      <div className="af-app-options">
                        <label className="af-check">
                          <input
                            type="checkbox"
                            disabled={disabled}
                            checked={app.potomki}
                            onChange={(e) =>
                              save({
                                ...trafik,
                                prilozheniya: apps.map((a) =>
                                  a.put === app.put
                                    ? { ...a, potomki: e.target.checked }
                                    : a,
                                ),
                              })
                            }
                          />
                          Включая дочерние процессы
                        </label>
                        <button
                          className="af-link"
                          disabled={disabled}
                          type="button"
                          onClick={() => {
                            if (remove === app.put)
                              save({
                                ...trafik,
                                prilozheniya: apps.filter(
                                  (a) => a.put !== app.put,
                                ),
                              });
                            else setRemove(app.put);
                          }}
                        >
                          {remove === app.put
                            ? "Подтвердить удаление"
                            : "Удалить"}
                        </button>
                      </div>
                    </article>
                  ))
                : domains.map((d) => (
                    <article className="af-app-rule af-app-head" key={d.domen}>
                      <div className="af-app-copy">
                        <b>{d.domen}</b>
                        <small>Включая поддомены</small>
                      </div>
                      <RouteSelect
                        label={`Маршрут ${d.domen}`}
                        value={d.marshrut}
                        disabled={disabled}
                        onChange={(r) =>
                          save({
                            ...trafik,
                            domeny: domains.map((v) =>
                              v.domen === d.domen ? { ...v, marshrut: r } : v,
                            ),
                          })
                        }
                      />
                      <button
                        className="af-link"
                        disabled={disabled}
                        type="button"
                        onClick={() => {
                          if (remove === d.domen)
                            save({
                              ...trafik,
                              domeny: domains.filter(
                                (v) => v.domen !== d.domen,
                              ),
                            });
                          else setRemove(d.domen);
                        }}
                      >
                        {remove === d.domen ? "Подтвердить" : "Удалить"}
                      </button>
                    </article>
                  ))}
            </div>
            {tab === "apps" && (
              <details className="af-details">
                <summary>Как работают дочерние процессы</summary>
                <p className="af-note">
                  Учитываются наблюдаемые потомки выбранного .exe, в том числе
                  запущенные позже. Приложение, запущенное отдельно,
                  автоматически в группу не попадает. Если родитель завершился
                  до запуска VPN, перезапустите приложение. При выборе VPN для
                  приложения DNS тоже разрешается через туннель.
                </p>
              </details>
            )}
          </>
        )}
        <details className="af-details">
          <summary>Дополнительно</summary>
          <label className="af-extra-row">
            <span>Журнал соединений</span>
            <Tumbler
              testId="zhurnal"
              podpis="Журнал соединений"
              aktiven={!disabled}
              vkl={status.zhurnal ?? false}
              naSmenu={(vkl) => naKomandu("setJournal", { vkl })}
            />
          </label>
          <button
            type="button"
            className="af-link"
            disabled={disabled}
            onClick={() => naKomandu("clearJournal", {})}
          >
            Очистить журнал
          </button>
        </details>
      </div>
    </section>
  );
}
