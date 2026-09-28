import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Pravila, type PravilaOtvet } from "./Pravila";
import type { ReklamaSostoyanie, StatusOtvet } from "../protokol";
import type { ReklamaPravila } from "../reklama";
import type { PravilaTrafika } from "../trafik";

afterEach(cleanup);

// Вкладка «Реклама»: настройка едет общим черновиком правил и уходит в
// setRules одним телом с остальным.

const TRAFIK: PravilaTrafika = { po_umolchaniyu: "vpn", prilozheniya: [], domeny: [], servisy: [] };
// Ключи в порядке службы: она сортирует ключи map (teloReklamy).
const VYKL_SLUZHBY: ReklamaPravila = { razresheno: [], uroven: "light", vkl: false };
const VYKL: StatusOtvet = { sostoyanie: "vyklyuchen" };

function pravila(reklama?: ReklamaPravila, reviziya = "r1"): PravilaOtvet {
  return { protsessy: [], domeny: [], reviziya_pravil: reviziya, trafik: TRAFIK, ...(reklama ? { reklama } : {}) };
}

function risovat(p: PravilaOtvet = pravila(VYKL_SLUZHBY), status: StatusOtvet = VYKL) {
  const send = vi.fn<(komanda: string, telo: unknown) => Promise<boolean>>().mockResolvedValue(true);
  const vid = render(<Pravila status={status} pravila={p} otlozheno={{}} naKomandu={send} />);
  fireEvent.click(screen.getByRole("tab", { name: /^Реклама/ }));
  const pererisovat = (p2: PravilaOtvet) => vid.rerender(<Pravila status={status} pravila={p2} otlozheno={{}} naKomandu={send} />);
  return { send, pererisovat };
}

async function primenit() {
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Применить изменения" })); });
}

const vvesti = (tekst: string) => fireEvent.change(screen.getByLabelText("Сайт-исключение"), { target: { value: tekst } });

describe("вкладка «Реклама»", () => {
  it("переключатель пишет черновик и не шлёт ничего до «Применить»", async () => {
    const { send } = risovat();
    fireEvent.click(screen.getByTestId("reklama-vkl"));
    expect(screen.getByText("Есть неприменённые изменения")).toBeInTheDocument();
    expect(send).not.toHaveBeenCalled();
    await primenit();
    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith("setRules", {
      trafik: TRAFIK,
      bez_ru_spiska: false,
      reklama: { vkl: true, uroven: "light", razresheno: [] },
      reviziya_pravil: "r1",
    });
  });

  it("уровень выбирается только у включённой и уходит в тело", async () => {
    const { send } = risovat();
    expect(screen.getByRole("radio", { name: "Расширенный" })).toBeDisabled();
    fireEvent.click(screen.getByTestId("reklama-vkl"));
    fireEvent.click(screen.getByRole("radio", { name: "Расширенный" }));
    expect(screen.getByText(/Изредка ломает вход или оплату/)).toBeInTheDocument();
    await primenit();
    expect(send).toHaveBeenCalledWith("setRules", expect.objectContaining({ reklama: { vkl: true, uroven: "multi", razresheno: [] } }));
  });

  it("ввод исключений разбирается как у сайтов, адрес отвергается", async () => {
    const { send } = risovat();
    vvesti("Пример.РФ, https://Mc.Yandex.ru/x");
    const razbor = screen.getByTestId("razbor-isklyucheniy");
    expect(razbor).toHaveTextContent("Добавится 2 исключения");
    expect(razbor).toHaveTextContent("xn--e1afmkfd.xn--p1ai");
    expect(razbor).toHaveTextContent("mc.yandex.ru");
    fireEvent.click(screen.getByRole("button", { name: "Добавить в черновик" }));
    expect(screen.getByRole("button", { name: "Удалить исключение mc.yandex.ru" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: /^Реклама/ })).toHaveTextContent("2");

    vvesti("1.2.3.4");
    expect(screen.getByRole("alert")).toHaveTextContent("1.2.3.4: это адрес");
    expect(screen.getByRole("button", { name: "Добавить в черновик" })).toBeDisabled();

    await primenit();
    expect(send).toHaveBeenCalledWith("setRules", expect.objectContaining({
      reklama: { vkl: false, uroven: "light", razresheno: ["xn--e1afmkfd.xn--p1ai", "mc.yandex.ru"] },
    }));
  });

  it("сверх 256 исключений черновик не растёт, а окно говорит почему", () => {
    const polno = Array.from({ length: 256 }, (_, i) => `s${i}.example`);
    risovat(pravila({ vkl: true, uroven: "light", razresheno: polno }));
    vvesti("novyy.example");
    fireEvent.click(screen.getByRole("button", { name: "Добавить в черновик" }));
    expect(screen.getByRole("status")).toHaveTextContent("Исключений может быть не больше 256");
    expect(screen.queryByText("Есть неприменённые изменения")).toBeNull();
  });

  it("исключение удаляется вторым нажатием", () => {
    risovat(pravila({ vkl: true, uroven: "light", razresheno: ["mc.yandex.ru", "ad.example"] }));
    fireEvent.click(screen.getByRole("button", { name: "Удалить исключение mc.yandex.ru" }));
    expect(screen.getByRole("button", { name: "Удалить исключение ad.example" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Подтвердить удаление mc.yandex.ru" }));
    expect(screen.queryByRole("button", { name: /mc\.yandex\.ru/ })).toBeNull();
    expect(screen.getByText("Есть неприменённые изменения")).toBeInTheDocument();
  });

  it("пустой список исключений так и назван", () => {
    risovat();
    expect(screen.getByText("Исключений нет")).toBeInTheDocument();
  });

  it("пока служба занята подъёмом, вкладка неактивна целиком", () => {
    risovat(pravila({ vkl: true, uroven: "light", razresheno: ["mc.yandex.ru"] }), { sostoyanie: "podnimaetsya" });
    expect(screen.getByTestId("reklama-vkl")).toBeDisabled();
    expect(screen.getByRole("radio", { name: "Базовый" })).toBeDisabled();
    expect(screen.getByLabelText("Сайт-исключение")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Удалить исключение mc.yandex.ru" })).toBeDisabled();
  });

  it("под блокировкой сети вне VPN переключатель не гаснет", () => {
    risovat(pravila(VYKL_SLUZHBY), { sostoyanie: "podnyat", kill_switch: true });
    expect(screen.getByTestId("reklama-vkl")).toBeEnabled();
  });

  it("смена сохранённой настройки из другого окна это конфликт черновика", () => {
    const { pererisovat } = risovat();
    fireEvent.click(screen.getByTestId("reklama-vkl"));
    pererisovat(pravila({ ...VYKL_SLUZHBY, uroven: "multi" }, "r2"));
    expect(screen.getByRole("alert")).toHaveTextContent("Сохранённые правила изменились");
  });

  it("ключи в порядке службы не дают ложного черновика", () => {
    risovat();
    fireEvent.click(screen.getByTestId("reklama-vkl"));
    fireEvent.click(screen.getByTestId("reklama-vkl"));
    expect(screen.queryByText("Есть неприменённые изменения")).toBeNull();
  });

  it("строки состояния считаются по сохранённому, а не по черновику", () => {
    const st: ReklamaSostoyanie = { uroven: "light", pravil: 44757, sobran: "2026-09-28T08:46:00Z", deystvuet: false, otkaz: "ядро не приняло файл списка" };
    risovat(pravila({ vkl: true, uroven: "light", razresheno: [] }), { sostoyanie: "podnyat", reklama: st });
    expect(screen.getByTestId("reklama-vazhnoe")).toHaveTextContent("На этом подключении блокировка не действует: ядро не приняло файл списка");
    fireEvent.click(screen.getByTestId("reklama-vkl"));
    expect(screen.getByTestId("reklama-vazhnoe")).toBeInTheDocument();
  });
});

describe("служба прошлой версии", () => {
  it("без reklama в listRules вкладки нет, а тело setRules прежнее", async () => {
    const send = vi.fn<(komanda: string, telo: unknown) => Promise<boolean>>().mockResolvedValue(true);
    render(<Pravila status={VYKL} pravila={pravila()} otlozheno={{}} naKomandu={send} />);
    expect(screen.queryByRole("tab", { name: /Реклама/ })).toBeNull();
    fireEvent.click(screen.getByRole("radio", { name: "Только выбранное" }));
    await primenit();
    expect(send).toHaveBeenCalledWith("setRules", { trafik: { ...TRAFIK, po_umolchaniyu: "direct" }, bez_ru_spiska: false, reviziya_pravil: "r1" });
  });
});
