import youtube from "../assets/services/youtube.svg";
import discord from "../assets/services/discord.svg";
import chatgpt from "../assets/services/openai.svg";
import instagram from "../assets/services/instagram.svg";
import whatsapp from "../assets/services/whatsapp.svg";
import telegram from "../assets/services/telegram.svg";
import spotify from "../assets/services/spotify.svg";
import netflix from "../assets/services/netflix.svg";

const icons: Readonly<Record<string, string>> = {
  youtube, discord, chatgpt, instagram, whatsapp, telegram, spotify, netflix,
};

export function IkonkaServisa({ id, imya }: { id: string; imya: string }) {
  // Bundled marks: an offline routing screen should not need its own VPN rescue.
  const icon = Object.prototype.hasOwnProperty.call(icons, id) ? icons[id] : undefined;
  return (
    <span className="af-monogram af-service-icon" aria-hidden="true">
      {icon ? <img src={icon} alt="" width={21} height={21} draggable={false} /> : imya.slice(0, 2)}
    </span>
  );
}
