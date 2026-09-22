import { imyaMarshruta, type KatalogServisov, type PravilaTrafika } from "./trafik";

export interface FiltrSoedineniy { domen?: string; put?: string }
export interface Soedinenie {
  Id: string; Host: string; Adres: string; Port: number;
  Protsess: string; Vyhod: string; Pravilo: string; Nachalo: string;
}
export interface SnimokSoedineniy {
  yadro: boolean; vremya: string; ogranichen: boolean; soedineniya: Soedinenie[];
}
export type ProveritSoedineniya = (filter:FiltrSoedineniy)=>Promise<SnimokSoedineniy>;

export function normalizovatProbuDomena(input:string):string {
  let value=input.trim();
  if (!value) return "";
  try {
    const url=new URL(value.includes("://") ? value : `https://${value}`);
    if (!["https:","http:"].includes(url.protocol) || url.username || url.password) return "";
    value=url.hostname.toLowerCase().replace(/\.$/,"");
    return /^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/.test(value) ? value : "";
  } catch { return ""; }
}
export function domenPopadaet(host:string,rule:string):boolean {
  return host===rule || host.endsWith(`.${rule}`);
}
// Только доменная часть намерения. Правила приложения, DNS и системные
// исключения объясняются отдельно: здесь нет сведений о конкретном сокете.
export function opisatDomen(host:string,trafik:PravilaTrafika,katalog?:KatalogServisov):string {
  const own=[...trafik.domeny].sort((a,b)=>b.domen.split(".").length-a.domen.split(".").length).find(r=>domenPopadaet(host,r.domen));
  if (own) return `${imyaMarshruta(own.marshrut)}: правило ${own.domen}, включая поддомены`;
  for (const rule of trafik.servisy) {
    const service=katalog?.servisy.find(s=>s.id===rule.id);
    if (service?.domeny.some(d=>domenPopadaet(host,d))) return `${imyaMarshruta(rule.marshrut)}: сервис ${service.imya}`;
  }
  return `Явного доменного правила нет. Общий режим: ${imyaMarshruta(trafik.po_umolchaniyu).toLowerCase()}; российский список и системные исключения могут изменить маршрут`;
}

export function razobratSnimok(value:unknown):SnimokSoedineniy {
  if (!value || typeof value!=="object") throw new Error("Служба не вернула снимок соединений");
  const v=value as Record<string,unknown>;
  if (typeof v.yadro!=="boolean" || typeof v.ogranichen!=="boolean" || typeof v.vremya!=="string" || !Number.isFinite(Date.parse(v.vremya)) || !Array.isArray(v.soedineniya)) throw new Error("Некорректный снимок соединений");
  const soedineniya=v.soedineniya.map((row:unknown)=>{
    if (!row || typeof row!=="object") throw new Error("Некорректная запись соединения");
    const c=row as Record<string,unknown>;
    if (typeof c.Id!=="string" || typeof c.Host!=="string" || typeof c.Adres!=="string" || typeof c.Port!=="number" || typeof c.Protsess!=="string" || typeof c.Vyhod!=="string" || typeof c.Pravilo!=="string" || typeof c.Nachalo!=="string") throw new Error("Некорректная запись соединения");
    return {Id:c.Id,Host:c.Host,Adres:c.Adres,Port:c.Port,Protsess:c.Protsess,Vyhod:c.Vyhod,Pravilo:c.Pravilo,Nachalo:c.Nachalo};
  });
  return {yadro:v.yadro,vremya:v.vremya,ogranichen:v.ogranichen,soedineniya};
}
