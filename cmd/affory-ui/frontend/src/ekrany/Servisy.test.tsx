import {act,cleanup,fireEvent,render,screen,within} from "@testing-library/react";
import {afterEach,expect,it,vi} from "vitest";
import {Pravila,type PravilaOtvet} from "./Pravila";
import {perekrytiyaServisa} from "../ohvat";
import {zadatMarshrutServisa,type VyborServisa} from "../trafik";

afterEach(cleanup);
const rules:PravilaOtvet={protsessy:[],domeny:[],trafik:{po_umolchaniyu:"vpn",prilozheniya:[],domeny:[],servisy:[]},katalog:{versiya:"test",istochnik:"test",servisy:[{id:"youtube",imya:"YouTube",domeny:["youtube.com"],istochnik:"test"}]}};
function choose(value:string){fireEvent.click(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"}));fireEvent.click(screen.getByRole("option",{name:value}));}

it("явное VPN сохраняется при смене общего режима, возврат к общему удаляет правило",async()=>{
  const send=vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  const control=screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"});
  expect(control).toHaveTextContent("По общему режиму");
  choose("Через VPN");
  expect(screen.getByTestId("svodka-pravil")).toHaveTextContent("1отдельное правило");
  fireEvent.click(screen.getByRole("radio",{name:"Только выбранное"}));
  expect(control).toHaveTextContent("Через VPN");
  expect(screen.getByLabelText("Маршруты сервисов")).toHaveTextContent("через VPN 1, напрямую 0");
  choose("По общему режиму");
  expect(screen.getByLabelText("Маршруты сервисов")).toHaveTextContent("через VPN 0, напрямую 1");
  expect(screen.getByTestId("svodka-pravil")).toHaveTextContent("0отдельных правил");
  expect(send).not.toHaveBeenCalled();
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Применить изменения"})));
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({trafik:{...rules.trafik,po_umolchaniyu:"direct",servisy:[]}}));
});

it("явный direct сохраняется после смены режима и счёт не растёт от повторного выбора",()=>{
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={vi.fn()}/>);
  choose("Напрямую");choose("Напрямую");
  fireEvent.click(screen.getByRole("radio",{name:"Только выбранное"}));
  fireEvent.click(screen.getByRole("radio",{name:"Всё через VPN"}));
  expect(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"})).toHaveTextContent("Напрямую");
  expect(screen.getByLabelText("Маршруты сервисов")).toHaveTextContent("Заданы отдельно: 1");
});

it("сброс сервисов виден без раскрытия доменов и сохраняет приложения и сайты",async()=>{
  const send=vi.fn().mockResolvedValue(true);
  const trafik={...rules.trafik!,servisy:[{id:"youtube",marshrut:"direct" as const}],domeny:[{domen:"example.org",marshrut:"vpn" as const}],prilozheniya:[{put:"C:\\App.exe",imya:"App",potomki:false,marshrut:"vpn" as const}]};
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,trafik}} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("button",{name:"Сбросить маршруты сервисов"}));
  expect(send).not.toHaveBeenCalled();
  expect(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"})).toHaveTextContent("По общему режиму");
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Применить изменения"})));
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({trafik:{...trafik,servisy:[]}}));
});

it("показывает другое правило сайта отдельно от счётчика сервисов",()=>{
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,trafik:{...rules.trafik!,domeny:[{domen:"api.youtube.com",marshrut:"direct"}]}}} otlozheno={{}} naKomandu={vi.fn()}/>);
  expect(screen.getByRole("tab",{name:/Сервисы/})).toHaveTextContent("1");
  expect(screen.getByLabelText("Маршруты сервисов")).toHaveTextContent("Другой маршрут задан в правилах сайтов: 1");
  const card=screen.getByRole("article");
  expect(within(card).getByText("Есть другой маршрут в правилах сайтов")).toBeInTheDocument();
  fireEvent.click(within(card).getByRole("button",{name:"1 домен с поддоменами"}));
  expect(within(card).getByText("Правило api.youtube.com: напрямую")).toBeInTheDocument();
});

it("более точное правило не даёт ложного предупреждения о перекрытом общем домене",()=>{
  expect(perekrytiyaServisa(["youtube.com"],[{domen:"com",marshrut:"direct"},{domen:"youtube.com",marshrut:"vpn"}],"vpn")).toEqual([]);
  expect(perekrytiyaServisa(["youtube.com"],[{domen:"notyoutube.com",marshrut:"direct"}],"vpn")).toEqual([]);
  expect(perekrytiyaServisa(["youtube.com"],[{domen:"com",marshrut:"direct"}],"vpn")).toEqual([{domen:"com",marshrut:"direct"}]);
});

it("смена сервиса сохраняет порядок правил, общий режим не создаёт явную запись",()=>{
  const trafik={...rules.trafik!,servisy:[{id:"youtube",marshrut:"vpn" as const},{id:"other",marshrut:"direct" as const}]};
  for(const choice of ["vpn","direct"] as VyborServisa[]) expect(zadatMarshrutServisa(trafik,"youtube",choice).servisy.map(s=>s.id)).toEqual(["youtube","other"]);
  expect(zadatMarshrutServisa(rules.trafik!,"youtube","inherit").servisy).toEqual([]);
});

it("недоступный каталог не мешает сбросить сохранённые маршруты сервисов",()=>{
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,katalog:undefined,trafik:{...rules.trafik!,servisy:[{id:"youtube",marshrut:"direct"}]}}} otlozheno={{}} naKomandu={vi.fn()}/>);
  expect(screen.queryByLabelText("Маршруты сервисов")).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Сбросить маршруты сервисов"}));
  expect(screen.getByTestId("svodka-pravil")).toHaveTextContent("0отдельных правил");
  expect(screen.getByRole("button",{name:"Применить изменения"})).toBeEnabled();
});
