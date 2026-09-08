package sostoyanie

import "path/filepath"

// Two directories, two owners. Program files are read-only to the user; data is
// SYSTEM-only with inheritance broken, because it inherits from C:\ProgramData
// otherwise, and that hands every authenticated user a write bit on new files.
func KatalogProgrammy() string { return filepath.Join(`C:\Program Files`, "Affory") }
func KatalogDannyh() string    { return filepath.Join(`C:\ProgramData`, "Affory") }

// Журналы компонентов лежат в подкаталоге данных, по файлу на компонент (§5
// п.6 спеки). Права наследуются от каталога данных через (OI)(CI): читают
// SYSTEM и админы, больше никто.
func KatalogZhurnalov() string { return filepath.Join(KatalogDannyh(), "log") }
