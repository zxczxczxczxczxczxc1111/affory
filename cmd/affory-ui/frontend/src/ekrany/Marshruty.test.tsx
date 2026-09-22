import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { Pravila, type PravilaOtvet } from "./Pravila";
import { Glavnyy } from "./Glavnyy";
afterEach(cleanup);
async function primenit() {
  await act(async()=>{fireEvent.click(screen.getByRole("button",{name:"Применить изменения"}));});
}
const rules: PravilaOtvet = {
  protsessy: [],
  domeny: [],
  trafik: {
    po_umolchaniyu: "direct",
    prilozheniya: [],
    domeny: [{ domen: "work.example", marshrut: "direct" }],
    servisy: [],
  },
  katalog: {
    versiya: "test",
    istochnik: "https://example.org",
    servisy: [
      {
        id: "youtube",
        imya: "YouTube",
        domeny: ["youtube.com", "googlevideo.com"],
        istochnik: "https://example.org",
      },
    ],
  },
};

it("повтор домена с регистром и точкой заменяет маршрут одной записи", async () => {
  const send=vi.fn();
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={rules} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Домен сайта"),{target:{value:" WORK.EXAMPLE. "}});
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({trafik:expect.objectContaining({domeny:[{domen:"work.example",marshrut:"vpn"}]})}));
});

it("повтор приложения обновляет правило на прежнем месте",async()=>{
  const first={put:"C:\\First.exe",imya:"First.exe",potomki:true,marshrut:"direct" as const};
  const second={put:"C:\\Second.exe",imya:"Second.exe",potomki:false,marshrut:"vpn" as const};
  const send=vi.fn().mockResolvedValue(true);
  render(<Pravila status={{sostoyanie:"vyklyuchen"}} pravila={{...rules,trafik:{...rules.trafik!,prilozheniya:[first,second]}}} otlozheno={{}} naKomandu={send}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Приложения/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  fireEvent.change(screen.getByLabelText("Путь к приложению"),{target:{value:first.put}});
  fireEvent.click(screen.getByRole("button",{name:"Добавить в черновик"}));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules",expect.objectContaining({trafik:expect.objectContaining({prilozheniya:[{...first,marshrut:"vpn"},second]})}));
});

it("новая форма учитывает смену общего режима, открытая сохраняет выбранный маршрут", () => {
  const initial:PravilaOtvet={...rules,trafik:{...rules.trafik!,po_umolchaniyu:"vpn"}};
  const props={status:{sostoyanie:"vyklyuchen" as const},otlozheno:{},naKomandu:vi.fn()};
  const view=render(<Pravila {...props} pravila={initial}/>);
  view.rerender(<Pravila {...props} pravila={rules}/>);
  fireEvent.click(screen.getByRole("tab",{name:/Сайты/}));
  fireEvent.click(screen.getByRole("button",{name:"Добавить"}));
  expect(screen.getByRole("combobox",{name:"Маршрут нового правила"})).toHaveTextContent("Через VPN");
  fireEvent.click(screen.getByRole("combobox",{name:"Маршрут нового правила"}));
  fireEvent.click(screen.getByRole("option",{name:"Напрямую"}));
  view.rerender(<Pravila {...props} pravila={initial}/>);
  expect(screen.getByRole("combobox",{name:"Маршрут нового правила"})).toHaveTextContent("Напрямую");
});
it("service toggle sends a domain bundle route without inventing process exclusions", async () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "vyklyuchen" }}
      pravila={rules}
      otlozheno={{}}
      naKomandu={send}
    />,
  );
  fireEvent.click(screen.getByLabelText("YouTube через VPN"));
  // Число и слово стоят в разных строках макета, поэтому пробел между ними
  // рисует раскладка, а не текст.
  expect(screen.getByTestId("svodka-pravil").textContent).toMatch(/2\s*правила/);
  expect(screen.getByText("2 домена с поддоменами")).toBeInTheDocument();
  await primenit();
  expect(send).toHaveBeenCalledWith(
    "setRules",
    expect.objectContaining({
      trafik: {
        ...rules.trafik,
        servisy: [{ id: "youtube", marshrut: "vpn" }],
      },
    }),
  );
});
it("changing the default preserves explicit direct domain intent", async () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "vyklyuchen" }}
      pravila={rules}
      otlozheno={{}}
      naKomandu={send}
    />,
  );
  fireEvent.click(screen.getByRole("radio", { name: "Всё через VPN" }));
  await primenit();
  expect(send).toHaveBeenCalledWith(
    "setRules",
    expect.objectContaining({
      trafik: { ...rules.trafik, po_umolchaniyu: "vpn" },
    }),
  );
});
it("application picker persists the complete path and descendant scope", async () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "vyklyuchen" }}
      pravila={rules}
      otlozheno={{}}
      zapushchennye={[{ imya: "Steam", put: "C:\\Games\\Steam\\steam.exe" }]}
      naKomandu={send}
    />,
  );
  fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
  fireEvent.click(screen.getByRole("button", { name: /^Steam,/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить в черновик" }));
  await primenit();
  expect(send).toHaveBeenCalledWith(
    "setRules",
    expect.objectContaining({
      trafik: expect.objectContaining({
        prilozheniya: [
          {
            put: "C:\\Games\\Steam\\steam.exe",
            imya: "steam.exe",
            potomki: true,
            marshrut: "vpn",
          },
        ],
      }),
    }),
  );
});

// A file dialog supplies a path, not a surprise rule and certainly not a launch party.
function openAppForm(picker: () => Promise<string>) {
  const send = vi.fn();
  render(<Pravila status={{ sostoyanie: "vyklyuchen" }} pravila={rules} otlozheno={{}}
    naVyborPrilozheniya={picker} naKomandu={send} />);
  fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
  return send;
}

it("browsing the PC fills a path and waits for an explicit save", async () => {
  const path = "C:\\Мои приложения\\Steam\\steam.EXE";
  const picker = vi.fn(async () => path);
  const send = openAppForm(picker);
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  await waitFor(() => expect(screen.getByLabelText("Путь к приложению")).toHaveValue(path));
  expect(picker).toHaveBeenCalledOnce();
  expect(send).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Добавить в черновик" }));
  await primenit();
  expect(send).toHaveBeenCalledWith("setRules", expect.objectContaining({
    trafik: expect.objectContaining({ prilozheniya: [
      { put: path, imya: "steam.EXE", potomki: true, marshrut: "vpn" },
    ] }),
  }));
});

it("cancelling file selection preserves a manually entered path", async () => {
  const send = openAppForm(async () => "");
  fireEvent.change(screen.getByLabelText("Путь к приложению"), { target: { value: "C:\\Apps\\old.exe" } });
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "Выбрать на ПК…" })).toBeEnabled());
  expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\old.exe");
  expect(screen.queryByRole("alert")).toBeNull();
  expect(send).not.toHaveBeenCalled();
});

it("reports a dialog error by the picker and allows another attempt", async () => {
  const picker = vi.fn().mockRejectedValueOnce(new Error("нет доступа")).mockResolvedValueOnce("C:\\Apps\\new.exe");
  const send = openAppForm(picker);
  fireEvent.change(screen.getByLabelText("Путь к приложению"), { target: { value: "C:\\Apps\\old.exe" } });
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось выбрать приложение: нет доступа");
  expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\old.exe");
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  await waitFor(() => expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\new.exe"));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(send).not.toHaveBeenCalled();
});

it("blocks duplicate dialogs and discards a selection after leaving the form", async () => {
  let choose!: (path: string) => void;
  const picker = vi.fn(() => new Promise<string>(resolve => { choose = resolve; }));
  const send = openAppForm(picker);
  fireEvent.change(screen.getByLabelText("Путь к приложению"), { target: { value: "C:\\Apps\\old.exe" } });
  fireEvent.click(screen.getByRole("button", { name: "Выбрать на ПК…" }));
  const pending = screen.getByRole("button", { name: "Выбор…" });
  expect(pending).toBeDisabled();
  expect(screen.getByRole("button", { name: "Добавить в черновик" })).toBeDisabled();
  fireEvent.click(pending);
  expect(picker).toHaveBeenCalledOnce();
  fireEvent.click(screen.getByRole("tab", { name: /Сайты/ }));
  await act(async () => choose("C:\\Apps\\new.exe"));
  fireEvent.click(screen.getByRole("tab", { name: /Приложения/ }));
  fireEvent.click(screen.getByRole("button", { name: "Добавить" }));
  expect(screen.getByLabelText("Путь к приложению")).toHaveValue("C:\\Apps\\old.exe");
  expect(send).not.toHaveBeenCalled();
});
it("sphere can cancel an in-progress connection and has no duplicate heading", () => {
  const act = vi.fn();
  render(<Glavnyy status={{ sostoyanie: "podnimaetsya" }} naDeystvie={act} />);
  fireEvent.click(screen.getByRole("button", { name: "Отменить подключение" }));
  expect(act).toHaveBeenCalledOnce();
  expect(screen.queryByText(/VPN включён|VPN выключен/)).toBeNull();
});
it("server row connects directly from the home screen", () => {
  const choose = vi.fn();
  render(
    <Glavnyy
      status={{ sostoyanie: "vyklyuchen" }}
      servery={[
        {
          id: "ams",
          imya: "Amsterdam",
          host: "vpn.example.org",
          port: 443,
          transport: "hy2",
          iz_podpiski: true,
        },
      ]}
      naVyborServera={choose}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "Подключиться к Amsterdam" }),
  );
  expect(choose).toHaveBeenCalledWith("ams");
});
it("pending rule application disables controls instead of losing a second edit", () => {
  const send = vi.fn();
  render(
    <Pravila
      status={{ sostoyanie: "podnyat" }}
      pravila={rules}
      otlozheno={{}}
      naKomandu={send}
      zanyato
    />,
  );
  expect(screen.getByLabelText("YouTube через VPN")).toBeDisabled();
  fireEvent.click(screen.getByLabelText("YouTube через VPN"));
  expect(send).not.toHaveBeenCalled();
});
it("журнал перенесён из правил в настройки", () => {
  render(<Pravila status={{sostoyanie:"podnyat",diagnostika:true}} pravila={rules} otlozheno={{}} naKomandu={vi.fn()}/>);
  expect(screen.queryByTestId("diagnostika")).toBeNull();
  expect(screen.queryByTestId("razdel-zhurnal")).toBeNull();
});
