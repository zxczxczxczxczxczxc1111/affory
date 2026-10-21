package sobytiya

import (
	"strings"
	"testing"
)

// Образец как его отдаёт EvtRender: пространство имён, Qualifiers у кода,
// время в UTC с семью знаками долей.
const obrazecSCM = `<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System>` +
	`<Provider Name='Service Control Manager' Guid='{555908d1-a6d7-4695-8e1e-26931d2012f4}' EventSourceName='Service Control Manager'/>` +
	`<EventID Qualifiers='49152'>7031</EventID><Version>0</Version><Level>2</Level>` +
	`<TimeCreated SystemTime='2026-10-21T13:40:12.1234567Z'/><Channel>System</Channel></System>` +
	`<EventData><Data Name='param1'>Affory</Data><Data Name='param2'>1</Data><Data Name='param3'>5000</Data>` +
	`<Data Name='param4'>1</Data><Data Name='param5'>Перезапуск службы</Data></EventData></Event>`

func TestRazobratSobytieSCM(t *testing.T) {
	z, err := razobratXML(obrazecSCM)
	if err != nil {
		t.Fatal(err)
	}
	if z.Istochnik != "Service Control Manager" || z.Kod != 7031 {
		t.Fatalf("источник %q, код %d", z.Istochnik, z.Kod)
	}
	if z.Vremya.IsZero() || z.Vremya.UTC().Hour() != 13 {
		t.Fatalf("время %v", z.Vremya)
	}
	if !z.Upominaet("affory") || z.Upominaet("sing-box.exe") {
		t.Fatal("упоминание определено неверно")
	}
	if s := z.Stroka(); !strings.Contains(s, "7031: Affory | 1 | 5000") {
		t.Fatalf("строка %q", s)
	}
}

// У отчёта WER данные без имён, и пустые поля в выгрузку не идут.
func TestRazobratWERBezImen(t *testing.T) {
	z, err := razobratXML(`<Event xmlns='http://schemas.microsoft.com/win/2004/08/events/event'><System>` +
		`<Provider Name='Windows Error Reporting'/><EventID>1001</EventID>` +
		`<TimeCreated SystemTime='2026-10-20T08:00:00.0Z'/></System>` +
		`<EventData><Data>0</Data><Data>APPCRASH</Data><Data></Data><Data>affory-ui.exe</Data></EventData></Event>`)
	if err != nil {
		t.Fatal(err)
	}
	if len(z.Dannye) != 3 || !z.Upominaet("affory-") {
		t.Fatalf("данные %q", z.Dannye)
	}
}

// Живой журнал: запрос годный и читается без прав администратора. Событий
// может не быть вовсе, это не провал.
func TestProchitatZhivoyZhurnal(t *testing.T) {
	zap, err := Prochitat("System", "*[System[TimeCreated[timediff(@SystemTime) <= 86400000]]]", 3, nil)
	if err != nil {
		t.Fatalf("журнал System не прочитан: %v", err)
	}
	if len(zap) > 3 {
		t.Fatalf("предел 3, отдано %d", len(zap))
	}
	for _, z := range zap {
		if z.Istochnik == "" || z.Vremya.IsZero() {
			t.Errorf("событие разобрано пустым: %+v", z)
		}
		t.Log(z.Stroka())
	}
}
