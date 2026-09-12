package sostoyanie

import (
	"fmt"
	"os"

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
	return zavestiKatalog(KatalogDannyh())
}

// polnyyDostupKFaylu это то же, что (F) у icacls: стандартные права целиком
// вместе со всеми правами файловой системы.
const polnyyDostupKFaylu = windows.STANDARD_RIGHTS_ALL | windows.FILE_GENERIC_READ |
	windows.FILE_GENERIC_WRITE | windows.FILE_GENERIC_EXECUTE | windows.DELETE |
	windows.FILE_WRITE_ATTRIBUTES | windows.FILE_WRITE_EA

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

	sistema, err := windows.StringToSid("S-1-5-18") // LocalSystem
	if err != nil {
		return fmt.Errorf("SID системы не разобран: %w", err)
	}
	adminy, err := windows.StringToSid("S-1-5-32-544") // BUILTIN\Administrators
	if err != nil {
		return fmt.Errorf("SID администраторов не разобран: %w", err)
	}

	// Полный доступ конкретной маской, а не GENERIC_ALL: родовые права вместе с
	// наследованием Windows разворачивает в ДВЕ записи на каждое доверенное
	// лицо, действующую и наследуемую, и список из двух записей превращается в
	// список из четырёх.
	//
	// Наследование на содержимое: каталог заводится пустым, но файлы в нём
	// создаются потом, и без этих флагов они получили бы права от родителя, то
	// есть от C:\ProgramData, откуда мы только что отвязались.
	var nasledovanie uint32 = windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT
	dostup := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: polnyyDostupKFaylu,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       nasledovanie,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(sistema),
			},
		},
		{
			AccessPermissions: polnyyDostupKFaylu,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       nasledovanie,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_GROUP,
				TrusteeValue: windows.TrusteeValueFromSID(adminy),
			},
		},
	}
	acl, err := windows.ACLFromEntries(dostup, nil)
	if err != nil {
		return fmt.Errorf("список доступа не собран: %w", err)
	}

	// PROTECTED_DACL_SECURITY_INFORMATION это то же, что `/inheritance:r`:
	// связь с родителем рвётся, и унаследованное больше не приезжает.
	if err := windows.SetNamedSecurityInfo(k, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil); err != nil {
		return fmt.Errorf("права каталога данных не выставлены: %w", err)
	}
	return nil
}
