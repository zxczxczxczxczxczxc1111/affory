import { useEffect, useRef, useState } from "react";
import { domenPopadaet, normalizovatProbuDomena, opisatDomen, type ProveritSoedineniya, type SnimokSoedineniy } from "../ohvat";
import { imyaMarshruta, type KatalogServisov, type PravilaTrafika } from "../trafik";
import { Knopka, Pole } from "./ui";

export function OhvatPravil({vid,trafik,katalog,chernovik,ozhidayut=false,disabled,naSbros,proverit}:{
  vid:"apps"|"sites"; trafik:PravilaTrafika; katalog?:KatalogServisov; chernovik:boolean;
  ozhidayut?:boolean; disabled:boolean; naSbros:()=>void; proverit?:ProveritSoedineniya;
}) {
  const [open,setOpen]=useState(false), [confirm,setConfirm]=useState(false);
  const [input,setInput]=useState(""), [busy,setBusy]=useState(false), [error,setError]=useState("");
  const [result,setResult]=useState<SnimokSoedineniy|null>(null);
  const epoch=useRef(0);
  useEffect(()=>()=>{epoch.current++;},[]);
  const sites=vid==="sites";
  const query=sites?normalizovatProbuDomena(input):input.trim();
  const count=sites?trafik.domeny.length:trafik.prilozheniya.length;
  const changeInput=(value:string)=>{epoch.current++;setInput(value);setResult(null);setError("");setBusy(false);};
  const inspect=async()=>{
    if (!proverit || busy || disabled) return;
    if (sites && input.trim() && !query) {setError("Укажи домен или ссылку HTTP/HTTPS");return;}
    const ticket=++epoch.current;
    setBusy(true);setError("");setResult(null);
    try {const snapshot=await proverit(sites?{domen:query}:{put:query});if(ticket===epoch.current)setResult(snapshot);}
    catch(e:unknown){if(ticket===epoch.current)setError(e instanceof Error?e.message:String(e));}
    finally{if(ticket===epoch.current)setBusy(false);}
  };
  const app=trafik.prilozheniya.find(a=>a.put.toLowerCase()===query.toLowerCase());
  return <section aria-label="Охват и проверка правил" className="border-border flex flex-col gap-3 border-t pt-4 text-[13px]">
    <div className="flex flex-wrap gap-2">
      <Knopka rang="vtoraya" aria-expanded={open} onClick={()=>setOpen(!open)}>{open?"Скрыть охват правил":"Показать охват правил"}</Knopka>
      <Knopka rang={confirm?"opasnaya":"vtoraya"} aktiven={!disabled && count>0} onClick={()=>{if(confirm){naSbros();setConfirm(false);}else setConfirm(true);}}>{confirm?"Подтвердить сброс в черновике":sites?"Сбросить правила сайтов":"Сбросить правила приложений"}</Knopka>
      {confirm && <Knopka rang="tekst" onClick={()=>setConfirm(false)}>Не сбрасывать</Knopka>}
    </div>
    {open && <div className="bg-surface border-border flex flex-col gap-3 rounded-xl border p-4">
      <p className="text-fg-secondary">{chernovik?"Охват черновика, ещё не применён":"Охват сохранённых правил"}</p>
      {!count && <p className="text-fg-muted">Явных правил {sites?"сайтов":"приложений"} нет</p>}
      <ul className="flex max-h-72 flex-col gap-3 overflow-y-auto">
        {sites?trafik.domeny.map(d=><li key={d.domen} className="flex flex-col gap-1">
          <div className="flex flex-wrap justify-between gap-2"><span className="text-foreground break-all">{d.domen}</span><span>{imyaMarshruta(d.marshrut)}</span></div>
          <span className="text-fg-muted">Сам домен и все поддомены</span>
          {trafik.domeny.filter(child=>child.domen!==d.domen && domenPopadaet(child.domen,d.domen)).map(child=><span key={child.domen} className="text-fg-secondary break-all">Более точное правило: {child.domen}, {imyaMarshruta(child.marshrut).toLowerCase()}</span>)}
        </li>):trafik.prilozheniya.map(a=><li key={a.put} className="flex flex-col gap-1">
          <div className="flex flex-wrap justify-between gap-2"><span className="text-foreground">{a.imya}</span><span>{imyaMarshruta(a.marshrut)}</span></div>
          <span className="text-fg-muted break-all">{a.put}</span>
          <span>{a.potomki?"Приложение и программы, которые оно запускает, если Affory успел заметить их запуск":"Только это приложение"}</span>
        </li>)}
      </ul>
      <p className="text-fg-muted">{sites?"Сначала действует правило приложения, затем сайта, затем сервиса. Например, отдельное правило для api.example.com важнее общего правила для example.com.":"Отдельное правило приложения действует первым. Например, если Steam открыл лаунчер, а тот запустил игру, сначала учитывается правило игры, затем лаунчера, затем Steam. Если запуск не удалось заметить, Affory не угадывает, какие программы связаны."}</p>
      <p className="text-fg-muted">Это настройки маршрута, а не результат сетевой проверки.</p>
    </div>}
    <form className="flex flex-col gap-3" onSubmit={e=>{e.preventDefault();void inspect();}}>
      <p className="text-foreground font-medium">Проверка по соединениям</p>
      <p className="text-fg-muted">Открой сайт или выполни действие в приложении, затем нажми «Проверить соединения». Оставь поле пустым, чтобы увидеть все текущие соединения.</p>
      <div className="flex flex-wrap gap-2">
        <Pole aria-label={sites?"Домен для проверки":"Путь приложения для проверки"} placeholder={sites?"api.example.org или ссылка":"Полный путь к файлу приложения (.exe)"} znachenie={input} naVvod={changeInput} className="min-w-[180px] flex-1"/>
        <Knopka rang="vtoraya" type="submit" aktiven={!disabled && !busy && !!proverit} zhdyot={busy}>Проверить соединения</Knopka>
      </div>
      {query && <p className="text-fg-secondary">{sites?opisatDomen(query,trafik,katalog):app?`${imyaMarshruta(app.marshrut)}: отдельное правило для ${app.imya}. Оно важнее правил программ, которые его запустили.`:"Отдельного правила для этого приложения нет. Оно может использовать правило программы, которая его запустила. По одному пути к файлу это проверить нельзя."}</p>}
      {chernovik && <p className="text-warn">Подсказка учитывает черновик. Проверка показывает текущие соединения, ещё без этих изменений.</p>}
      {!chernovik && ozhidayut && <p className="text-warn">Сохранённые правила ещё не применены. Проверка показывает соединения с прежними правилами.</p>}
      {error && <p role="alert" className="text-danger">{error}</p>}
      {busy && <p role="status">Проверяю текущие соединения</p>}
      {result && <div className="flex flex-col gap-2" role="status">
        <p>Соединения на {new Date(result.vremya).toLocaleTimeString("ru-RU")}</p>
        {!result.yadro?<p>Подключение не запущено. Подключи VPN и повтори проверку.</p>:!result.soedineniya.length?<p>Подходящих открытых соединений не обнаружено. Это не означает, что правило не работает: короткие соединения могли уже закрыться, а адрес сайта может быть не виден Affory.</p>:<ul className="border-border max-h-80 divide-y overflow-y-auto rounded-lg border">
          {result.soedineniya.map((c,index)=><li key={`${c.Id}-${index}`} className="flex flex-col gap-1 p-3">
            <span className="text-foreground break-all">{c.Host || c.Adres}:{c.Port}</span>
            <span className="break-all">{c.Protsess || "Не удалось определить приложение"}</span>
            <span>Куда идёт соединение: {c.Vyhod==="direct"?"напрямую":c.Vyhod.startsWith("srv-")?"через VPN":"не удалось определить"}</span>
            <details className="text-fg-muted break-all"><summary>Технические сведения</summary><p>Выход: {c.Vyhod || "неизвестен"}</p><p>Правило: {c.Pravilo || "не сообщено"}</p></details>
          </li>)}
        </ul>}
        {result.ogranichen && <p>Показаны первые 200 совпадений. Уточни фильтр.</p>}
      </div>}
      <p className="text-fg-muted">{sites?"Проверка example.com включает соединения с api.example.com и другими его поддоменами. Она не перебирает все возможные адреса.":"По указанному пути видны только соединения этого приложения. Чтобы увидеть программы, которые оно запустило, очисти поле и найди их в общем списке."} Здесь видно, куда направлено соединение, но это ещё не означает, что сайт ответил. Запись журнала не включается.</p>
    </form>
  </section>;
}
