# Сборка ядра.
#
# С 06.09.2026 ядро АПСТРИМНОЕ, `SagerNet/sing-box`. До этого дня оно бралось из
# форка `Leadaxe/sing-box-lx` ради одного транспорта, xhttp: у апстрима его нет
# и не будет (позиция мейнтейнера). Владелец от xhttp отказался, и вместе с ним
# отпала причина сидеть на чужом форке, чьи регрессии уже стоили выпуска 0.8.0
# (DNS через туннель умирал в lx.2, лечился только к lx.34).
#
# Версия ПРИБИТА к 1.14.0-rc.5 намеренно, а не взята из стабильной линии.
# Проверено 06.09.2026 сборкой: 1.13.21 отвергает `dns_mode` у входящего tun
# (`json: unknown field`), а это ровно то поле, которым туннель перехватывает
# DNS. Стабильная линия обошлась бы возвратом к утечке DNS, то есть дороже, чем
# rc той же минорной версии, с которой мы и съезжаем.
#
# НАБОР ТЕГОВ УРЕЗАН НАМЕРЕННО, и это стоило 60 МБ памяти. Полный набор форка
# (LX_TAGS) тянет WireGuard, AmneziaWG, OpenVPN, OpenConnect, naive, DHCP, lxd и
# командный слой для их мобильного клиента. Нам не нужно ничего из этого.
# Замерено в госте: полный набор дал 190.2 МБ частных страниц на два процесса,
# урезанный 130.1 МБ, бинарь 53.2 МБ против 45.4 МБ.
#
# Что оставлено и почему каждый:
#   with_gvisor    — стек TUN, у нас stack: gvisor
#   with_quic      — транспорт hy2 (hysteria2) из списка поддерживаемых
#   with_utls      — отпечаток TLS для reality
#   with_clash_api — им работает проба живости туннеля, без него подъём слеп
#
# Проверка достаточности набора это не рассуждение: двадцать профилей генератора
# прогоняются через `sing-box check` этой сборкой (тест Invariant8), и там есть
# и hy2, и ss, и ws, и grpc, и reality-tcp, и anytls, и tuic. Плюс корпус из 21
# живой формы ссылки, где каждая либо отвергается разбором с названной причиной,
# либо собирается и принимается ЯДРОМ.
[CmdletBinding()]
param(
    [string]$Rabochiy = (Join-Path $env:TEMP 'affory-yadro'),
    # Пусто значит `ustanovka\yadro\sing-box.exe` рядом со скриптом. Считается
    # НЕ здесь: в блоке param под `powershell -File` переменная $PSScriptRoot
    # пуста, и умолчание валило скрипт на Join-Path. Не замечали до 06.09.2026,
    # потому что все прогоны шли с явным -Kuda и умолчание не вычислялось ни разу.
    [string]$Kuda     = '',
    # ТЕГ апстрима, а не ветка. Ветка движется, и собранное по ней ядро
    # невоспроизводимо: выпуск 1.14.0-lx.1-affory уже потерян ровно так.
    # Список смотреть `git ls-remote --tags --refs <repo>`.
    [string]$Teg      = 'v1.14.0-rc.5',
    # Дополнительные теги сборки, через запятую. Пусто значит рабочий набор и
    # ничего больше: умолчание здесь это то, что уезжает в выпуск, и менять его
    # ради одного опыта нельзя. Заведено 05.09.2026 ради задачи 7, где нужна
    # ВТОРАЯ сборка со стандартным gRPC для сравнения с нашей.
    [string]$LishnieTegi = '',
    # Точный коммит форка. Пусто значит вершину ветки, и тогда собранное
    # ЗАПИСЫВАЕТСЯ в отпечаток рядом с бинарём: без этого «то же самое ядро»
    # невоспроизводимо в принципе. Выпуск 1.14.0-lx.1-affory собран с ветки,
    # которой больше нет, и вернуться к нему теперь можно только сохранённым
    # бинарём. Второй раз так попадать незачем.
    [string]$Kommit = '',
    # Версия тулчейна Go, например `go1.26.7`. Пусто значит «чем машина богата»,
    # и тогда отпечаток только ЗАПИСЫВАЕТ версию задним числом, а повторить по
    # нему сборку нельзя: выпуск 0.8.2 собран go1.26.7, на машине давно 1.27.0,
    # и байты расходятся не из-за кода. Заданная версия уезжает в GOTOOLCHAIN,
    # и Go подтягивает её сам, ставить руками ничего не нужно.
    [string]$Go = '',
    # Собрать РОВНО то, что уехало в выпуск: коммит, теги и тулчейн берутся из
    # ustanovka\YADRO-VYPUSKA.json, то есть из отпечатка, а не из умолчаний
    # скрипта. Умолчания движутся вместе с проектом, отпечаток нет.
    [switch]$KakVypusk,
    [switch]$Zanovo
)

$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8

# Каталог скрипта берётся из MyInvocation: он заполнен и под `-File`, и под
# точечным вызовом, в отличие от $PSScriptRoot в блоке param.
$svoyKatalog = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not $Kuda) { $Kuda = Join-Path $svoyKatalog 'yadro\sing-box.exe' }

# Выпуск восстанавливается из своего отпечатка, а не из умолчаний скрипта.
if ($KakVypusk) {
    $faylVypuska = Join-Path $svoyKatalog 'YADRO-VYPUSKA.json'
    if (-not (Test-Path $faylVypuska)) { throw "нет отпечатка выпуска: $faylVypuska" }
    $vypusk = Get-Content $faylVypuska -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $Kommit -and $vypusk.kommit) { $Kommit = $vypusk.kommit }
    if (-not $Go     -and $vypusk.go)     {
        # В отпечатке лежит вся строка `go version go1.26.7 windows/amd64`,
        # GOTOOLCHAIN понимает только середину.
        $m = [regex]::Match([string]$vypusk.go, '\bgo\d+\.\d+(\.\d+)?\b')
        if ($m.Success) { $Go = $m.Value }
    }
    Write-Host "как выпуск $($vypusk.vypusk): коммит $Kommit, тулчейн $Go" -ForegroundColor Cyan
}

# Объявлена ДО первого вызова намеренно: PowerShell разбирает файл сверху вниз,
# и вызов функции выше её объявления это CommandNotFoundException, а не
# предупреждение. Блок GOTOOLCHAIN ниже зовёт Nativno, и пока пин тулчейна был
# сломан, до этого вызова просто не доходило: дефект лежал тихо с 07.09.2026.
function Nativno {
    param([Parameter(Mandatory)][scriptblock]$Chto, [Parameter(Mandatory)][string]$Chinit)
    $prezhnee = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { & $Chto } finally { $ErrorActionPreference = $prezhnee }
    if ($LASTEXITCODE -ne 0) { throw $Chinit }
}

# GOTOOLCHAIN ставится ДО первой команды go, иначе первой же и промахнёмся.
#
# Проверка обязательна и не для красоты: при выключенном GOTOOLCHAIN
# (`GOTOOLCHAIN=local` в окружении машины) Go молча возьмёт свой тулчейн, соберёт
# и отчитается успехом, а отпечаток запишет уже ДРУГУЮ версию. Молчаливое
# расхождение в файле, весь смысл которого воспроизводимость, хуже отказа.
if ($Go) {
    $env:GOTOOLCHAIN = $Go
    $chto = "$(Nativno { & go version } 'go version не ответила')"
    if ($chto -notmatch [regex]::Escape($Go)) {
        throw "заказан тулчейн $Go, а go отвечает '$chto'. Скорее всего GOTOOLCHAIN выключен в окружении машины."
    }
    Write-Host "тулчейн: $chto" -ForegroundColor DarkGray
}

$TEGI = 'with_gvisor,with_quic,with_utls,with_clash_api'
if ($LishnieTegi) { $TEGI = "$TEGI,$LishnieTegi" }
$repo = 'https://github.com/SagerNet/sing-box.git'
$Rabochiy = [IO.Path]::GetFullPath($Rabochiy)
$put = [IO.Path]::GetFullPath((Join-Path $Rabochiy 'sing-box'))
if (-not $put.StartsWith($Rabochiy.TrimEnd('\') + '\', [StringComparison]::OrdinalIgnoreCase) -or (Split-Path $put -Leaf) -ne 'sing-box') {
    throw 'Unsafe core workspace path'
}

# Сборка по точному коммиту всегда идёт с чистого места: доставать нужный
# коммит в каталог, где уже лежит другой, значит собрать смесь и не заметить.
if (($Zanovo -or $Kommit) -and (Test-Path -LiteralPath $put)) { Remove-Item -LiteralPath $put -Recurse -Force }
New-Item -ItemType Directory -Force -Path $Rabochiy | Out-Null

if (-not (Test-Path $put)) {
    if ($Kommit) {
        # `git clone` по SHA не умеет, а мелкий клон ветки нужного коммита может
        # уже не содержать. Поэтому fetch именно этого объекта: GitHub такое
        # разрешает, и история при этом остаётся в одну ревизию.
        Write-Host "клон $repo по коммиту $Kommit" -ForegroundColor Cyan
        New-Item -ItemType Directory -Force -Path $put | Out-Null
        Push-Location $put
        try {
            Nativno { & git init -q } 'git init не удался'
            Nativno { & git remote add origin $repo } 'remote не добавлен'
            Nativno { & git fetch --depth 1 origin $Kommit } "коммит $Kommit не выкачался"
            Nativno { & git checkout -q FETCH_HEAD } "переход на $Kommit не удался"
        } finally { Pop-Location }
    } else {
        Write-Host "клон $repo, тег $Teg" -ForegroundColor Cyan
        Nativno { & git clone --depth 1 --branch $Teg $repo $put } 'клон не удался'
    }
}

Push-Location $put
try {
    # Сабмодули ставятся ПОИМЁННО и БЕЗ --depth. Обе оговорки куплены временем:
    # `git submodule update --init` целиком падает на Windows с "Filename too
    # long" внутри clients/apple, который к сборке отношения не имеет; а с
    # --depth 1 git печатает "checked out <sha>", возвращает ноль и оставляет
    # каталог ПУСТЫМ, то есть врёт успехом.
    #
    # Список берётся ИЗ go.mod, а не пишется руками. Руками написанный список
    # 05.09.2026 назвал три сабмодуля, из которых существовал один: форк перевёл
    # gvisor и sing-tun в обычные модули, а скрипт продолжал требовать каталоги,
    # и сборка падала на `pathspec did not match`. Сборке нужны ровно те
    # каталоги, на которые в go.mod стоит replace, и никакие другие.
    $nuzhny = @(Select-String -Path (Join-Path $put 'go.mod') -Pattern '=>\s*\./(submodules/[\w.-]+)' |
                ForEach-Object { $_.Matches[0].Groups[1].Value } | Sort-Object -Unique)
    # Пустой список это НОРМА для апстрима: сабмодули были свойством форка.
    # Раньше здесь стоял throw, и на апстриме он сработал бы ложно.
    if ($nuzhny) { Write-Host "сабмодули по go.mod: $($nuzhny -join ', ')" }
    else { Write-Host 'replace на ./submodules нет, это апстрим' }
    foreach ($m in $nuzhny) {
        Nativno { & git submodule update --init --force $m } "сабмодуль $m не поставлен"
        if (-not (Test-Path (Join-Path $put "$m/go.mod"))) {
            throw "сабмодуль $m пуст, хотя git отчитался успехом"
        }
    }

    # Версия берётся из ТЕГА, под которым лежит дерево. Мелкий клон по тегу этот
    # тег несёт, поэтому describe тут честен; при сборке по -Kommit тега может не
    # быть, и тогда версию называет сам параметр -Teg.
    $opisanie = "$(& git describe --tags --exact-match 2>$null)".Trim()
    if (-not $opisanie) { $opisanie = $Teg }
    if ($opisanie -notmatch '^v([0-9]+\.[0-9]+\.[0-9]+[0-9A-Za-z.-]*)$') {
        throw "версия не выводится: describe дал '$opisanie', а ждали vX.Y.Z"
    }
    $ver = "$($Matches[1])-affory-family.1"
    $sobrannyy = "$(Nativno { & git rev-parse HEAD } 'коммит форка не прочитался')".Trim()
    if (-not $sobrannyy) { throw 'коммит форка пуст: отпечаток был бы враньём' }
    # Заказанный коммит сверяется с тем, что реально лежит в дереве.
    #
    # Без сверки обещание -Kommit держится на том, что уборка выше отработала, а
    # она может и не отработать: каталог, оставшийся от оборванного прогона,
    # сборку не остановит, и на выходе будет ядро из ДРУГОЙ ревизии с отпечатком,
    # утверждающим обратное. Отпечаток, которому нельзя верить, хуже его
    # отсутствия: по нему перестают проверять.
    if ($Kommit -and $sobrannyy -ne $Kommit) {
        throw "заказан коммит $Kommit, а в дереве $sobrannyy. Чистое место: -Zanovo"
    }
    # This is a pinned extension, not a hopeful patch lottery against tomorrow's upstream.
    if ($sobrannyy -ne 'c881f561e9304ac7a2662d27d80394b3fec1b96c') { throw 'Process-family patch requires the pinned upstream revision' }
    $patch = Join-Path $svoyKatalog 'patches\process-family.patch'
    & git apply --check $patch 2>$null
    if ($LASTEXITCODE -eq 0) {
        Nativno { & git apply $patch } 'process-family patch failed'
    } else {
        Nativno { & git apply --reverse --check $patch } 'core contains incompatible local changes; use a fresh workspace'
    }
    $familySource = Join-Path $svoyKatalog 'processfamily'
    $familyTarget = Join-Path $put 'common\afforyprocess'
    New-Item -ItemType Directory -Force -Path $familyTarget | Out-Null
    Get-ChildItem -LiteralPath $familySource -Filter '*.go' | ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination $familyTarget -Force }
    $changedSources = @(Select-String -LiteralPath $patch -Pattern '^\+\+\+ b/(.+\.go)$' | ForEach-Object { Join-Path $put $_.Matches[0].Groups[1].Value })
    $toolchainRoot = "$(Nativno { & go env GOROOT } 'Go root not found')".Trim()
    $formatter = Join-Path $toolchainRoot 'bin\gofmt.exe'
    Nativno { & $formatter -w $changedSources } 'core source normalization failed'
    $extensionHashes = [ordered]@{ patch = (Get-FileHash -LiteralPath $patch -Algorithm SHA256).Hash.ToLower() }
    Get-ChildItem -LiteralPath $familySource -Filter '*.go' | Sort-Object Name | ForEach-Object {
        $extensionHashes[$_.Name] = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLower()
    }
    Write-Host "сборка $ver, коммит $sobrannyy, теги: $TEGI" -ForegroundColor Cyan

    $env:CGO_ENABLED = '0'
    # -buildvcs=false обязателен для воспроизводимости, и это не суеверие.
    #
    # Go штампует в бинарь версию главного модуля, выведенную из git. Клон ветки
    # везёт теги и даёт `v1.14.0-lx.16`, а выкачка одного коммита тегов не несёт
    # и даёт псевдоверсию `v0.0.0-<дата>-<sha>`. Замерено 05.09.2026: две сборки
    # ОДНОГО коммита разошлись на 1 090 356 байт ровно из-за этой строки.
    # Версия самого sing-box приходит через ldflags и от штампа не зависит.
    Nativno {
        & go build -trimpath -buildvcs=false -tags $TEGI `
            -ldflags "-X 'github.com/sagernet/sing-box/constant.Version=$ver' -s -w -buildid=" `
            -o (Join-Path $put 'sing-box.exe') ./cmd/sing-box
    } 'сборка не удалась'
} finally { Pop-Location }

$novoe = Join-Path $put 'sing-box.exe'
Nativno { & $novoe version } 'собранное ядро не запускается' | Select-Object -First 1

# Каталог назначения заводится сам. Умолчание лежит в `ustanovka\yadro`, который
# уже есть, поэтому дыру видно только когда ядро кладут в СВОЁ место: бисекция
# по тегам форка 05.09.2026 потеряла на этом целую сборку, потому что падение
# случилось ПОСЛЕ компиляции, на копировании.
$katalogKuda = Split-Path $Kuda -Parent
if ($katalogKuda -and -not (Test-Path $katalogKuda)) {
    New-Item -ItemType Directory -Force -Path $katalogKuda | Out-Null
}

# Прежнее ядро не затирается молча: если новая сборка окажется хуже, откатывать
# будет нечем.
if (Test-Path $Kuda) {
    $bak = "$Kuda.bak"
    Copy-Item $Kuda $bak -Force
    Write-Host "прежнее ядро сохранено: $bak"
}
Copy-Item $novoe $Kuda -Force
Write-Host "ядро положено: $Kuda" -ForegroundColor Green

# Отпечаток кладётся РЯДОМ с бинарём и описывает, из чего он получен.
#
# Без него «то же самое ядро» не факт, а надежда: ветка форка движется, теги
# мелкого клона недоступны, а версия в бинаре придумана этим же скриптом.
# Выпуск 1.14.0-lx.1-affory уже невоспроизводим по этой причине.
#
# Каталог ядра вне git, поэтому запись сюда это память МАШИНЫ. В историю
# отпечаток выпущенного ядра кладёт sobrat-reliz.ps1.
# Ветка пишется ТОЛЬКО когда собирали её вершину.
#
# При сборке по коммиту ветка не при чём: 06.09.2026 отпечаток ядра lx.34
# утверждал `vetka: lx-1.14`, хотя на этой ветке такого коммита нет вовсе, её
# вершина стоит на lx.16. В файле, весь смысл которого происхождение, это ложь,
# по которой потом будут искать не там.
$otkuda = if ($Kommit) { "коммит запрошен напрямую, тег не при чём" } else { $Teg }

$otpechatok = [ordered]@{
    repozitoriy = $repo
    teg         = $otkuda
    kommit      = $sobrannyy
    process_family = $extensionHashes
    versiya     = $ver
    tegi        = $TEGI
    go          = "$(Nativno { & go version } 'go version не ответила')"
    sha256      = (Get-FileHash $Kuda -Algorithm SHA256).Hash.ToLower()
    bayt        = (Get-Item $Kuda).Length
    sobrano     = (Get-Date).ToString('o')
}
$putOtpechatka = "$Kuda.otpechatok.json"
$otpechatok | ConvertTo-Json -Depth 5 | Set-Content -Path $putOtpechatka -Encoding UTF8
Write-Host "отпечаток: $putOtpechatka"
Write-Host "повторить эту же сборку: .\sobrat-yadro.ps1 -Kommit $sobrannyy" -ForegroundColor Cyan
Write-Host ''
Write-Host 'Дальше обязательно: go test ./... с AFFORY_SINGBOX=<путь к ядру>, там sing-box check восьми профилей.'
