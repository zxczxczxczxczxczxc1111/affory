import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { OhvatPravil } from "./OhvatPravil";
import { normalizovatProbuDomena, opisatDomen, razobratSnimok, type SnimokSoedineniy } from "../ohvat";
import type { PravilaTrafika } from "../trafik";

afterEach(cleanup);
const trafik:PravilaTrafika={po_umolchaniyu:"vpn",servisy:[],prilozheniya:[{put:"C:\\app.exe",imya:"App",potomki:true,marshrut:"direct"}],domeny:[{domen:"example.org",marshrut:"vpn"},{domen:"api.example.org",marshrut:"direct"}]};
const empty:SnimokSoedineniy={yadro:true,vremya:"2026-09-22T11:00:00Z",ogranichen:false,soedineniya:[]};
const props={vid:"sites" as const,trafik,chernovik:false,disabled:false,naSbros:vi.fn()};

it("обзор показывает более точные поддомены и сбрасывает только после подтверждения",()=>{
  const reset=vi.fn();
  render(<OhvatPravil {...props} naSbros={reset}/>);
  fireEvent.click(screen.getByRole("button",{name:"Показать охват правил"}));
  expect(screen.getByText(/Более точное правило: api.example.org/)).toHaveTextContent("напрямую");
  fireEvent.click(screen.getByRole("button",{name:"Сбросить правила сайтов"}));
  expect(reset).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button",{name:"Подтвердить сброс в черновике"}));
  expect(reset).toHaveBeenCalledTimes(1);
});

it("проверка передаёт нормализованный домен, пустота не выдаётся за поломку",async()=>{
  const inspect=vi.fn().mockResolvedValue(empty);
  render(<OhvatPravil {...props} proverit={inspect}/>);
  fireEvent.change(screen.getByLabelText("Домен для проверки"),{target:{value:"https://API.EXAMPLE.ORG/path"}});
  expect(screen.getByText(/Напрямую: правило api.example.org/)).toBeInTheDocument();
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Проверить соединения"})));
  expect(inspect).toHaveBeenCalledWith({domen:"api.example.org"});
  expect(screen.getByRole("status")).toHaveTextContent("Это не означает, что правило не работает");
});

it("ошибка, выключенное ядро и реальные записи различаются",async()=>{
  const inspect=vi.fn().mockRejectedValueOnce(new Error("ядро не ответило")).mockResolvedValueOnce({...empty,yadro:false}).mockResolvedValueOnce({...empty,ogranichen:true,soedineniya:[{Id:"1",Host:"api.example.org",Adres:"203.0.113.1",Port:443,Protsess:"C:\\app.exe",Vyhod:"direct",Pravilo:"domain_suffix=api.example.org",Nachalo:empty.vremya}]});
  render(<OhvatPravil {...props} proverit={inspect}/>);
  const refresh=()=>act(async()=>fireEvent.click(screen.getByRole("button",{name:"Проверить соединения"})));
  await refresh();expect(screen.getByRole("alert")).toHaveTextContent("ядро не ответило");
  expect(screen.queryByText(/Подходящих открытых/)).toBeNull();
  await refresh();expect(screen.getByRole("status")).toHaveTextContent("Подключение не запущено");
  await refresh();expect(screen.getByRole("status")).toHaveTextContent("Куда идёт соединение: напрямую");
  expect(screen.getByRole("status")).toHaveTextContent("Показаны первые 200");
});

it("поздний снимок не показывается для нового фильтра",async()=>{
  let finish:(value:SnimokSoedineniy)=>void=()=>{throw new Error("not started");};
  const inspect=vi.fn(()=>new Promise<SnimokSoedineniy>(resolve=>{finish=resolve;}));
  render(<OhvatPravil {...props} proverit={inspect}/>);
  fireEvent.click(screen.getByRole("button",{name:"Проверить соединения"}));
  expect(screen.getByRole("button",{name:"Проверить соединения"})).toBeDisabled();
  fireEvent.change(screen.getByLabelText("Домен для проверки"),{target:{value:"other.org"}});
  await act(async()=>finish(empty));
  expect(screen.queryByText(/Соединения на/)).toBeNull();
});

it("приложение проверяется по точному пути без обещания узнать родство по имени",async()=>{
  const inspect=vi.fn().mockResolvedValue(empty);
  render(<OhvatPravil {...props} vid="apps" chernovik proverit={inspect}/>);
  fireEvent.change(screen.getByLabelText("Путь приложения для проверки"),{target:{value:"C:\\app.exe"}});
  expect(screen.getByText(/Подсказка учитывает черновик/)).toBeInTheDocument();
  await act(async()=>fireEvent.click(screen.getByRole("button",{name:"Проверить соединения"})));
  expect(inspect).toHaveBeenCalledWith({put:"C:\\app.exe"});
  expect(screen.getByText(/По указанному пути видны соединения всех запусков этого файла/)).toBeInTheDocument();
});

it("суффиксы имеют границы, более точный домен побеждает, IDNA приводится",()=>{
  expect(opisatDomen("x.api.example.org",trafik)).toContain("Напрямую: правило api.example.org");
  expect(opisatDomen("notexample.org",trafik)).toContain("Явного доменного правила нет");
  expect(normalizovatProbuDomena("пример.рф")).toBe("xn--e1afmkfd.xn--p1ai");
  expect(normalizovatProbuDomena("https://user:password@example.org")).toBe("");
  expect(()=>razobratSnimok({})).toThrow();
  expect(()=>razobratSnimok({...empty,soedineniya:[{Host:"missing fields"}]})).toThrow();
});
