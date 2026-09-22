import { expect, it } from "vitest";
import { obyasnitMarshrut, opisatPraviloSoedineniya } from "./proba-marshruta";
import { normalizovatProbuDomena } from "./ohvat";
import type { PravilaTrafika } from "./trafik";

const trafik:PravilaTrafika={po_umolchaniyu:"direct",prilozheniya:[{put:"C:\\browser.exe",imya:"Browser",potomki:true,marshrut:"vpn"}],domeny:[{domen:"example.org",marshrut:"direct"},{domen:"api.example.org",marshrut:"vpn"}],servisy:[{id:"test",marshrut:"vpn"}]};
const base={trafik,put:"C:\\BROWSER.exe",domen:"example.org",bezRu:true,killSwitch:false,proksi:false,katalog:{versiya:"test",istochnik:"test",servisy:[{id:"test",imya:"Test",domeny:["example.org","service.org"],istochnik:"test"}]}};
it("правило приложения важнее сайта, но DNS сохраняет правило сайта",()=>{
  const p=obyasnitMarshrut(base);
  expect(p.dannye.marshrut).toBe("vpn");expect(p.dannye.prichina).toContain("Browser");
  expect(p.dns.marshrut).toBe("direct");expect(p.dns.prichina).toContain("example.org");
});
it("локальный прокси перекрывает direct-приложение, DNS для него не угадывается",()=>{
  const p=obyasnitMarshrut({...base,proksi:true,trafik:{...trafik,prilozheniya:[{...trafik.prilozheniya[0],marshrut:"direct"}]}});
  expect(p.dannye.marshrut).toBe("vpn");expect(p.dannye.prichina).toContain("локальный прокси");expect(p.dns.marshrut).toBeNull();
});

it("объясняет правило ядра без подмены неизвестных форматов",()=>{
  expect(opisatPraviloSoedineniya("process_path=C:\\app.exe => route(direct)")).toContain("отдельное правило приложения");
  expect(opisatPraviloSoedineniya("process_path_tree=C:\\steam.exe => route(vybor)")).toContain("запустила приложение");
  expect(opisatPraviloSoedineniya("domain_suffix=example.org => route(vybor)")).toContain("правило сайта или сервиса");
  expect(opisatPraviloSoedineniya("inbound=proksi-in => route(vybor)")).toContain("локальный прокси");
  expect(opisatPraviloSoedineniya("final")).toBe("Сработал общий маршрут");
  expect(opisatPraviloSoedineniya("new-unknown-format")).toContain("технических сведениях");
});
it("поддомен важнее основного домена, домен важнее сервиса",()=>{
  const p=obyasnitMarshrut({...base,put:"",domen:"x.api.example.org",trafik:{...trafik,prilozheniya:[]}});
  expect(p.dannye.marshrut).toBe("vpn");expect(p.dannye.prichina).toContain("api.example.org");
  expect(obyasnitMarshrut({...base,domen:"service.org"}).dns.prichina).toContain("Test");
});
it("не угадывает связанные запуски и отсутствующее приложение",()=>{
  expect(obyasnitMarshrut({...base,put:"C:\\unknown.exe"}).dannye.marshrut).toBeNull();
  expect(obyasnitMarshrut({...base,put:""}).dannye.marshrut).toBeNull();
});

it("недоступный каталог не выдаётся за отсутствие сервисного правила",()=>{
  const p=obyasnitMarshrut({...base,put:"",domen:"service.org",katalog:undefined,trafik:{...trafik,prilozheniya:[]}});
  expect(p.dannye.marshrut).toBeNull();expect(p.dns.marshrut).toBeNull();expect(p.dns.prichina).toContain("Каталог сервисов недоступен");
});
it("DNS по умолчанию для VPN-приложения, локальные имена и российский набор различаются",()=>{
  expect(obyasnitMarshrut({...base,domen:"unknown.org"}).dns.marshrut).toBe("vpn");
  expect(obyasnitMarshrut({...base,domen:"unknown.org",bezRu:false}).dns.marshrut).toBeNull();
  for(const domen of ["printer","router.local","router.lan","router.home.arpa"]){
    expect(obyasnitMarshrut({...base,domen,trafik:{...trafik,domeny:[{domen,marshrut:"vpn"}]}}).dns.marshrut).toBe("direct");
    expect(normalizovatProbuDomena(domen)).toBe(domen);
  }
  expect(normalizovatProbuDomena("127.0.0.1")).toBe("");
  expect(normalizovatProbuDomena("a".repeat(64)+".org")).toBe("");
});
it("не выдаёт конфликт блокировки за применимый маршрут",()=>{
  expect(obyasnitMarshrut({...base,killSwitch:true}).dannye.prichina).toContain("нельзя применить");
  const valid={...trafik,po_umolchaniyu:"vpn" as const,domeny:[]};
  expect(obyasnitMarshrut({...base,trafik:valid,killSwitch:true,bezRu:false}).dns.marshrut).toBe("vpn");
});
