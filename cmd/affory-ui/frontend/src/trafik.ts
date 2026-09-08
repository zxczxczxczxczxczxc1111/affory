// Routes describe intent explicitly; changing a default must not flip every switch.
export type Marshrut = "vpn" | "direct";
export interface PraviloPrilozheniya {
  put: string;
  imya: string;
  potomki: boolean;
  marshrut: Marshrut;
}
export interface PraviloDomena {
  domen: string;
  marshrut: Marshrut;
}
export interface PraviloServisa {
  id: string;
  marshrut: Marshrut;
}
export interface PravilaTrafika {
  po_umolchaniyu: Marshrut;
  prilozheniya: PraviloPrilozheniya[];
  domeny: PraviloDomena[];
  servisy: PraviloServisa[];
}
export interface KatalogServisov {
  versiya: string;
  istochnik: string;
  servisy: { id: string; imya: string; domeny: string[]; istochnik: string }[];
}
export const imyaMarshruta = (route: Marshrut) =>
  route === "vpn" ? "Через VPN" : "Напрямую";
