package sostoyanie

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ZavestiKatalogDannyh creates the data directory and cuts it off from whatever
// C:\ProgramData hands down. Everything in here is either a secret (DPAPI blobs,
// the subscription URL) or a measuring instrument the acceptance thresholds are
// read from, and neither should be writable by a normal account.
//
// SIDs, not names. On a Russian Windows the group is "Администраторы" and the
// account is "СИСТЕМА"; icacls with English names fails there with a message
// about an invalid parameter, and the failure looks like a bug in our code.
func ZavestiKatalogDannyh() error {
	if err := zavestiKatalog(KatalogDannyh()); err != nil {
		return err
	}
	return otkrytZhurnaly(KatalogZhurnalov())
}

// otkrytZhurnaly даёт пользователям машины ЧТЕНИЕ папки журналов (01.10.2026).
//
// До этого журналы лежали под тем же замком, что и ключи (О6 аудита 1.6.1), и
// кнопка «Открыть папку с журналами» не работала ни у кого: папку открывает
// Проводник, а он идёт с обычным токеном даже у администратора, у которого
// окно запущено с повышением. Владелец решил открыть чтение: в журналах нет
// ключей, а адреса серверов человек и так видит в окне. Писать по-прежнему
// может только служба. Остальной каталог данных остаётся запертым.
func otkrytZhurnaly(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("папка журналов не создана: %w", err)
	}
	if err := zakrytDerevo(dir, sidyZhurnalov); err != nil {
		return fmt.Errorf("права папки журналов не выставлены: %w", err)
	}
	return nil
}

// ErrKatalogObshchiy означает, что каталог программы не отдельный наш, а общий
// (`D:\Games`): права в нём не наши, и переписывать их нельзя.
var ErrKatalogObshchiy = errors.New("каталог программы общий, а не отдельный каталог Affory")

// ErrKatalogSsylka означает, что на месте каталога стоит ссылка на другой.
var ErrKatalogSsylka = errors.New("на месте каталога стоит ссылка на другой каталог")

// ZakrytKatalogProgrammy запирает каталог программы от записи обычным
// пользователем: SYSTEM и администраторы пишут, остальные читают и запускают.
//
// Каталог программы до 1.6.2 прав не получал вовсе и наследовал их от
// родителя. В `C:\Program Files` это безопасно, а в `C:\Affory` родитель это
// корень диска, где любой вошедший может менять созданное. Подменённый
// affory-svc.exe запускался бы при следующем старте от SYSTEM, подменённый
// affory-ui.exe у всех при входе (пункт К2 аудита 1.6.1).
//
// Трогается только каталог по имени Affory. Установка вне его (прежняя, в
// `D:\Games`) оставлена как есть: запереть общий каталог значит отнять его у
// человека вместе со всем, что там лежит.
func ZakrytKatalogProgrammy(dir string) error {
	if !strings.EqualFold(filepath.Base(filepath.Clean(dir)), "Affory") {
		return fmt.Errorf("%w: %s", ErrKatalogObshchiy, dir)
	}
	return zakrytDerevo(dir, sidyProgrammy)
}

// polnyyDostupKFaylu это то же, что (F) у icacls: стандартные права целиком
// вместе со всеми правами файловой системы.
const polnyyDostupKFaylu = windows.STANDARD_RIGHTS_ALL | windows.FILE_GENERIC_READ |
	windows.FILE_GENERIC_WRITE | windows.FILE_GENERIC_EXECUTE | windows.DELETE |
	windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA

// chtenieIZapusk это (RX) у icacls.
const chtenieIZapusk = windows.FILE_GENERIC_READ | windows.FILE_GENERIC_EXECUTE

// Кому что. Каталог данных только SYSTEM и администраторам, каталог программы
// ещё и на чтение с запуском всем пользователям и приложениям из магазина, как
// у `C:\Program Files`.
var (
	sidyDannyh = []dostupSida{
		{"S-1-5-18", polnyyDostupKFaylu, windows.TRUSTEE_IS_USER},      // LocalSystem
		{"S-1-5-32-544", polnyyDostupKFaylu, windows.TRUSTEE_IS_GROUP}, // BUILTIN\Administrators
	}
	sidyProgrammy = append(append([]dostupSida{}, sidyDannyh...),
		dostupSida{"S-1-5-32-545", chtenieIZapusk, windows.TRUSTEE_IS_GROUP}, // BUILTIN\Users
		dostupSida{"S-1-15-2-1", chtenieIZapusk, windows.TRUSTEE_IS_GROUP},   // ALL APPLICATION PACKAGES
	)
	// Журналы читают пользователи машины, пишет только служба (otkrytZhurnaly).
	sidyZhurnalov = append(append([]dostupSida{}, sidyDannyh...),
		dostupSida{"S-1-5-32-545", chtenieIZapusk, windows.TRUSTEE_IS_GROUP}, // BUILTIN\Users
	)
)

type dostupSida struct {
	sid   string
	prava windows.ACCESS_MASK
	tip   windows.TRUSTEE_TYPE
}

// zavestiKatalog ставит каталогу список доступа ЦЕЛИКОМ, а не правит его.
//
// Прежде это делал icacls: `/inheritance:r` плюс два `/grant:r`. Ни то, ни
// другое не трогает явную запись для ПОСТОРОННЕГО SID, и на рабочей машине
// 12.09.2026 в каталоге ключей так и жила третья запись, на учётку человека.
// Это доступ к DPAPI-блобам без всякого повышения прав, то есть ровно та дыра,
// ради закрытия которой каталог и запирается.
//
// Список, собранный здесь и поставленный одним вызовом, не может содержать
// лишнего по построению: чего в нём не перечислено, того нет. Заодно уходит
// запуск чужой программы с разбором её кода возврата.
func zavestiKatalog(k string) error {
	if err := os.MkdirAll(k, 0o700); err != nil {
		return fmt.Errorf("каталог данных не создан: %w", err)
	}
	if err := zakrytDerevo(k, sidyDannyh); err != nil {
		return fmt.Errorf("права каталога данных не выставлены: %w", err)
	}
	return nil
}

// zakrytDerevo ставит корню и всему внутри владельца Administrators и
// защищённый список доступа из sidy.
//
// Владелец меняется не для порядка. Каталог, заведённый заранее обычным
// пользователем, остаётся ЕГО: владелец всегда может переписать список доступа,
// сколько бы мы его ни запирали, и вернуть себе чтение DPAPI-блобов или запись
// в каталог программы. Содержимое переписывается по той же причине: файл,
// подложенный заранее со своей явной записью, сохранил бы её поверх
// наследования.
//
// Всё делается через ОТКРЫТЫЙ дескриптор без перехода по ссылкам. Путь с
// точкой соединения на месте каталога увёл бы права, выставляемые от SYSTEM,
// в чужой каталог, например в System32. Ссылка на месте корня это отказ,
// ссылка внутри пропускается.
//
// Отказ на корне возвращается, отказы на содержимом только пишутся в журнал:
// файл, занятый антивирусом, не повод оставлять незапертым весь каталог.
func zakrytDerevo(koren string, sidy []dostupSida) error {
	vernut := vklyuchitPrivilegii("SeRestorePrivilege", "SeTakeOwnershipPrivilege")
	defer vernut()

	adminy, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		return fmt.Errorf("SID администраторов не разобран: %w", err)
	}
	dlyaKatalogov, err := spisokDostupa(sidy, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT)
	if err != nil {
		return err
	}
	dlyaFaylov, err := spisokDostupa(sidy, windows.NO_INHERITANCE)
	if err != nil {
		return err
	}

	katalog, err := zakrytOdin(koren, adminy, dlyaKatalogov, dlyaFaylov)
	if err != nil {
		return err
	}
	if !katalog {
		return fmt.Errorf("%s не каталог", koren)
	}
	zakrytSoderzhimoe(koren, adminy, dlyaKatalogov, dlyaFaylov, 0)
	return nil
}

// glubinaObhoda это потолок вложенности. В наших каталогах её два уровня
// (`novaya`, `log`), и глубокое дерево там может быть только чужим.
const glubinaObhoda = 8

func zakrytSoderzhimoe(dir string, adminy *windows.SID, dlyaKatalogov, dlyaFaylov *windows.ACL, glubina int) {
	if glubina >= glubinaObhoda {
		log.Printf("права: %s глубже %d уровней, дальше не иду", dir, glubinaObhoda)
		return
	}
	zapisi, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("права: содержимое %s не читается: %v", dir, err)
		return
	}
	for _, z := range zapisi {
		put := filepath.Join(dir, z.Name())
		katalog, err := zakrytOdin(put, adminy, dlyaKatalogov, dlyaFaylov)
		if err != nil {
			log.Printf("права: %v", err)
			continue
		}
		if katalog {
			zakrytSoderzhimoe(put, adminy, dlyaKatalogov, dlyaFaylov, glubina+1)
		}
	}
}

// zakrytOdin ставит права одному объекту и говорит, каталог ли это.
func zakrytOdin(put string, adminy *windows.SID, dlyaKatalogov, dlyaFaylov *windows.ACL) (bool, error) {
	p, err := windows.UTF16PtrFromString(put)
	if err != nil {
		return false, fmt.Errorf("%s: %w", put, err)
	}
	// BACKUP_SEMANTICS нужен, чтобы открыть каталог, и вместе с включённым
	// SeRestorePrivilege даёт право на запись прав даже там, где чужой список
	// доступа закрыл объект от администраторов. OPEN_REPARSE_POINT открывает
	// саму ссылку, а не то, куда она ведёт.
	h, err := windows.CreateFile(p,
		windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return false, fmt.Errorf("%s не открывается для смены прав: %w", put, err)
	}
	defer windows.CloseHandle(h)

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return false, fmt.Errorf("%s: сведения не читаются: %w", put, err)
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return false, fmt.Errorf("%w: %s", ErrKatalogSsylka, put)
	}
	katalog := info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	acl := dlyaFaylov
	if katalog {
		acl = dlyaKatalogov
	}
	// PROTECTED_DACL_SECURITY_INFORMATION это то же, что `/inheritance:r`:
	// связь с родителем рвётся, и унаследованное больше не приезжает.
	if err := windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		adminy, nil, acl, nil); err != nil {
		return katalog, fmt.Errorf("%s: права не выставлены: %w", put, err)
	}
	return katalog, nil
}

// spisokDostupa собирает список доступа из sidy.
//
// Полный доступ конкретной маской, а не GENERIC_ALL: родовые права вместе с
// наследованием Windows разворачивает в ДВЕ записи на каждое доверенное лицо,
// действующую и наследуемую, и список из двух записей превращается в список из
// четырёх.
//
// Наследование на содержимое у каталогов: файлы в них создаются потом, и без
// этих флагов они получили бы права от родителя, от которого мы только что
// отвязались. У файлов наследовать нечему, и флаги им не ставятся.
func spisokDostupa(sidy []dostupSida, nasledovanie uint32) (*windows.ACL, error) {
	dostup := make([]windows.EXPLICIT_ACCESS, 0, len(sidy))
	for _, s := range sidy {
		sid, err := windows.StringToSid(s.sid)
		if err != nil {
			return nil, fmt.Errorf("SID %s не разобран: %w", s.sid, err)
		}
		dostup = append(dostup, windows.EXPLICIT_ACCESS{
			AccessPermissions: s.prava,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       nasledovanie,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  s.tip,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	acl, err := windows.ACLFromEntries(dostup, nil)
	if err != nil {
		return nil, fmt.Errorf("список доступа не собран: %w", err)
	}
	return acl, nil
}

// vklyuchitPrivilegii включает привилегии процессу и отдаёт функцию, которая
// возвращает прежнее состояние. Включённый навсегда SeRestorePrivilege дал бы
// службе запись мимо списков доступа везде, где она откроет файл с
// BACKUP_SEMANTICS, а нужен он ровно на время смены прав.
//
// Привилегии, которых у процесса нет (тест без повышения), просто не
// включаются: смена прав тогда упрётся в обычную проверку доступа.
func vklyuchitPrivilegii(imena ...string) func() {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &tok); err != nil {
		return func() {}
	}
	var prezhnie []windows.Tokenprivileges
	for _, imya := range imena {
		var luid windows.LUID
		if err := windows.LookupPrivilegeValue(nil, windows.StringToUTF16Ptr(imya), &luid); err != nil {
			continue
		}
		novoe := windows.Tokenprivileges{PrivilegeCount: 1}
		novoe.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
		var prezhnee windows.Tokenprivileges
		var dlina uint32
		if err := windows.AdjustTokenPrivileges(tok, false, &novoe,
			uint32(unsafe.Sizeof(prezhnee)), &prezhnee, &dlina); err != nil {
			continue
		}
		// Пустое прежнее состояние значит, что привилегия уже была включена или
		// её нет вовсе: возвращать нечего.
		if prezhnee.PrivilegeCount > 0 {
			prezhnie = append(prezhnie, prezhnee)
		}
	}
	return func() {
		for i := len(prezhnie) - 1; i >= 0; i-- {
			_ = windows.AdjustTokenPrivileges(tok, false, &prezhnie[i], 0, nil, nil)
		}
		_ = tok.Close()
	}
}
