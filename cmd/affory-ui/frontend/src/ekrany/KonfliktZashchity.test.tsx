import {act,cleanup,fireEvent,render,screen} from "@testing-library/react";
import {afterEach,expect,it,vi} from "vitest";
import {Pravila,type PravilaOtvet} from "./Pravila";
import {Nastroyki} from "./Nastroyki";
import {Glavnyy} from "./Glavnyy";
import {Vybor} from "./Vybor";
import type {PravilaTrafika} from "../trafik";

afterEach(cleanup);
const trafik:PravilaTrafika={po_umolchaniyu:"vpn",prilozheniya:[],domeny:[],servisy:[]};
const rules:PravilaOtvet={protsessy:[],domeny:[],trafik,katalog:{versiya:"test",istochnik:"test",servisy:[{id:"youtube",imya:"YouTube",domeny:["youtube.com"],istochnik:"test"}]}};

it("при включённой блокировке прямой режим и вариант сервиса недоступны",()=>{
  const send=vi.fn();
  render(<Pravila status={{sostoyanie:"vyklyuchen",kill_switch:true}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  expect(screen.getByRole("radio",{name:"Только выбранное"})).toBeDisabled();
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"}));
  const direct=screen.getByRole("option",{name:"Напрямую"});
  expect(direct).toHaveAttribute("aria-disabled","true");
  fireEvent.click(direct);
  expect(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"})).toHaveTextContent("По общему режиму");
  expect(screen.queryByRole("button",{name:"Применить изменения"})).toBeNull();
  expect(send).not.toHaveBeenCalled();
});

it("новая форма выбирает VPN, а включение блокировки во время ввода не отправляет direct",()=>{
  const send=vi.fn();
  const props={pravila:rules,otlozheno:{},naKomandu:send};
  const view=render(<Pravila {...props} status={{sostoyanie:"vyklyuchen",kill_switch:false}}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:"example.org"}});
  expect(screen.getByRole("combobox",{name:"Маршрут нового правила"})).toHaveTextContent("Напрямую");
  view.rerender(<Pravila {...props} status={{sostoyanie:"vyklyuchen",kill_switch:true}}/>);
  expect(screen.getByRole("button",{name:"Добавить в черновик"})).toBeDisabled();
  expect(screen.getByText(/Выбери «Через VPN», чтобы добавить правило/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));expect(send).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button",{name:"Закрыть"}));fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  expect(screen.getByRole("combobox",{name:"Маршрут нового правила"})).toHaveTextContent("Через VPN");
});

it("конфликтующий черновик сохраняется, исправление позволяет применить без изменения защиты",async()=>{
  const send=vi.fn().mockResolvedValue(true);
  const props={pravila:rules,otlozheno:{},naKomandu:send};
  const view=render(<Pravila {...props} status={{sostoyanie:"vyklyuchen",kill_switch:false}}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:"example.org"}});fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  view.rerender(<Pravila {...props} status={{sostoyanie:"vyklyuchen",kill_switch:true}}/>);
  expect(screen.getByRole("alert")).toHaveTextContent("1 сайт");
  expect(screen.getByRole("button",{name:"Применить изменения"})).toBeDisabled();
  fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));expect(send).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут example.org"}));fireEvent.click(screen.getByRole("option",{name:"Через VPN"}));
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Применить изменения"})));
  expect(send).toHaveBeenCalledExactlyOnceWith("setRules",expect.objectContaining({trafik:{...trafik,domeny:[{domen:"example.org",marshrut:"vpn"}]}}));
});

it.each([
  {...trafik,po_umolchaniyu:"direct" as const},
  {...trafik,prilozheniya:[{put:"C:\\App.exe",imya:"App",potomki:true,marshrut:"direct" as const}]},
  {...trafik,domeny:[{domen:"example.org",marshrut:"direct" as const}]},
  {...trafik,servisy:[{id:"youtube",marshrut:"direct" as const}]},
])("настройки не включают блокировку поверх прямых маршрутов: %j",(current)=>{
  const send=vi.fn(),open=vi.fn();
  render(<Nastroyki status={{sostoyanie:"vyklyuchen",kill_switch:false}} trafik={current} otlozheno={{}} naKomandu={send} naUdalenie={vi.fn()} naPravila={open}/>);
  expect(screen.getByTestId("ves-trafik")).toBeDisabled();fireEvent.click(screen.getByTestId("ves-trafik"));expect(send).not.toHaveBeenCalled();
  expect(screen.getByText(/Блокировка несовместима с прямыми маршрутами/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button",{name:"Открыть правила"}));expect(open).toHaveBeenCalledOnce();
});

it("пока правила не прочитаны или есть черновик, включение недоступно; выключение остаётся доступным",()=>{
  const send=vi.fn(),refresh=vi.fn();
  const props={otlozheno:{},naKomandu:send,naUdalenie:vi.fn(),obnovitPravila:refresh};
  const view=render(<Nastroyki {...props} status={{sostoyanie:"vyklyuchen"}}/>);
  expect(screen.getByTestId("ves-trafik")).toBeDisabled();fireEvent.click(screen.getByRole("button",{name:"Обновить правила"}));expect(refresh).toHaveBeenCalledOnce();
  view.rerender(<Nastroyki {...props} status={{sostoyanie:"vyklyuchen"}} trafik={trafik} estChernovikPravil/>);
  expect(screen.getByTestId("ves-trafik")).toBeDisabled();expect(screen.getByText(/Сначала примени или отмени черновик/)).toBeInTheDocument();
  view.rerender(<Nastroyki {...props} status={{sostoyanie:"vyklyuchen",kill_switch:true}}/>);
  expect(screen.getByTestId("ves-trafik")).toBeEnabled();fireEvent.click(screen.getByTestId("ves-trafik"));expect(send).toHaveBeenCalledExactlyOnceWith("setKillSwitch",{vkl:false});
});

it("прямые правила, появившиеся при открытом подтверждении, блокируют отправку",()=>{
  const send=vi.fn(),props={status:{sostoyanie:"podnyat" as const},otlozheno:{},naKomandu:send,naUdalenie:vi.fn()};
  const view=render(<Nastroyki {...props} trafik={trafik}/>);
  fireEvent.click(screen.getByTestId("ves-trafik"));expect(screen.getByTestId("podtverdit-rezhim")).toBeEnabled();
  view.rerender(<Nastroyki {...props} trafik={{...trafik,po_umolchaniyu:"direct"}}/>);
  expect(screen.getByTestId("podtverdit-rezhim")).toBeDisabled();fireEvent.click(screen.getByTestId("podtverdit-rezhim"));expect(send).not.toHaveBeenCalled();
  expect(screen.getByRole("alert")).toHaveTextContent("Включение блокировки недоступно");
});

it("главный экран не отправляет выборочный режим при включённой блокировке",()=>{
  const change=vi.fn();render(<Glavnyy status={{sostoyanie:"podnyat",kill_switch:true}} pravila={rules} naTrafik={change}/>);
  expect(screen.getByRole("radio",{name:"Только выбранное"})).toBeDisabled();fireEvent.click(screen.getByRole("radio",{name:"Только выбранное"}));expect(change).not.toHaveBeenCalled();
});

it("список пропускает недоступные варианты при вводе и выборе клавиатурой",()=>{
  const change=vi.fn();render(<Vybor label="Маршрут" value="vpn" options={[{value:"vpn",label:"Через VPN"},{value:"direct",label:"Напрямую",disabled:true},{value:"inherit",label:"По общему режиму"}]} onChange={change}/>);
  const control=screen.getByRole("combobox");fireEvent.keyDown(control,{key:"Enter"});fireEvent.keyDown(control,{key:"ArrowDown"});
  expect(control).toHaveAttribute("aria-activedescendant",screen.getByRole("option",{name:"По общему режиму"}).id);
  fireEvent.keyDown(control,{key:"н"});expect(control).not.toHaveAttribute("aria-activedescendant",screen.getByRole("option",{name:"Напрямую"}).id);
  fireEvent.keyDown(control,{key:"Enter"});expect(change).toHaveBeenCalledExactlyOnceWith("inherit");
});
