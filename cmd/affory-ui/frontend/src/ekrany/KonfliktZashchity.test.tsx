import {act,cleanup,fireEvent,render,screen} from "@testing-library/react";
import {afterEach,expect,it,vi} from "vitest";
import {Pravila,type PravilaOtvet} from "./Pravila";
import {Nastroyki} from "./Nastroyki";
import {Glavnyy} from "./Glavnyy";
import {Vybor} from "./Vybor";
import type {PravilaTrafika} from "../trafik";

// Защита сети и прямые маршруты, редакция 23.09.2026.
//
// Раньше это был КОНФЛИКТ: прямое правило роняло сборку конфига, поэтому окно
// запрещало и включать защиту, и выбирать «Напрямую», пока человек своими
// руками не перепишет каждое правило. Теперь прямые правила просто СПЯТ, пока
// защита включена: служба их не применяет, набор на диске не меняется, и всё
// возвращается в дело само, когда защиту выключают. Так же поступает ProtonVPN
// с раздельным туннелированием; переписывать чужие настройки на VPN было бы
// необратимо.
//
// Файл проверяет обе стороны: запретов больше нет, и человек при этом ЗНАЕТ,
// что его правила сейчас не работают.

afterEach(cleanup);
const trafik:PravilaTrafika={po_umolchaniyu:"vpn",prilozheniya:[],domeny:[],servisy:[]};
const rules:PravilaOtvet={protsessy:[],domeny:[],trafik,katalog:{versiya:"test",istochnik:"test",servisy:[{id:"youtube",imya:"YouTube",domeny:["youtube.com"],istochnik:"test"}]}};

it("при включённой защите прямой режим и вариант сервиса доступны",()=>{
  const send=vi.fn();
  render(<Pravila status={{sostoyanie:"vyklyuchen",kill_switch:true}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  expect(screen.getByRole("radio",{name:"Только выбранное"})).toBeEnabled();
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"}));
  const direct=screen.getByRole("option",{name:"Напрямую"});
  expect(direct).not.toHaveAttribute("aria-disabled","true");
  fireEvent.click(direct);
  expect(screen.getByRole("combobox",{name:"Маршрут сервиса YouTube"})).toHaveTextContent("Напрямую");
});

it("под защитой окно говорит, что прямые маршруты сейчас не действуют",()=>{
  render(<Pravila status={{sostoyanie:"podnyat",kill_switch:true}} otlozheno={{}} naKomandu={vi.fn()}
    pravila={{...rules,trafik:{...trafik,domeny:[{domen:"example.org",marshrut:"direct"}]}}}/>);
  expect(screen.getByText(/прямые маршруты сейчас не действуют/i)).toBeInTheDocument();
});

it("новая форма при защите предлагает исключение и добавляет его",()=>{
  const send=vi.fn();
  const props={pravila:rules,otlozheno:{},naKomandu:send};
  render(<Pravila {...props} status={{sostoyanie:"vyklyuchen",kill_switch:true}}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:"example.org"}});
  expect(screen.getByRole("combobox",{name:"Маршрут нового правила"})).toHaveTextContent("Напрямую");
  // Правило заводится, но человек предупреждён, что оно пока спит.
  expect(screen.getByText(/заработает после её выключения/i)).toBeInTheDocument();
  const knopka=screen.getByRole("button",{name:"Добавить в черновик"});
  expect(knopka).toBeEnabled();
  fireEvent.click(knopka);
  expect(screen.getByRole("button",{name:"Применить изменения"})).toBeEnabled();
});

it("черновик с прямым правилом применяется и при включённой защите",async()=>{
  const send=vi.fn().mockResolvedValue(true);
  const props={pravila:rules,otlozheno:{},naKomandu:send};
  const view=render(<Pravila {...props} status={{sostoyanie:"vyklyuchen",kill_switch:false}}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:"example.org"}});fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  view.rerender(<Pravila {...props} status={{sostoyanie:"vyklyuchen",kill_switch:true}}/>);
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Применить изменения"})));
  expect(send).toHaveBeenCalledExactlyOnceWith("setRules",expect.objectContaining({trafik:{...trafik,domeny:[{domen:"example.org",marshrut:"direct"}]}}));
});

it.each([
  {...trafik,po_umolchaniyu:"direct" as const},
  {...trafik,prilozheniya:[{put:"C:\\App.exe",imya:"App",potomki:true,marshrut:"direct" as const}]},
  {...trafik,domeny:[{domen:"example.org",marshrut:"direct" as const}]},
  {...trafik,servisy:[{id:"youtube",marshrut:"direct" as const}]},
])("настройки включают защиту поверх прямых маршрутов: %j",(current)=>{
  const send=vi.fn();
  render(<Nastroyki status={{sostoyanie:"vyklyuchen",kill_switch:false}} trafik={current} otlozheno={{}} naKomandu={send} naUdalenie={vi.fn()} naPravila={vi.fn()}/>);
  const tumbler=screen.getByTestId("ves-trafik");
  expect(tumbler).toBeEnabled();
  fireEvent.click(tumbler);
  expect(send).toHaveBeenCalledExactlyOnceWith("setKillSwitch",{vkl:true});
});

it("перед включением защиты сказано, что именно перестанет действовать",()=>{
  const send=vi.fn();
  render(<Nastroyki status={{sostoyanie:"podnyat",kill_switch:false}} otlozheno={{}} naKomandu={send} naUdalenie={vi.fn()}
    trafik={{...trafik,domeny:[{domen:"example.org",marshrut:"direct"}]}}/>);
  // На поднятом VPN смена режима спрашивает подтверждение: там и место
  // предупреждению, пока человек ещё не нажал.
  fireEvent.click(screen.getByTestId("ves-trafik"));
  expect(screen.getByTestId("usnut-pravila")).toHaveTextContent(/1 сайт/);
  expect(screen.getByTestId("usnut-pravila")).toHaveTextContent(/не действуют/);
  expect(screen.getByTestId("podtverdit-rezhim")).toBeEnabled();
});

it("черновик правил по-прежнему мешает включению; выключение остаётся доступным",()=>{
  const send=vi.fn(),refresh=vi.fn();
  const props={otlozheno:{},naKomandu:send,naUdalenie:vi.fn(),obnovitPravila:refresh};
  const view=render(<Nastroyki {...props} status={{sostoyanie:"vyklyuchen"}}/>);
  fireEvent.click(screen.getByRole("button",{name:"Обновить правила"}));expect(refresh).toHaveBeenCalledOnce();
  view.rerender(<Nastroyki {...props} status={{sostoyanie:"vyklyuchen"}} trafik={trafik} estChernovikPravil/>);
  expect(screen.getByTestId("ves-trafik")).toBeDisabled();expect(screen.getByText(/Сначала примени или отмени черновик/)).toBeInTheDocument();
  view.rerender(<Nastroyki {...props} status={{sostoyanie:"vyklyuchen",kill_switch:true}}/>);
  expect(screen.getByTestId("ves-trafik")).toBeEnabled();fireEvent.click(screen.getByTestId("ves-trafik"));expect(send).toHaveBeenCalledExactlyOnceWith("setKillSwitch",{vkl:false});
});

it("главный экран отправляет выборочный режим и при включённой защите",()=>{
  const change=vi.fn();render(<Glavnyy status={{sostoyanie:"podnyat",kill_switch:true}} pravila={rules} naTrafik={change}/>);
  const radio=screen.getByRole("radio",{name:"Только выбранное"});
  expect(radio).toBeEnabled();fireEvent.click(radio);expect(change).toHaveBeenCalledExactlyOnceWith("direct");
});

it("список пропускает недоступные варианты при вводе и выборе клавиатурой",()=>{
  const change=vi.fn();render(<Vybor label="Маршрут" value="vpn" options={[{value:"vpn",label:"Через VPN"},{value:"direct",label:"Напрямую",disabled:true},{value:"inherit",label:"По общему режиму"}]} onChange={change}/>);
  const control=screen.getByRole("combobox");fireEvent.keyDown(control,{key:"Enter"});fireEvent.keyDown(control,{key:"ArrowDown"});
  expect(control).toHaveAttribute("aria-activedescendant",screen.getByRole("option",{name:"По общему режиму"}).id);
  fireEvent.keyDown(control,{key:"н"});expect(control).not.toHaveAttribute("aria-activedescendant",screen.getByRole("option",{name:"Напрямую"}).id);
  fireEvent.keyDown(control,{key:"Enter"});expect(change).toHaveBeenCalledExactlyOnceWith("inherit");
});
