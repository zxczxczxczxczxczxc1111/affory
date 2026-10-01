import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Nastroyki } from "./Nastroyki";

afterEach(cleanup);

it("журналы доступны в настройках и отправляют отдельные команды",()=>{
  const send=vi.fn();
  render(<Nastroyki status={{sostoyanie:"podnyat",diagnostika:true}} otlozheno={{}} naKomandu={send} naUdalenie={vi.fn()}/>);
  fireEvent.click(screen.getByTestId("razdel-diagnostika"));
  expect(screen.getByTestId("diagnostika")).toBeChecked();
  fireEvent.click(screen.getByTestId("diagnostika"));
  expect(send).toHaveBeenCalledWith("setDiagnostics",{vkl:false});
  fireEvent.click(screen.getByTestId("zhurnal"));
  expect(send).toHaveBeenCalledWith("setJournal",{vkl:true});
  fireEvent.click(screen.getByTestId("ochistit-zhurnal"));
  expect(send).toHaveBeenCalledWith("clearJournal",{});
});

// О6 аудита 1.6.1. Диагностику собирает служба, файл пишет оболочка по пути
// из диалога, экран узнаёт только итог. Папку журналов с 01.10.2026 читают
// все пользователи машины.
it("диагностика сохраняется файлом, итог виден",async()=>{
  const sohranit=vi.fn(async()=>"сохранено: affory-diagnostika-2026-09-28-134000.txt, 1,2 МБ");
  render(<Nastroyki status={{sostoyanie:"podnyat"}} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} naDiagnostiku={sohranit}/>);
  fireEvent.click(screen.getByTestId("razdel-diagnostika"));
  fireEvent.click(screen.getByTestId("sohranit-diagnostiku"));
  expect(await screen.findByTestId("itog-diagnostiki")).toHaveTextContent("affory-diagnostika-2026-09-28-134000.txt, 1,2 МБ");
  expect(sohranit).toHaveBeenCalledTimes(1);
});

it("передумал в диалоге: ни итога, ни отказа",async()=>{
  const sohranit=vi.fn(async()=>"");
  render(<Nastroyki status={{sostoyanie:"podnyat"}} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} naDiagnostiku={sohranit}/>);
  fireEvent.click(screen.getByTestId("razdel-diagnostika"));
  fireEvent.click(screen.getByTestId("sohranit-diagnostiku"));
  await waitFor(()=>expect(sohranit).toHaveBeenCalledTimes(1));
  await waitFor(()=>expect(screen.getByTestId("sohranit-diagnostiku")).toBeEnabled());
  expect(screen.queryByTestId("itog-diagnostiki")).toBeNull();
  expect(screen.queryByRole("alert")).toBeNull();
});

it("отказ выгрузки виден, повтор доступен",async()=>{
  const sohranit=vi.fn().mockRejectedValueOnce(new Error("выгрузка диагностики прервалась, сохрани её заново")).mockResolvedValue("сохранено: a.txt, 12 КБ");
  render(<Nastroyki status={{sostoyanie:"podnyat"}} otlozheno={{}} naKomandu={vi.fn()} naUdalenie={vi.fn()} naDiagnostiku={sohranit}/>);
  fireEvent.click(screen.getByTestId("razdel-diagnostika"));
  fireEvent.click(screen.getByTestId("sohranit-diagnostiku"));
  expect(await screen.findByRole("alert")).toHaveTextContent("прервалась");
  fireEvent.click(screen.getByTestId("sohranit-diagnostiku"));
  expect(await screen.findByTestId("itog-diagnostiki")).toHaveTextContent("a.txt");
  expect(screen.queryByRole("alert")).toBeNull();
});

it("без службы диагностику не собрать: кнопка неактивна",()=>{
  render(<Nastroyki status={{sostoyanie:"sluzhba-molchit"}} otlozheno={null} naKomandu={vi.fn()} naUdalenie={vi.fn()} naDiagnostiku={vi.fn()}/>);
  fireEvent.click(screen.getByTestId("razdel-diagnostika"));
  expect(screen.getByTestId("sohranit-diagnostiku")).toBeDisabled();
});

it("папка открывается даже без связи со службой, ошибка видна и повтор доступен",async()=>{
  const open=vi.fn().mockRejectedValueOnce(new Error("Папка недоступна")).mockResolvedValue(undefined);
  render(<Nastroyki status={{sostoyanie:"sluzhba-molchit"}} otlozheno={null} naKomandu={vi.fn()} naUdalenie={vi.fn()} naPapkuZhurnalov={open}/>);
  fireEvent.click(screen.getByTestId("razdel-diagnostika"));
  expect(screen.getByTestId("zhurnal")).toBeDisabled();
  expect(screen.getByTestId("diagnostika")).toBeDisabled();
  fireEvent.click(screen.getByRole("button",{name:"Открыть папку с журналами"}));
  expect(await screen.findByRole("alert")).toHaveTextContent("Папка недоступна");
  fireEvent.click(screen.getByRole("button",{name:"Открыть папку с журналами"}));
  await waitFor(()=>expect(screen.queryByText("Папка недоступна")).toBeNull());
  expect(open).toHaveBeenCalledTimes(2);
});
