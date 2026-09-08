package protokol

// Двадцать девять кодов, и ни одного сверх без строки в §9.1 спеки. Свободный
// текст ошибки это экран, который никто не проектировал, и тест, который некому
// написать.
//
// Число и состав сверяются воротами, а не глазами: 02.09.2026 сверка нашла ТРИ
// расхождения разом. Заголовок говорил «двадцать два» при двадцати четырёх, а у
// `tunnel-not-carrying` и `name-resolve-failed` строки в спеке не было вовсе,
// хотя оба живут в проде. Правило без проверки протухает ровно так.
const (
	KodDnsResolveFailed      = "dns-resolve-failed"
	KodServerAuthFailed      = "server-auth-failed"
	KodSubscriptionUnreach   = "subscription-unreachable"
	KodSubscriptionMalformed = "subscription-malformed"
	// Двадцать первый, добавлен волной 3 вместе со строкой в §9.1 спеки. Обе
	// проверенные панели отвечают на истекшую подписку кодом 200 и исправными
	// ссылками на 0.0.0.0: без своего кода это либо «подписка отдала
	// непонятное» (человек ищет опечатку вместо оплаты), либо пустой список
	// (теряется текст панели вместе с кодом оплаты).
	KodSubscriptionExpired = "subscription-expired"
	KodSelectedServerGone  = "selected-server-gone"
	KodTunCreateFailed     = "tun-create-failed"
	KodWintunMissing       = "wintun-missing"
	KodNoAdmin             = "no-admin"
	// Двадцать второй, волна 3, строка в §9.1 спеки есть. НЕ путать с no-admin:
	// тот означает «службы нет» и ведёт на экран установки, а этот означает
	// «служба есть, но эта команда не для тебя». Показать вместо него экран
	// установки значило бы предлагать жать UAC там, где UAC не поможет.
	KodTrebuetsyaAdmin       = "admin-required"
	KodFirewallDisabled      = "firewall-disabled"
	KodFirewallFailed        = "firewall-failed"
	KodKillswitchOrphan      = "killswitch-orphan"
	KodHealthSnapshotStale   = "health-snapshot-stale"
	KodHealthSnapshotMissing = "health-snapshot-missing"
	KodUpdateRollback        = "update-rollback"
	// Проверка и загрузка обновления с сервера обновлений (план «шесть
	// удобств» §5). Раздельно: не проверилось это сеть, не скачалось это
	// архив или хеш, и человеку в первом случае ждать, во втором сообщать.
	KodObnovlenieNeProvereno = "update-check-failed"
	KodObnovlenieNeSkachano  = "update-download-failed"
	KodPipeSquatted          = "pipe-squatted"
	KodSecretsUnreadable     = "secrets-unreadable"
	KodForeignProxyHijack    = "foreign-proxy-hijack"
	KodForeignRegistryHijack = "foreign-registry-hijack"
	KodAllServersDown        = "all-servers-down"
	// Ядро запустилось, а его управляющий порт молчит. Отдельный код, а не
	// KodTunCreateFailed: адаптер к этому моменту как раз создан, и валить на
	// TUN значит отправить человека чинить исправное. И не KodAllServersDown:
	// про серверы тут ещё ничего не известно, спросить было некого.
	KodYadroNeOtvechaet = "core-not-responding"
	// Туннель поднялся, а потом перестал нести трафик. Отдельный код, а не
	// KodAllServersDown: то событие означает «подняться не удалось», это
	// означает «поднятое умерло», и в журнале их обязано быть видно порознь.
	KodTunnelNeNeset    = "tunnel-not-carrying"
	KodProtocolMismatch = "protocol-mismatch"

	// Ядро не приняло исходящий этого сервера при проверке конфига, и сервер
	// ИСКЛЮЧЁН, а не уронил подъём целиком. Заведён 02.09.2026 по находке 43,
	// строка в §9.1 спеки есть.
	//
	// НЕ путать с KodAllServersDown: тот означает «серверы не отвечают», этот
	// означает «сервер даже не собрался в исходящий». Показать вместо него
	// all-servers-down значило бы отправить человека проверять сеть там, где
	// надо исправить ссылку.
	KodServerRejectedByCore = "server-rejected-by-core"

	// Имя сервера не разрешилось, и сервер ИСКЛЮЧЁН из правил, а не уронил
	// работу целиком. Заведён после живого прогона 01.09.2026: один мёртвый
	// узел в подписке из одиннадцати запрещал включить режим «весь трафик».
	KodNameResolveFailed = "name-resolve-failed"

	// Сервер нельзя убрать из набора, пока ядро держит его кандидатом. Горячей
	// перезагрузки конфига у ядра нет, поэтому выпавший из набора сервер
	// остаётся достижимым для ядра и невидимым для набора, экрана и правил
	// брандмауэра. НЕ путать с selected-server-gone: тот про сервер, которого
	// уже нет, этот про сервер, который ещё держат.
	KodKandidatZanyat = "candidate-in-use"

	// Три кода живого переключения, и у каждого СВОЁ действие для человека.
	// Замерено: ядро отвечает 400 на три разные причины, по коду они
	// неразличимы, по телу различимы. Один код на все три отправил бы человека
	// переподключаться там, где переподключение не помогает, и наоборот.
	//
	// Тега нет в конфиге живого ядра. Достижимо честно: сервер добавлен уже
	// после подключения, а конфиг ядра собран при подъёме.
	KodNuzhenPodyom = "switch-needs-reconnect"
	// Ядро команду приняло, но проба через новый выбор не прошла. Выбор возвращён
	// на прежний, подключение цело. Самый частый отказ на практике и
	// единственный, где человек может что-то сделать сам.
	KodNovyyNeNesyot = "switch-target-not-carrying"
	// Всё остальное, включая наш дефект в имени группы и молчание clash_api.
	KodPereklyuchenieNeDoehalo = "switch-failed"
)

// Scaffolding, and NOT one of the twenty. It exists so a command that will be
// written in wave 3 can answer in wave 1 instead of hanging, and it must be gone
// by wave 6: grep for it before calling the product finished.
const KodNeRealizovano = "not-implemented"

// Волна 5. Правило не принято: путь процесса не ведёт к файлу или строка не
// похожа на имя домена. Отдельный код, а не protocol-mismatch: тот на экране
// значит «обнови программу», а тут человеку надо поправить строку.
const KodPraviloNegodno = "rule-invalid"

// Файл журнала соединений не стёрся: обычно его держит открытым просмотрщик.
const KodZhurnalNeStyort = "journal-clear-failed"

// Адрес выхода не измерен: эндпоинт не ответил или локальный прокси не поднят.
const KodVyhodNeIzmeren = "exit-ip-unmeasured"

// Полоса не измерена: мишень не задана, не отдала ни байта, либо замер отменён.
//
// Отдельный код, а не «выход не измерен»: тот про адрес выхода, а этот про
// число, которое пойдёт в объявление hysteria2. Экран на них говорит разное,
// потому что чинятся они по-разному.
const KodPolosaNeIzmerena = "bandwidth-unmeasured"

// Тело команды не разобралось. Это про ФОРМУ запроса, а не про его смысл:
// «протокол разных версий» отправило бы человека переустанавливать исправную
// программу.
const KodTeloNegodno = "request-invalid"

// Архив обновления не принят: нет .sha256, хеш не совпал, путь наружу, нет службы.
const KodArhivNegoden = "update-archive-invalid"

// Откат обновления сам не удался: прежняя служба тоже молчит.
const KodOtkatNeUdalsya = "update-rollback-failed"

// Служба сломалась внутри: команда уронила панику, перехват ответил кадром.
//
// Заведён 04.09.2026 долгом полосы Г. До него на панику отвечал
// KodProtocolMismatch, потому что строки «служба сломалась внутри» в §9.1 не
// было. Экран на тот код говорит «служба и программа разных версий, обнови
// программу», и человек шёл переустанавливать исправную программу вместо того,
// чтобы повторить команду.
//
// НЕ путать с protocol-mismatch: тот означает «половинки разных версий» и живёт
// в полутора десятках законных точек (тело команды не разбирается, первый кадр
// не hello, версия протокола не та). Здесь версии сошлись, сломались мы.
const KodVnutrennyayaOshibka = "internal-error"
