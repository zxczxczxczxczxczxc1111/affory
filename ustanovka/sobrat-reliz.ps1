# Сборка выпуска: бинари, установщик NSIS, архив обновления с .sha256.
#
# Выпуск состоит из трёх вещей:
#   * Affory-<версия>-setup.exe: установщик (ustanovka\affory.nsi);
#   * affory-<версия>.zip плюс affory-<версия>.zip.sha256: архив для команды
#     installUpdate: четыре файла верхнего уровня, ядро в том числе;
#   * sing-box-<версия ядра>.zip не собирается: ядро это своя сборка апстримного
#     sing-box (ustanovka\sobrat-yadro.ps1), оно кладётся в установщик как есть.
#
# Ничего секретного в выпуск не попадает: ни адресов, ни ключей, ни подписки.
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Versiya,
    [string]$Yadro = (Join-Path $PSScriptRoot 'yadro\sing-box.exe'),
    [string]$Makensis = 'C:\Program Files (x86)\NSIS\makensis.exe',
    # Отпечаток сертификата подписи. Без него выпуск собирается НЕПОДПИСАННЫМ
    # и громко об этом говорит: молчаливая сборка без подписи это ровно тот
    # случай, когда о ней забывают.
    [string]$Otpechatok = '',
    [switch]$BezFronta
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
if ($Versiya -notmatch '^\d+\.\d+\.\d+$') { throw "версия $Versiya не вида X.Y.Z" }
if (-not (Test-Path $Yadro)) { throw "нет ядра $Yadro" }
if (-not (Test-Path $Makensis)) { throw "нет makensis: $Makensis" }

$koren  = Split-Path $PSScriptRoot -Parent
$sborka = Join-Path $PSScriptRoot 'sborka'
$vypusk = Join-Path $PSScriptRoot 'vypusk'
$taskBuildPath = [IO.Path]::GetFullPath($sborka)
$taskExpectedPath = Join-Path ([IO.Path]::GetFullPath($PSScriptRoot)) 'sborka'
if ($taskBuildPath -ne $taskExpectedPath) { throw 'build cleanup escaped the installer workspace' }
if (Test-Path -LiteralPath $taskBuildPath) { Remove-Item -LiteralPath $taskBuildPath -Recurse -Force }
$null = New-Item -ItemType Directory -Path $sborka, $vypusk -Force

Push-Location $koren
try {
    if (-not $BezFronta) {
        Push-Location (Join-Path $koren 'cmd\affory-ui\frontend')
        try {
            # ErrorActionPreference сбрасывается ВОКРУГ npm намеренно.
            #
            # В Windows PowerShell 5.1 редирект `2>&1` на НАТИВНОЙ команде
            # оборачивает каждую строку stderr в ErrorRecord, и под
            # ErrorActionPreference=Stop безобидное «npm notice» про новую
            # версию убивает весь прогон. 04.09.2026 так оборвался полный
            # заход приёмки на первой же строке, ещё до единого судьи.
            # Успех сборки судится по коду выхода, а не по молчанию stderr.
            $prezhnee = $ErrorActionPreference
            $ErrorActionPreference = 'Continue'
            try { & npm run build 2>&1 | Select-Object -Last 2 }
            finally { $ErrorActionPreference = $prezhnee }
            if ($LASTEXITCODE -ne 0) { throw 'фронт не собрался' }
        } finally { Pop-Location }
    }
    # -s -w: без таблицы символов и DWARF, выпуск легче на треть. -trimpath:
    # пути машины сборки в бинарь не попадают.
    $ld = "-s -w -X main.versiyaProgrammy=$Versiya"
    & go build -trimpath -ldflags $ld -o (Join-Path $sborka 'affory-svc.exe') ./cmd/affory-svc
    if ($LASTEXITCODE -ne 0) { throw 'affory-svc не собрался' }
    & go build -trimpath -ldflags $ld -o (Join-Path $sborka 'affory-cli.exe') ./cmd/affory-cli
    if ($LASTEXITCODE -ne 0) { throw 'affory-cli не собрался' }
    # Иконка и версия в ресурсах affory-ui.exe: go build сам подхватывает
    # rsrc_windows_amd64.syso из каталога пакета. Без него у exe нет иконки
    # ни в проводнике, ни в панели задач (выпуск 0.6.0 ушёл таким).
    $winres = Join-Path $env:USERPROFILE 'go\bin\go-winres.exe'
    if (-not (Test-Path $winres)) { throw "нет ${winres}, поставить: go install github.com/tc-hib/go-winres@latest" }
    Push-Location (Join-Path $koren 'cmd\affory-ui')
    try {
        & $winres make --in winres\winres.json --out rsrc --arch amd64 --file-version $Versiya --product-version $Versiya
        if ($LASTEXITCODE -ne 0) { throw 'ресурсы affory-ui не собрались' }
    } finally { Pop-Location }
    & go build -trimpath -ldflags "$ld -H windowsgui" -o (Join-Path $sborka 'affory-ui.exe') ./cmd/affory-ui
    if ($LASTEXITCODE -ne 0) { throw 'affory-ui не собрался' }
} finally { Pop-Location }
Copy-Item $Yadro (Join-Path $sborka 'sing-box.exe')

# Ядро выпуска обязано быть ОПОЗНАВАЕМЫМ.
#
# Выпуск 1.14.0-lx.1-affory собран с ветки форка, которой больше не существует,
# и вернуться к нему теперь можно только сохранённым бинарём: ни коммита, ни
# способа его узнать не записал никто. Отпечаток кладёт sobrat-yadro.ps1, и
# отсутствие отпечатка ОСТАНАВЛИВАЕТ выпуск: ядро, про которое нельзя сказать,
# из чего оно, уезжает людям ровно один раз, а разбираться приходится годами.
#
# Хеш сверяется с тем ядром, которое едет СЕЙЧАС: отпечаток от прошлой сборки
# рядом со свежим бинарём это враньё, причём убедительное.
$putOtpYadra = "$Yadro.otpechatok.json"
if (-not (Test-Path $putOtpYadra)) {
    throw "рядом с ядром нет отпечатка ($putOtpYadra). Пересобрать: ustanovka\sobrat-yadro.ps1"
}
$otpYadra = Get-Content $putOtpYadra -Raw -Encoding UTF8 | ConvertFrom-Json
$heshYadra = (Get-FileHash $Yadro -Algorithm SHA256).Hash.ToLower()
if ($otpYadra.sha256 -ne $heshYadra) {
    throw ("отпечаток описывает другое ядро (в файле $($otpYadra.sha256), на диске $heshYadra)." +
           ' Пересобрать: ustanovka\sobrat-yadro.ps1')
}
if (-not $otpYadra.kommit) { throw 'в отпечатке ядра нет коммита, воспроизвести сборку нечем' }
# Поле переименовано вместе с уходом с форка: раньше сборка бралась с ветки, теперь
# с тега. Пустое поле в ИСТОРИЧЕСКОЙ записи хуже отсутствующего: оно выглядит как
# ответ. Записи выпусков до 0.8.1 несут vetka, с 0.8.1 несут teg.
if (-not $otpYadra.teg) { throw 'в отпечатке ядра нет тега, запись в историю ушла бы с пустым полем' }

# А это уже В ИСТОРИЮ. Каталог ядра вне git, поэтому единственное место, где
# запись переживёт машину, это отслеживаемый файл рядом со скриптами выпуска.
# Хеш здесь от НЕПОДПИСАННОГО ядра: подпись ставится ниже и файл меняет, а
# воспроизводится сборкой именно неподписанный бинарь.
$zapis = [ordered]@{
    vypusk      = $Versiya
    repozitoriy = $otpYadra.repozitoriy
    teg         = $otpYadra.teg
    kommit      = $otpYadra.kommit
    process_family = $otpYadra.process_family
    versiya     = $otpYadra.versiya
    tegi        = $otpYadra.tegi
    go          = $otpYadra.go
    sha256      = $otpYadra.sha256
    sobrano     = $otpYadra.sobrano
}
$zapis | ConvertTo-Json -Depth 5 | Set-Content -Path (Join-Path $PSScriptRoot 'YADRO-VYPUSKA.json') -Encoding UTF8
Write-Host "ядро выпуска: $($otpYadra.versiya), коммит $($otpYadra.kommit)" -ForegroundColor Cyan

# Подпись ДО упаковки, иначе в установщик уедут подписанные файлы, а в архив
# обновления неподписанные, и после первого же самообновления продукт станет
# подписанным наполовину.
$podpisat = Join-Path $PSScriptRoot 'podpisat.ps1'
$kBinaryam = 'affory-svc.exe', 'affory-cli.exe', 'affory-ui.exe', 'sing-box.exe' |
    ForEach-Object { Join-Path $sborka $_ }
if ($Otpechatok) {
    & $podpisat -Fayly $kBinaryam -Otpechatok $Otpechatok
} else {
    Write-Host 'ВЫПУСК БЕЗ ПОДПИСИ: отпечаток не задан (-Otpechatok)' -ForegroundColor Yellow
}

# Архив обновления: только наши exe, формат тот же, что принимает installUpdate.
$zip = Join-Path $vypusk "affory-$Versiya.zip"
Remove-Item $zip -Force -ErrorAction SilentlyContinue
# Ядро едет В архиве с 0.7.0. До этого оно обновлялось только установщиком,
# то есть машина, обновлявшаяся архивом, годами сидела бы на старом ядре и
# никто бы этого не заметил. Архив от этого тяжелеет, и это честная плата.
Compress-Archive -Path (Join-Path $sborka 'affory-svc.exe'), (Join-Path $sborka 'affory-cli.exe'), (Join-Path $sborka 'affory-ui.exe'), (Join-Path $sborka 'sing-box.exe') -DestinationPath $zip
# Потолок загрузки по сети в службе 64 МБ (predelArhivaSeti). Упереться в него
# молча нельзя: обновление тогда перестанет ставиться, а причина будет в чужом
# файле.
$razmerZip = (Get-Item $zip).Length
if ($razmerZip -gt 60MB) { throw "архив обновления $([math]::Round($razmerZip/1MB,1)) МБ, потолок загрузки в службе 64 МБ: поднимать надо и там" }
$h = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLower()
[IO.File]::WriteAllText("$zip.sha256", "$h  affory-$Versiya.zip`n")

# Установщик.
Push-Location $PSScriptRoot
try {
    $nsisKlyuchi = @("/V2", "/DVERSIYA=$Versiya", "/DSBORKA=sborka")
    if ($Otpechatok) {
        $taskSigningShell = (Get-Process -Id $PID).Path
        $nsisKlyuchi += @("/DOTPECHATOK=$Otpechatok", "/DPODPISAT=$podpisat", "/DSIGN_POWERSHELL=$taskSigningShell")
    }
    & $Makensis $nsisKlyuchi affory.nsi 2>&1 | Select-Object -Last 3
    if ($LASTEXITCODE -ne 0) { throw 'makensis завершился с ошибкой' }
} finally { Pop-Location }
$setup = Join-Path $vypusk "Affory-$Versiya-setup.exe"
if (-not (Test-Path $setup)) { throw "установщик не появился: $setup" }
# Подпись меняет байты, значит сумма считается ПОСЛЕ неё. Иначе .sha256 будет
# описывать файл, которого никто никогда не увидит.
if ($Otpechatok) { & $podpisat -Fayly $setup -Otpechatok $Otpechatok }
$hs = (Get-FileHash $setup -Algorithm SHA256).Hash.ToLower()
[IO.File]::WriteAllText("$setup.sha256", "$hs  Affory-$Versiya-setup.exe`n")

Write-Host ''
Write-Host "=== Выпуск $Versiya ===" -ForegroundColor Cyan
Get-ChildItem $vypusk | ForEach-Object { Write-Host ("  {0,-34} {1,10:N0} байт" -f $_.Name, $_.Length) }
