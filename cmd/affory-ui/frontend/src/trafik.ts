import { slovoPosleChisla } from "./chisla";

// Routes describe intent explicitly; changing a default must not flip every switch.
export type Marshrut = "vpn" | "direct";
export interface PraviloPrilozheniya {
  put: string;
  imya: string;
  potomki: boolean;
  marshrut: Marshrut;
}
export interface PraviloDomena {
  domen: string;
  marshrut: Marshrut;
}
export interface PraviloServisa {
  id: string;
  marshrut: Marshrut;
  /** Пути клиента сервиса среди запущенных программ. Имя файла знает каталог,
   *  путь - только машина: у Discord в нём номер сборки, у лаунчеров диск
   *  установки. Пусто значит «клиент не запущен», и маршрут держится на
   *  доменах. */
  programmy?: string[];
}
export interface PravilaTrafika {
  po_umolchaniyu: Marshrut;
  prilozheniya: PraviloPrilozheniya[];
  domeny: PraviloDomena[];
  servisy: PraviloServisa[];
}

export interface ChernovikPravil {
  trafik: PravilaTrafika;
  bezRu: boolean;
  baza: string;
  reviziya?: string;
}

export function snimokPravil(trafik: PravilaTrafika, bezRu: boolean): string {
  return JSON.stringify({trafik,bezRu});
}
export interface KatalogServisov {
  versiya: string;
  istochnik: string;
  servisy: { id: string; imya: string; domeny: string[]; programmy?: string[]; istochnik?: string }[];
}
/** Правила приложений и программы включённых сервисов одним списком, в порядке
 *  применения: свои правила первыми. Ровно это же делает служба при сборке
 *  конфига, и расходиться им нельзя - иначе проба показывает один маршрут, а
 *  ядро выбирает другой. */
export function programmyTrafika(trafik: PravilaTrafika): PraviloPrilozheniya[] {
  const itog = [...trafik.prilozheniya];
  for (const s of trafik.servisy) {
    for (const put of s.programmy ?? []) {
      // Охват запускаемых программ у карточки включён всегда: Steam это
      // лаунчер и игры, которые он запускает.
      itog.push({put, imya: put.split(/[/\\]/).pop() ?? put, potomki: true, marshrut: s.marshrut});
    }
  }
  return itog;
}

export const imyaMarshruta = (route: Marshrut) =>
  route === "vpn" ? "Через VPN" : "Напрямую";

export type VyborServisa = Marshrut | "inherit";
/** Пути клиента приезжают ВМЕСТЕ с маршрутом: карточка сервиса это домены и
 *  программа сразу, и включать их по отдельности человеку негде.
 *
 *  Пустой список путей не стирает прежние: программу могли закрыть между
 *  включением карточки и сменой маршрута, и потерять правило из-за этого
 *  значило бы молча снять маршрут с программы. */
export function zadatMarshrutServisa(trafik:PravilaTrafika,id:string,value:VyborServisa,programmy?:string[]):PravilaTrafika {
  if(value==="inherit") return {...trafik,servisy:trafik.servisy.filter(s=>s.id!==id)};
  const bylo=trafik.servisy.find(s=>s.id===id);
  const puti=programmy?.length?programmy:bylo?.programmy;
  const pravilo:PraviloServisa={id,marshrut:value,...(puti?.length?{programmy:puti}:{})};
  return {...trafik,servisy:bylo?trafik.servisy.map(s=>s.id===id?pravilo:s):[...trafik.servisy,pravilo]};
}

// Перечисление словами, а не парами «слово: число». «Напрямую настроены:
// приложения: 2; сайты: 1» ставило двоеточие внутрь двоеточия и читалось
// строкой отчёта (владелец, 23.09.2026).
export function prichinaPryamogoTrafika(trafik:PravilaTrafika):string|null {
  const parts:string[]=[];
  if(trafik.po_umolchaniyu==="direct")parts.push("общий режим «Только выбранное»");
  const apps=trafik.prilozheniya.filter(r=>r.marshrut==="direct").length;
  const sites=trafik.domeny.filter(r=>r.marshrut==="direct").length;
  const services=trafik.servisy.filter(r=>r.marshrut==="direct").length;
  if(apps)parts.push(`${apps} ${slovoPosleChisla(apps,"приложение","приложения","приложений")}`);
  if(sites)parts.push(`${sites} ${slovoPosleChisla(sites,"сайт","сайта","сайтов")}`);
  if(services)parts.push(`${services} ${slovoPosleChisla(services,"сервис","сервиса","сервисов")}`);
  return parts.length?`Напрямую настроено: ${perechislit(parts)}`:null;
}

/** «а, б и в»: последний соединяется союзом, а не запятой. */
function perechislit(chasti:string[]):string {
  if(chasti.length<2)return chasti[0]??"";
  return `${chasti.slice(0,-1).join(", ")} и ${chasti[chasti.length-1]}`;
}
