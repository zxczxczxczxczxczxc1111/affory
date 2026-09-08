package petlya

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zxczxczxczxczxczxc1111/affory/internal/genkonfig"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/protokol"
	"github.com/zxczxczxczxczxczxc1111/affory/internal/yadra"
)

// Переключение между серверами.
//
// Сегодняшний судья на стенде печатает НЕГОДЕН уже 194 секунды подряд и делает
// это по делу: у всех входов прода один внешний адрес, и отличить сервер A от
// сервера B по трафику снаружи нельзя. Здесь у каждого сервера СВОЯ мишень, и
// смена несущего читается сменой имени в ответе.
func TestPereklyuchenieMenyaetNesushchego(t *testing.T) {
	a, b := ParaUzlov(t)
	k := PodnyatVybor(t, Vybor{Uzly: []Uzel{a.Uzel, b.Uzel}})

	if imya := k.SprositCherezProksi(t, a.Adres); imya != a.Imya {
		t.Fatalf("до переключения несёт %q, а выбран был %q", imya, a.Imya)
	}

	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Second)
	defer otmena()
	if err := yadra.PostavitVybor(ctx, k.AdresKlash, k.SekretKlash,
		genkonfig.TegSelector, b.Uzel.Teg(t)); err != nil {
		t.Fatalf("продуктовый код не переключил селектор: %v", err)
	}

	if imya := k.SprositCherezProksi(t, a.Adres); imya != b.Imya {
		t.Errorf("после переключения несёт %q, а ждали %q", imya, b.Imya)
	}
}

// Холодный подъём: несущим встаёт ВЫБРАННЫЙ сервер, а не первый по списку.
func TestHolodnyyPodyomNesyotVybrannogo(t *testing.T) {
	a, b := ParaUzlov(t)
	// Выбран второй, в списке он тоже второй: если ядро молча берёт первого,
	// это видно сразу.
	k := PodnyatVybor(t, Vybor{Uzly: []Uzel{a.Uzel, b.Uzel}, Vybran: 1})

	if imya := k.SprositCherezProksi(t, a.Adres); imya != b.Imya {
		t.Errorf("холодный подъём несёт %q, а выбран был %q", imya, b.Imya)
	}
}

// Авто-режим: urltest обязан выбрать ЖИВОГО из живого и мёртвого.
func TestAvtoVybiraetZhivogo(t *testing.T) {
	zhivoy := UzelKMisheni(t, NovayaMishenNaAdrese(t, "127.0.0.2", "zhivoy"))
	myortvyy := MyortvyyUzel(t)

	k := PodnyatVybor(t, Vybor{
		// Мёртвый первым: авто, берущее первого попавшегося, покраснеет.
		Uzly:   []Uzel{myortvyy, zhivoy.Uzel},
		Rezhim: protokol.RezhimAvto,
	})

	if imya := k.SprositCherezProksi(t, zhivoy.Adres); imya != zhivoy.Imya {
		t.Errorf("авто несёт %q, а живой это %q", imya, zhivoy.Imya)
	}
}

// Авто-режим должен стоять НА ГРУППЕ, а не на сервере: иначе «авто» это
// чучело, которое ничего не переизбирает.
func TestAvtoStoitNaGruppe(t *testing.T) {
	a, b := ParaUzlov(t)
	k := PodnyatVybor(t, Vybor{Uzly: []Uzel{a.Uzel, b.Uzel}, Rezhim: protokol.RezhimAvto})

	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Second)
	defer otmena()
	vybor, err := yadra.VyborGruppy(ctx, k.AdresKlash, k.SekretKlash, genkonfig.TegSelector)
	if err != nil {
		t.Fatalf("выбор группы не прочитался: %v", err)
	}
	if vybor != genkonfig.TegAvto {
		t.Errorf("в авто-режиме селектор стоит на %q, а должен на %q", vybor, genkonfig.TegAvto)
	}
}

// Кэш ядра не перебивает выбор.
//
// Ровно этот класс сидел в продукте до 04.09.2026 и не проверялся НИ ОДНИМ из
// двадцати пяти судей: ядро восстанавливало прошлый выбор из cache_file и
// перебивало им default из конфига, то есть человек выбирал один сервер, а нёс
// другой. Стык был ничей: судья переключения меряет живое переключение, судья
// авто меряет urltest.
func TestKeshYadraNePerebivaetVybor(t *testing.T) {
	a, b := ParaUzlov(t)
	kesh := filepath.Join(t.TempDir(), "kesh.db")

	// Кэш и конфиг тянут в РАЗНЫЕ стороны: выбран B, а в кэш кладётся A.
	// Совпадение сделало бы тест зелёным при любом поведении ядра, и первая же
	// мутация это показала: с кэшем, совпадающим с умолчанием, снятый default
	// не менял ничего.
	pervyy := PodnyatVybor(t, Vybor{Uzly: []Uzel{a.Uzel, b.Uzel}, Vybran: 1, Kesh: kesh})
	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Second)
	defer otmena()
	if err := yadra.PostavitVybor(ctx, pervyy.AdresKlash, pervyy.SekretKlash,
		genkonfig.TegSelector, a.Uzel.Teg(t)); err != nil {
		t.Fatalf("продуктовый код не переключил селектор: %v", err)
	}
	if imya := pervyy.SprositCherezProksi(t, a.Adres); imya != a.Imya {
		t.Fatalf("до перезапуска несёт %q, а переключали на %q", imya, a.Imya)
	}
	pervyy.Pogasit()

	// Второй подъём с ТЕМ ЖЕ кэшем и тем же выбором B. Побеждать обязан конфиг.
	vtoroy := PodnyatVybor(t, Vybor{Uzly: []Uzel{a.Uzel, b.Uzel}, Vybran: 1, Kesh: kesh})
	if imya := vtoroy.SprositCherezProksi(t, a.Adres); imya != b.Imya {
		t.Errorf("после перезапуска несёт %q, а выбран %q: кэш перебил выбор", imya, b.Imya)
	}
}

// Переход авто в ручной.
//
// В гостевом прогоне 07.09.2026 судья авто напечатал «переход авто->ручной не
// сработал: режим 'avto', выбран ”». Здесь проверяется ядровая половина того
// же перехода: если она зелёная, искать надо в службе, а не в ядре, и это уже
// половина разбора.
func TestPerehodAvtoVRuchnoy(t *testing.T) {
	a, b := ParaUzlov(t)
	k := PodnyatVybor(t, Vybor{Uzly: []Uzel{a.Uzel, b.Uzel}, Rezhim: protokol.RezhimAvto})

	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Second)
	defer otmena()
	if err := yadra.PostavitVybor(ctx, k.AdresKlash, k.SekretKlash,
		genkonfig.TegSelector, b.Uzel.Teg(t)); err != nil {
		t.Fatalf("переход в ручной не прошёл: %v", err)
	}

	vybor, err := yadra.VyborGruppy(ctx, k.AdresKlash, k.SekretKlash, genkonfig.TegSelector)
	if err != nil {
		t.Fatalf("выбор группы не прочитался: %v", err)
	}
	if vybor != b.Uzel.Teg(t) {
		t.Errorf("после перехода в ручной селектор стоит на %q, а ставили %q", vybor, b.Uzel.Teg(t))
	}
	if imya := k.SprositCherezProksi(t, a.Adres); imya != b.Imya {
		t.Errorf("после перехода в ручной несёт %q, а выбран %q", imya, b.Imya)
	}
}

// Обратный переход, ручной в авто: селектор обязан встать НА ГРУППУ, а не
// остаться на сервере. Именно эта половина и печаталась пустой в госте.
func TestPerehodRuchnogoVAvto(t *testing.T) {
	a, b := ParaUzlov(t)
	k := PodnyatVybor(t, Vybor{Uzly: []Uzel{a.Uzel, b.Uzel}})

	ctx, otmena := context.WithTimeout(context.Background(), 10*time.Second)
	defer otmena()
	if err := yadra.PostavitVybor(ctx, k.AdresKlash, k.SekretKlash,
		genkonfig.TegSelector, genkonfig.TegAvto); err != nil {
		t.Fatalf("переход в авто не прошёл: %v", err)
	}

	vybor, err := yadra.VyborGruppy(ctx, k.AdresKlash, k.SekretKlash, genkonfig.TegSelector)
	if err != nil {
		t.Fatalf("выбор группы не прочитался: %v", err)
	}
	if vybor != genkonfig.TegAvto {
		t.Errorf("после перехода в авто селектор стоит на %q, а должен на %q", vybor, genkonfig.TegAvto)
	}
	// Несущего у группы спрашивают отдельным продуктовым вызовом: селектор
	// показывает группу, а не сервер, и «кто именно несёт» это другой вопрос.
	nesyot, err := yadra.Nesyot(ctx, k.AdresKlash, k.SekretKlash, genkonfig.TegSelector, genkonfig.TegAvto)
	if err != nil {
		t.Fatalf("несущий не прочитался: %v", err)
	}
	if nesyot != a.Uzel.Teg(t) && nesyot != b.Uzel.Teg(t) {
		t.Errorf("несущим объявлен %q, а кандидатов было двое: %q и %q",
			nesyot, a.Uzel.Teg(t), b.Uzel.Teg(t))
	}
}
