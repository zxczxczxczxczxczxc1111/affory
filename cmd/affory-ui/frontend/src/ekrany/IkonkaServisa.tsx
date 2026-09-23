import youtube from "../assets/services/youtube.svg";
import discord from "../assets/services/discord.svg";
import chatgpt from "../assets/services/openai.svg";
import instagram from "../assets/services/instagram.svg";
import claude from "../assets/services/claude.svg";
import telegram from "../assets/services/telegram.svg";
import spotify from "../assets/services/spotify.svg";
import soundcloud from "../assets/services/soundcloud.svg";
import steam from "../assets/services/steam.svg";
import epicgames from "../assets/services/epicgames.svg";
import battlenet from "../assets/services/battledotnet.svg";
import riot from "../assets/services/riotgames.svg";
import ubisoft from "../assets/services/ubisoft.svg";
import { ZnachokServisa } from "./ui";

const icons: Readonly<Record<string, string>> = {
  youtube, discord, chatgpt, instagram, claude, telegram, spotify, soundcloud,
  steam, epicgames, battlenet, riot, ubisoft,
};

/** Знак сервиса одним цветом. Тринадцать чужих логотипов в своих палитрах
 *  превращали строгий экран в набор наклеек, поэтому цвет здесь берётся у
 *  текста, а от файла остаётся только форма. Незнакомый сервис показывает
 *  две буквы имени: каталог обновляется отдельно от программы. */
export function IkonkaServisa({ id, imya }: { id: string; imya: string }) {
  // Bundled marks: an offline routing screen should not need its own VPN rescue.
  const icon = Object.prototype.hasOwnProperty.call(icons, id) ? icons[id] : undefined;
  return icon ? (
    <ZnachokServisa src={icon} className="h-[18px] w-[18px]" />
  ) : (
    <span aria-hidden="true" className="text-[11px] font-semibold uppercase">{imya.slice(0, 2)}</span>
  );
}
