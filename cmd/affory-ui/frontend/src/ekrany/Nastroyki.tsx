import { useEffect, useState } from "react";
import type { OtkazNaEkrane, RezultatProverki, Sostoyanie, StatusOtvet } from "../protokol";
import { otlozhenaDo } from "./Pravila";
import { Udalenie } from "./Udalenie";
import { Karta, Knopka, Kolonka, Neudacha, Pole, Razdel, Ryad, Shapka, Tumbler } from "./ui";
import { obyom } from "./Glavnyy";

// Settings tab. 4.8 brought the two launch switches, 4.7 the uninstall, 4.11
// the rest: the §9.2 defaults shown rather than implied, the "all traffic"
// mode with its warning, the local proxy port, and the three deferred
// checks (leaks, exit address, update) disabled with the wave from hello.
// Rare settings live here in sections; there is no second page.

export interface NastroykiProps {
  status: StatusOtvet;
  /** Deferred commands with wave numbers, from hello. `null` until it answers. */
  otlozheno: Record<string, number> | null;
  /** Sends a service command; the answer folds into `status` upstream. */
  naKomandu: (komanda: string, telo: unknown) => void;
  naUdalenie: (steretKlyuchi: boolean) => void;
  /** Whether the service answers at all. Absent means "read it off the
   *  status", which is what the screen did before the shell told it. */
  svyaz?: "est" | "net";
  /** Reconnects to the service. The one action a screen without a service
   *  can still offer. */
  povtorit?: () => void;
  /** Last checkLeaks answer; `null` until the button is pressed. */
  proverka?: RezultatProverki | null;
  /** Why the last check brought nothing. App used to write `proverka` only
   *  on success, so a failed check left the PREVIOUS one on screen as if it
   *  were current (03.09.2026). */
  proverkaOtkaz?: OtkazNaEkrane | null;
  /** Writes the whole set to a file, and reads it back. Administrator only
   *  (owner 03.09.2026: secrets leaving the machine stay behind the UAC).
   *  The PASSWORD comes from here, the path from the shell's own dialog: the
   *  screen is not allowed near the bridge, and the bridge never sees the
   *  password. */
  vyvestiProfil?: (parol: string) => void;
  vvestiProfil?: (parol: string) => void;
  /** Outcome of the last attempt in the person's words. */
  itogProfilya?: string | null;
  /** Wave 6.5: opens the native archive dialog; the path goes to installUpdate from App. */
  naObnovlenie?: () => void;
  /** Last checkExitIp answer. Shown in the row: a press must change something on screen. */
  adresVyhoda?: AdresVyhoda | null;
  /** Последний замер полосы. Кнопка тратит настоящий трафик человека, и
   *  молчание после неё читается как «ничего не делает». */
  zamerPolosy?: ZamerPolosy | null;
}

/** Замер полосы, обе стороны. Число и его отсутствие здесь разные вещи:
 *  `null` значит «не измерено», а ноль значил бы «канала нет» и уехал бы в
 *  объявление, где ноль это Brutal с нулевой оценкой канала. */
export interface ZamerPolosy {
  mbitVniz: number | null;
  sovetVniz: number | null;
  mbitVverh: number | null;
  sovetVverh: number | null;
  otkazVverh: string;
  cherezTunnel: boolean;
  vremya: string;
}

/** Мбит/с одним числом. Десятые оставлены: разница между 87 и 87.4 не важна,
 *  а между 0.4 и 0 важна очень. */
function mbit(v: number | null): string {
  return v === null ? "не измерено" : `${v.toFixed(1)} Мбит/с`;
}

export interface AdresVyhoda {
  adres: string;
  cherez: "tunnel" | "napryamuyu";
  vremya: string;
}

function tekstAdresa(a: AdresVyhoda, podnyat: boolean): string {
  if (a.cherez !== "tunnel") return `напрямую, туннель не поднят: ${a.adres}, ${a.vremya}`;
  // The measurement was true when it was taken. After a disconnect the line
  // "через туннель" is a claim about NOW, and it is a false one (03.09.2026).
  return podnyat
    ? `через туннель: ${a.adres}, ${a.vremya}`
    : `измерено в ${a.vremya}, туннель с тех пор опущен: ${a.adres}`;
}

/** "12:03" for the moment an answer landed. The service does not stamp
 *  checkLeaks, and the screen must not pass a stale report off as fresh. */
function chasyMinuty(): string {
  return new Date().toLocaleTimeString("ru-RU", { hour: "2-digit", minute: "2-digit" });
}

const ITOG: Record<string, string> = {
  ok: "в порядке",
  utechka: "утечка",
  ne_izmereno: "не измерено",
  ne_vidim: "не проверяется",
};

export function Nastroyki({
  status, otlozheno, svyaz, povtorit, naKomandu, naUdalenie,
  proverka = null, proverkaOtkaz = null, naObnovlenie, adresVyhoda = null, zamerPolosy = null,
  vyvestiProfil, vvestiProfil, itogProfilya = null,
}: NastroykiProps) {
  // The password lives exactly as long as this screen does, goes into the body
  // of one command and nowhere else: not a file name, not an argument, not a
  // log line. It is deliberately NOT lifted to App.
  const [parolProfilya, zadatParolProfilya] = useState("");
  const [dialog, zadatDialog] = useState(false);
  // The mode question waits for an answer: switching re-raises the tunnel
  // and drops every connection, which is not what a toggle usually does.
  const [vopros, zadatVopros] = useState<boolean | null>(null);
  // Полоса живёт строками, а не числами: пустое поле это «не введено», а не
  // ноль, и ноль отсюда уходит в службу только по кнопке «Снять».
  const [vverh, zadatVverh] = useState("");
  const [vniz, zadatVniz] = useState("");
  const [mishen, zadatMishen] = useState("");
  const [mishenVverh, zadatMishenVverh] = useState("");
  const objavlena = (status.polosa_vverh ?? 0) > 0 && (status.polosa_vniz ?? 0) > 0;
  // Пара или ничего: Brutal управляет обоими направлениями сразу, и одно число
  // означало бы объявленный ноль во втором.
  const paraGodna = Number(vverh) > 0 && Number(vniz) > 0;
  const svyazNet = svyaz !== undefined ? svyaz === "net" : status.sostoyanie === "sluzhba-molchit";
  const aktiven = !svyazNet;
  const zhdyomHello = otlozheno === null;
  // Every grey button says why it is grey. The three checks used to go grey
  // in silence while the explanation beside them showed its usual text.
  const pochemuSero = (svoya: string | undefined): string | undefined =>
    svyazNet ? "служба не отвечает" : zhdyomHello ? "служба ещё не ответила" : svoya;
  const mozhnoZvat = !svyazNet && !zhdyomHello;
  const podnyat = status.sostoyanie === "podnyat";
  const killSwitch = status.kill_switch ?? false;
  const nahodka = status.obnovlenie ?? null;
  // The service accepts the mode only over a raised tunnel (killswitch.go);
  // switching it off is allowed in any state.
  const mozhnoRezhim = aktiven;
  // The screen remembers WHICH tunnel state produced the answer it shows: a
  // leak report taken over a raised tunnel says nothing about a dropped one.
  const [snyato, zadatSnyato] = useState<{ sostoyanie: Sostoyanie; vremya: string } | null>(null);
  useEffect(() => {
    if (proverka) zadatSnyato({ sostoyanie: status.sostoyanie, vremya: chasyMinuty() });
    else zadatSnyato(null);
    // The tunnel state is captured at the moment of the answer on purpose:
    // adding it to the deps would re-stamp a stale report as fresh.

  }, [proverka]);
  const ustarela = proverka !== null && snyato !== null && snyato.sostoyanie !== status.sostoyanie;
  const pokazatPunkty = proverka !== null && !proverkaOtkaz && !ustarela;

  const utechki = otlozhenaDo(otlozheno, "checkLeaks");
  const adresOtlozhen = otlozhenaDo(otlozheno, "checkExitIp");
  const obnovlenie = otlozhenaDo(otlozheno, "installUpdate");

  return (
    <Kolonka aria-label="Настройки">
      {/* No summary line: versiya_sluzhby is the protocol number, and "служба 1"
          under the title told the human nothing (owner, 02.09.2026). */}
      <Shapka zagolovok="Настройки" svodka={svyazNet ? "служба не отвечает" : undefined}>
        {svyazNet && povtorit && (
          <Knopka rang="vtoraya" testId="povtorit-svyaz" onClick={povtorit}>Повторить</Knopka>
        )}
      </Shapka>

      <Razdel nazvanie="запуск">
        <Karta>
          <Ryad nazvanie="запускать при входе в Windows" poyasnenie="программа поднимается в трее, окно не открывается" aktiven={aktiven}>
            <Tumbler
              testId="avtozapusk"
              podpis="запускать при входе в Windows"
              vkl={status.avtozapusk ?? false}
              aktiven={aktiven}
              naSmenu={(vkl) => naKomandu("setAutostart", { vkl })}
            />
          </Ryad>
          <Ryad nazvanie="подключаться при старте" poyasnenie="туннель поднимает служба, до входа в систему" aktiven={aktiven}>
            <Tumbler
              testId="pri-starte"
              podpis="подключаться при старте"
              vkl={status.podklyuchat_pri_starte ?? false}
              aktiven={aktiven}
              naSmenu={(vkl) => naKomandu("setConnectOnStart", { vkl })}
            />
          </Ryad>
        </Karta>
      </Razdel>

      <Razdel nazvanie="защита">
        {vopros !== null ? (
          <section role="dialog" aria-label="Смена режима" className="border-border bg-surface flex flex-col gap-3 rounded-lg border p-5">
            <h3 className="text-foreground text-base font-semibold">
              {vopros ? "включить режим «весь трафик»" : "выключить режим «весь трафик»"}
            </h3>
            <p className="text-fg-secondary text-sm">
              туннель переподнимается под новый режим, все соединения разорвутся и поднимутся заново.
              загрузки и звонки оборвутся
            </p>
            <div className="flex gap-2">
              <Knopka rang="glavnaya" testId="podtverdit-rezhim" onClick={() => { naKomandu("setKillSwitch", { vkl: vopros }); zadatVopros(null); }}>
                {vopros ? "Включить" : "Выключить"}
              </Knopka>
              <Knopka rang="tekst" testId="otmena-rezhima" onClick={() => zadatVopros(null)}>Отмена</Knopka>
            </div>
          </section>
        ) : (
          <Karta>
            <Ryad
              testId="ves-trafik-ryad"
              nazvanie="Блокировать сеть при обрыве VPN"
              poyasnenie={
                mozhnoRezhim || !aktiven
                  ? "Действует во время подключения. После отключения сферы сеть освобождается."
                  : "Защита включится вместе с VPN."
              }
              aktiven={mozhnoRezhim}
            >
              <Tumbler
                testId="ves-trafik"
                podpis="весь трафик только через туннель"
                vkl={killSwitch}
                aktiven={mozhnoRezhim}
                naSmenu={(vkl) => { if (podnyat) zadatVopros(vkl); else naKomandu("setKillSwitch", { vkl }); }}
              />
            </Ryad>
            <Ryad
              testId="proksi"
              nazvanie="локальный прокси"
              poyasnenie={
                status.port_proksi
                  ? `127.0.0.1:${status.port_proksi} · http и socks, для программ, которые ходят через прокси`
                  : podnyat
                    ? "не поднят: порт занят другой программой, туннель это не задевает"
                    : "поднимается вместе с туннелем"
              }
              aktiven={aktiven}
            />
          </Karta>
        )}

        {/* Полоса канала. Нужна ровно одному протоколу, hysteria2, у которого
            объявление полосы и ЕСТЬ переключатель Brutal: отдельного флага нет.
            Поэтому здесь не «ограничить скорость», а «сказать протоколу, какой
            канал он делит».

            Источников два. Ссылка сервера может нести upmbps и downmbps сама,
            и наша подписка так и делает для входа hy2-brutal. Эти поля её
            перебивают: ссылку пишет держатель сервера, а тут человек говорит
            про СВОЙ домашний канал, которым Brutal и управляет. Пустые поля
            значат «взять из ссылки», а не «BBR». */}
        <details className="af-details af-settings-details"><summary>Параметры Hysteria2 и ручной замер</summary><Karta testId="polosa">
          <Ryad
            nazvanie="полоса канала"
            poyasnenie={
              objavlena
                ? `${status.polosa_vverh} вверх · ${status.polosa_vniz} вниз, Мбит · включает Brutal у hysteria2`
                : "Автоматически из ссылки сервера. Ручные значения включают Brutal; завышенные могут ухудшить соединение."
            }
            aktiven={aktiven}
            lomat
          >
            <div className="flex items-center gap-2">
              <Pole
                testId="polosa-vverh"
                tip="text"
                znachenie={vverh}
                naVvod={(z) => zadatVverh(z.replace(/[^0-9]/g, ""))}
                placeholder="вверх"
                aktiven={aktiven}
                aria-label="полоса вверх, Мбит"
                className="w-24"
              />
              <Pole
                testId="polosa-vniz"
                tip="text"
                znachenie={vniz}
                naVvod={(z) => zadatVniz(z.replace(/[^0-9]/g, ""))}
                placeholder="вниз"
                aktiven={aktiven}
                aria-label="полоса вниз, Мбит"
                className="w-24"
              />
              <Knopka
                rang="vtoraya"
                testId="sohranit-polosu"
                aktiven={aktiven && paraGodna}
                onClick={() => naKomandu("setBandwidth", { vverh: Number(vverh), vniz: Number(vniz) })}
              >
                Сохранить
              </Knopka>
              {objavlena && (
                <Knopka
                  rang="vtoraya"
                  testId="snyat-polosu"
                  aktiven={aktiven}
                  onClick={() => {
                    zadatVverh("");
                    zadatVniz("");
                    naKomandu("setBandwidth", { vverh: 0, vniz: 0 });
                  }}
                >
                  Снять
                </Knopka>
              )}
            </div>
          </Ryad>
          {/* Замер вместо ввода. Число для Brutal обязано быть измерением: он
              шлёт ровно с объявленной скоростью, и цену завышения платит канал
              человека, который своей полосы не знает.

              Мишень задаётся ЯВНО и умолчания не имеет. Клиент не ходит
              самовольно на чужой хост и не тратит трафик мобильного тарифа без
              спроса: минута скачивания это десятки мегабайт. */}
          <Ryad
            nazvanie="измерить полосу"
            poyasnenie="Для собственного сервера измерений. Обычная страница сайта не принимает тестовую загрузку."
            aktiven={aktiven}
            lomat
          >
            <div className="flex items-center gap-2">
              <Pole
                testId="polosa-mishen"
                tip="text"
                znachenie={mishen}
                naVvod={zadatMishen}
                placeholder="https://адрес/большой-файл"
                aktiven={aktiven}
                aria-label="мишень замера приёма"
                className="w-56"
              />
              {/* Вторая мишень отдельная, потому что направления живут по
                  разным адресам: у speed.cloudflare.com это __down и __up.
                  Пустое поле значит «отдачу не мерим», и замер приёма всё
                  равно состоится. */}
              <Pole
                testId="polosa-mishen-vverh"
                tip="text"
                znachenie={mishenVverh}
                naVvod={zadatMishenVverh}
                placeholder="https://адрес/приём-заливки"
                aktiven={aktiven}
                aria-label="мишень замера отдачи"
                className="w-56"
              />
              <Knopka
                rang="vtoraya"
                testId="izmerit-polosu"
                aktiven={aktiven && mishen.trim() !== ""}
                onClick={() => {
                  const adres = mishen.trim();
                  if (adres === "") return;
                  naKomandu("measureBandwidth", {
                    adres, adres_vverh: mishenVverh.trim(), potokov: 4, sekund: 10,
                  });
                }}
              >
                Измерить
              </Knopka>
            </div>
          </Ryad>
          {/* Результат прямо под кнопкой. Замер тратит десятки мегабайт, и
              молчание после него человек читает как сломанную кнопку, а
              значит жмёт ещё раз (владелец, 03.09.2026). */}
          {zamerPolosy && (
            <div className="border-border border-t p-4" data-testid="zamer-polosy">
              <p className="text-sm">
                приём {mbit(zamerPolosy.mbitVniz)} · отдача {mbit(zamerPolosy.mbitVverh)}
                {" · "}
                {zamerPolosy.cherezTunnel ? "через туннель" : "напрямую, туннель не поднят"}
                {" · "}
                {zamerPolosy.vremya}
              </p>
              {zamerPolosy.otkazVverh !== "" && (
                <p className="text-fg-muted mt-1 text-xs" data-testid="zamer-otkaz-vverh">
                  отдача не измерена: {zamerPolosy.otkazVverh}
                </p>
              )}
              <div className="mt-3 flex items-center gap-2">
                {/* Объявляется СОВЕТ, а не замер. Запас вниз и есть весь смысл:
                    Brutal шлёт ровно с объявленной скоростью, и завышение
                    стоило на стенде 10% полосы и 30% задержки. */}
                <Knopka
                  rang="vtoraya"
                  testId="obyavit-polosu"
                  aktiven={aktiven && zamerPolosy.sovetVniz !== null && zamerPolosy.sovetVverh !== null}
                  onClick={() => {
                    if (zamerPolosy.sovetVniz === null || zamerPolosy.sovetVverh === null) return;
                    zadatVverh(String(zamerPolosy.sovetVverh));
                    zadatVniz(String(zamerPolosy.sovetVniz));
                    naKomandu("setBandwidth", { vverh: zamerPolosy.sovetVverh, vniz: zamerPolosy.sovetVniz });
                  }}
                >
                  {zamerPolosy.sovetVverh !== null && zamerPolosy.sovetVniz !== null ? `Применить ${zamerPolosy.sovetVverh} / ${zamerPolosy.sovetVniz} Мбит/с к Hysteria2` : "Применить к Hysteria2"}
                </Knopka>
                {zamerPolosy.sovetVverh === null && (
                  <span className="text-fg-muted text-xs">
                    Для применения нужен успешный замер в обе стороны.
                  </span>
                )}
              </div>
            </div>
          )}
        </Karta></details>
      </Razdel>

      <details className="af-details af-settings-details"><summary>Диагностика соединения</summary><Razdel nazvanie="проверка">
        <Karta testId="proverka">
          <Ryad
            nazvanie="утечки"
            poyasnenie={pochemuSero(utechki) ?? "чей адрес видит интернет, куда уходит DNS"}
            aktiven={aktiven && !utechki}
          >
            <Knopka rang="vtoraya" testId="proverit-utechki" aktiven={mozhnoZvat && !utechki} onClick={() => naKomandu("checkLeaks", {})}>
              Проверить
            </Knopka>
          </Ryad>
          {proverkaOtkaz && (
            <div className="border-border border-t p-4">
              <Neudacha
                testId="otkaz-proverki"
                kod={proverkaOtkaz.kod}
                tekst={proverkaOtkaz.tekst}
                deystvie={mozhnoZvat && !utechki ? () => naKomandu("checkLeaks", {}) : undefined}
                podpisDeystviya="Повторить"
              />
            </div>
          )}
          {ustarela && !proverkaOtkaz && (
            <Ryad
              testId="proverka-ustarela"
              nazvanie="прошлый результат больше не отвечает за сейчас"
              poyasnenie="состояние туннеля сменилось после проверки, проверь заново"
              aktiven={false}
            />
          )}
          {pokazatPunkty && snyato && (
            <div className="border-border border-t px-4 py-3">
              <p className="text-fg-muted mb-1 text-xs" data-testid="proverka-snyata">проверено в {snyato.vremya}</p>
              <ul data-testid="punkty" className="flex max-h-64 flex-col gap-1 overflow-y-auto text-sm">
                {proverka.punkty.map((p) => (
                  <li key={p.imya} data-itog={p.itog} className={p.itog === "utechka" ? "text-warn break-words" : p.itog === "ok" ? "break-words" : "text-fg-muted break-words"}>
                    <span className="font-medium">{p.imya}</span>
                    {" · "}
                    {ITOG[p.itog] ?? p.itog}
                    {" · "}
                    <span className="text-fg-muted">{p.tekst}</span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          <Ryad
            testId="adres-vyhoda"
            nazvanie="адрес выхода"
            poyasnenie={pochemuSero(adresOtlozhen) ?? (adresVyhoda ? tekstAdresa(adresVyhoda, podnyat) : "по кнопке и при смене сервера, не по таймеру")}
            aktiven={aktiven && !adresOtlozhen}
          >
            <Knopka rang="vtoraya" testId="proverit-adres" aktiven={mozhnoZvat && !adresOtlozhen} onClick={() => naKomandu("checkExitIp", {})}>
              Проверить
            </Knopka>
          </Ryad>
        </Karta>
      </Razdel>

      </details>
      <Razdel nazvanie="обновление">
        <Karta>
          {/* The service checks the update server daily on its own; this row
              shows what it found and lets the human act. The archive-from-disk
              path below stays as the second way in. */}
          <Ryad
            testId="obnovlenie"
            nazvanie={nahodka ? `есть ${nahodka.versiya}, ${obyom(nahodka.razmer)}` : `программа ${status.versiya_programmy ?? "dev"}`}
            poyasnenie={
              pochemuSero(undefined) ??
              (nahodka
                ? "служба скачает архив, сверит хеш и перезапустится; при неудаче за 20 секунд остаётся прежняя версия"
                : status.obnovlenie_provereno
                  ? `проверено ${vremya(status.obnovlenie_provereno)}, новее нет; проверяется раз в сутки`
                  : "ещё не проверялось; служба проверяет раз в сутки")
            }
            aktiven={aktiven}
          >
            {nahodka ? (
              <Knopka rang="glavnaya" testId="ustanovit-obnovlenie" aktiven={mozhnoZvat} onClick={() => naKomandu("downloadUpdate", {})}>
                Установить
              </Knopka>
            ) : (
              <Knopka rang="vtoraya" testId="proverit-versiyu" aktiven={mozhnoZvat} onClick={() => naKomandu("checkUpdate", {})}>
                Проверить
              </Knopka>
            )}
          </Ryad>
          <Ryad
            testId="arhiv-sborki"
            nazvanie="архив сборки"
            poyasnenie={pochemuSero(obnovlenie) ?? "архив с файлом .sha256 рядом; второй путь, когда сервер обновлений недоступен"}
            aktiven={aktiven && !obnovlenie}
          >
            <Knopka rang="vtoraya" testId="proverit-obnovlenie" aktiven={mozhnoZvat && !obnovlenie} onClick={() => (naObnovlenie ? naObnovlenie() : naKomandu("installUpdate", {}))}>
              Выбрать архив
            </Knopka>
          </Ryad>
        </Karta>
      </Razdel>

      <details className="af-details af-settings-details"><summary>Профиль и обслуживание</summary><div>
      {(vyvestiProfil || vvestiProfil) && (
        <Razdel nazvanie="профиль">
          <Karta>
            <Ryad
              nazvanie="пароль профиля"
              poyasnenie={pochemuSero(undefined) ?? "нужен и на вывод, и на ввод; нигде не сохраняется и в журнал не пишется"}
              aktiven={mozhnoZvat}
            >
              <input
                type="password"
                data-testid="parol-profilya"
                aria-label="пароль профиля"
                autoComplete="off"
                value={parolProfilya}
                onChange={(e) => zadatParolProfilya(e.target.value)}
                className="bg-fill-subtle border-border-hover text-foreground placeholder:text-fg-faint h-8 min-w-0 rounded-md border px-2.5 text-[13px]"
              />
            </Ryad>
            {vyvestiProfil && (
              <Ryad
                nazvanie="вывести профиль"
                poyasnenie={pochemuSero(undefined) ?? "серверы, подписка и правила одним файлом; ключи внутри, поэтому файл хранить как пароль; только для администратора машины"}
                aktiven={mozhnoZvat}
              >
                <Knopka
                  rang="vtoraya"
                  testId="vyvesti-profil"
                  aktiven={mozhnoZvat && parolProfilya !== ""}
                  onClick={() => vyvestiProfil(parolProfilya)}
                >
                  Вывести
                </Knopka>
              </Ryad>
            )}
            {vvestiProfil && (
              <Ryad
                nazvanie="ввести профиль"
                poyasnenie={pochemuSero(undefined) ?? "заменит серверы, подписку и правила целиком; только для администратора машины"}
                aktiven={mozhnoZvat}
              >
                <Knopka
                  rang="vtoraya"
                  testId="vvesti-profil"
                  aktiven={mozhnoZvat && parolProfilya !== ""}
                  onClick={() => vvestiProfil(parolProfilya)}
                >
                  Ввести
                </Knopka>
              </Ryad>
            )}
            {itogProfilya && (
              // The outcome belongs next to the button that produced it: the
              // banner is for refusals, and a success has no banner at all.
              <Ryad testId="itog-profilya" nazvanie={itogProfilya} aktiven={false} lomat />
            )}
          </Karta>
        </Razdel>
      )}

      <Razdel nazvanie="удаление">
        {dialog ? (
          <Udalenie naUdalenie={naUdalenie} naOtmenu={() => zadatDialog(false)} />
        ) : (
          <Karta>
            <Ryad nazvanie="удалить программу" poyasnenie="служба, адаптер и файлы; ключи по выбору">
              <Knopka rang="opasnaya" testId="otkryt-udalenie" onClick={() => zadatDialog(true)}>
                Удалить
              </Knopka>
            </Ryad>
          </Karta>
        )}
      </Razdel>
      </div></details>
    </Kolonka>
  );
}

/** "03.09.2026, 13:00" from an ISO stamp; the raw string when it does not parse. */
function vremya(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString("ru-RU", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });
}
