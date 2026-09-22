// Клавиатура полосы вкладок, одна на все полосы окна: «Разделы» в заголовке и
// «Вид правил» внутри правил. До 22.09.2026 стрелки в обеих не делали ничего, а
// Tab проходил через каждую вкладку по очереди, поэтому дойти с клавиатуры до
// содержимого раздела стоило четырёх нажатий вместо одного.
//
// Порядок взят из ARIA APG для tablist: стрелки ходят по кругу, Home и End
// прыгают к краям, в обычный порядок Tab попадает только ВЫБРАННАЯ вкладка
// (roving tabindex), а выбор следует за фокусом.

/** Куда перейти по клавише. `null` значит «клавиша не наша, не мешать». */
export function sleduyushchayaVkladka(klavisha: string, tekushchaya: number, vsego: number): number | null {
  if (vsego <= 0) return null;
  // Круг намеренный: полоса короткая, и упор в край на четырёх вкладках
  // читается как заевшая клавиша, а не как граница списка.
  if (klavisha === "ArrowRight" || klavisha === "ArrowDown") return (tekushchaya + 1) % vsego;
  if (klavisha === "ArrowLeft" || klavisha === "ArrowUp") return (tekushchaya - 1 + vsego) % vsego;
  if (klavisha === "Home") return 0;
  if (klavisha === "End") return vsego - 1;
  return null;
}
