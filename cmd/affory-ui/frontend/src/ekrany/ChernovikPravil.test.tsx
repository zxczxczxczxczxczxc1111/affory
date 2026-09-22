import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Pravila, type PravilaOtvet } from "./Pravila";

afterEach(cleanup);
const rules:PravilaOtvet={reviziya_pravil:"v1",protsessy:[],domeny:[],trafik:{po_umolchaniyu:"vpn",prilozheniya:[],domeny:[],servisy:[]},katalog:{versiya:"test",istochnik:"test",servisy:[{id:"youtube",imya:"YouTube",domeny:["youtube.com"],istochnik:"test"}]}};

it("повторное нажатие во время применения не отправляет второй запрос",async()=>{
  let complete: (value:boolean)=>void = ()=>{throw new Error("request not started");};
  const send=vi.fn(()=>new Promise<boolean>(resolve=>{complete=resolve;}));
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("radio",{name:"Только выбранное"}));
  const button=screen.getByRole("button",{name:"Применить изменения"});
  fireEvent.click(button);
  expect(button).toBeDisabled();
  fireEvent.click(button);
  expect(send).toHaveBeenCalledTimes(1);
  await act(async()=>complete(true));
  expect(screen.queryByRole("button",{name:"Применить изменения"})).toBeNull();
});

it("изменения разных вкладок применяются одной командой с исходной ревизией",async()=>{
  const send=vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"podnyat"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByLabelText("YouTube через VPN"));
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:"example.org"}});
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  expect(screen.getByRole("status")).toHaveTextContent("добавлено в черновик");
  expect(send).not.toHaveBeenCalled();
  await act(async()=>{fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));});
  expect(send).toHaveBeenCalledTimes(1);
  expect(send).toHaveBeenCalledWith("setRules",{reviziya_pravil:"v1",bez_ru_spiska:false,trafik:{...rules.trafik,servisy:[{id:"youtube",marshrut:"direct"}],domeny:[{domen:"example.org",marshrut:"direct"}]}});
  expect(screen.getByRole("status")).toHaveTextContent("Правила применены");
});

it("ошибка сохраняет черновик и позволяет повторить применение",async()=>{
  const send=vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("radio",{name:"Только выбранное"}));
  await act(async()=>{fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));});
  expect(screen.getByRole("alert")).toHaveTextContent("Черновик сохранён");
  expect(screen.getByRole("radio",{name:"Только выбранное"})).toBeChecked();
  await act(async()=>{fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));});
  expect(send).toHaveBeenCalledTimes(2);
  expect(send.mock.calls[0]).toEqual(send.mock.calls[1]);
  expect(screen.getByRole("status")).toHaveTextContent("Правила сохранены");
});

it("конфликт со свежими правилами блокирует перезапись, отмена не посылает команды",()=>{
  const send=vi.fn();
  const props={status:{sostoyanie:"vyklyuchen" as const},otlozheno:{},naKomandu:send};
  const view=render(<Pravila {...props} pravila={rules}/>);
  fireEvent.click(screen.getByRole("radio",{name:"Только выбранное"}));
  view.rerender(<Pravila {...props} pravila={{...rules,reviziya_pravil:"v2",bez_ru_spiska:true}}/>);
  expect(screen.getByRole("button",{name:"Применить изменения"})).toBeDisabled();
  expect(screen.getByRole("alert")).toHaveTextContent("Сохранённые правила изменились");
  fireEvent.click(screen.getByRole("button",{name:"Отменить изменения"}));
  expect(send).not.toHaveBeenCalled();
  expect(screen.getByRole("radio",{name:"Всё через VPN"})).toBeChecked();
  expect(screen.queryByRole("button",{name:"Применить изменения"})).toBeNull();
});

it("неподтверждённый ответ не стирает введённый путь",async()=>{
  const send=vi.fn().mockResolvedValue(undefined);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Путь к приложению"),{target:{value:"C:\\Missing\\app.exe"}});
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  await act(async()=>{fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));});
  expect(screen.queryByText("Правила сохранены")).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Missing\\app.exe");
});
