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
