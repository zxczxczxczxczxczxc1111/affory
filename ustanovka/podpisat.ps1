# Подпись выпуска самоподписанным сертификатом.
#
# Зачем это вообще, если SmartScreen всё равно ругается. Затем, что подпись
# отвечает на другой вопрос. SmartScreen отвечает «знаем ли мы этого издателя»
# и для самоподписанного будет отвечать «нет» вечно. Подпись отвечает «те ли
# это байты, что вышли со сборки», и вот на это она отвечает честно: любой
# изменённый байт ломает её так, что видно из проводника.
#
# Сертификат берётся ИЗ ХРАНИЛИЩА по отпечатку, а не из pfx с паролем в
# аргументе. Это не педантизм: командную строку чужого процесса на Windows
# читает кто угодно, и способа скормить signtool пароль иначе, чем через
# аргумент, не существует. Из хранилища пароля нет вовсе, значит нечему течь.
#
# Создание сертификата (один раз, отпечаток записать):
#   New-SelfSignedCertificate -Type CodeSigningCert -Subject 'CN=Affory' `
#       -KeyAlgorithm RSA -KeyLength 4096 -HashAlgorithm SHA256 `
#       -KeyExportPolicy Exportable -KeyUsage DigitalSignature `
#       -TextExtension @('2.5.29.37={text}1.3.6.1.5.5.7.3.3') `
#       -CertStoreLocation 'Cert:\CurrentUser\My' -NotAfter (Get-Date).AddYears(10)
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string[]]$Fayly,
    [Parameter(Mandatory)][string]$Otpechatok,
    # Метка времени обязательна. Без неё подпись умирает в день истечения
    # сертификата: проверяющему неоткуда узнать, что подписано было раньше.
    [string]$Metka = 'http://timestamp.digicert.com'
)
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8

function NaytiSigntool {
    # Берётся САМАЯ СВЕЖАЯ версия SDK, а не первая попавшаяся: в Windows Kits
    # лежат каталоги нескольких сборок, и в старых signtool может не быть вовсе.
    $baza = 'C:\Program Files (x86)\Windows Kits\10\bin'
    if (Test-Path $baza) {
        $najden = Get-ChildItem $baza -Directory -ErrorAction SilentlyContinue |
            Sort-Object Name -Descending |
            ForEach-Object { Join-Path $_.FullName 'x64\signtool.exe' } |
            Where-Object { Test-Path $_ } |
            Select-Object -First 1
        if ($najden) { return $najden }
    }
    $izPuti = (Get-Command signtool.exe -ErrorAction SilentlyContinue).Source
    if ($izPuti) { return $izPuti }
    # Пусто, а не отказ: без SDK подписывает встроенный командлет, см. ниже.
    return ''
}

$Otpechatok = ($Otpechatok -replace '[^0-9A-Fa-f]', '').ToUpper()
if ($Otpechatok.Length -ne 40) { throw "отпечаток должен быть 40 шестнадцатеричных знаков, получено $($Otpechatok.Length)" }
$cert = Get-ChildItem Cert:\CurrentUser\My | Where-Object { $_.Thumbprint -eq $Otpechatok }
if (-not $cert) { throw "в Cert:\CurrentUser\My нет сертификата с отпечатком $Otpechatok" }
if ($cert.NotAfter -lt (Get-Date)) { throw "сертификат истёк $($cert.NotAfter.ToString('dd.MM.yyyy'))" }

# Запасной путь без Windows SDK, заведён 08.09.2026.
#
# Set-AuthenticodeSignature кладёт ту же самую подпись Authenticode с меткой
# времени и живёт в самой Windows, поэтому потеря SDK перестала быть поводом
# выпускать неподписанное. signtool остаётся первым выбором: он умеет больше
# (двойная подпись, свои цепочки) и у него понятнее диагностика.
$signtool = NaytiSigntool
$chem = if ($signtool) { "signtool $signtool" } else { 'встроенный Set-AuthenticodeSignature, SDK не найден' }
Write-Host "подпись: $($cert.Subject), до $($cert.NotAfter.ToString('dd.MM.yyyy'))" -ForegroundColor Cyan
Write-Host "чем: $chem" -ForegroundColor DarkGray

foreach ($f in $Fayly) {
    if (-not (Test-Path $f)) { throw "нечего подписывать: $f" }
    # 2>&1 на родной программе под PS 5.1 заворачивает stderr в ErrorRecord и
    # валит скрипт при ErrorActionPreference=Stop, даже когда программа
    # отработала как надо. Отсюда локальное послабление.
    if ($signtool) {
        $prezhniy = $ErrorActionPreference
        $ErrorActionPreference = 'Continue'
        $vyvod = (& $signtool sign /sha1 $Otpechatok /fd sha256 /tr $Metka /td sha256 /q $f 2>&1 | Out-String)
        $ErrorActionPreference = $prezhniy
        if ($LASTEXITCODE -ne 0) { throw "подпись не легла на $(Split-Path $f -Leaf): $vyvod" }
    } else {
        $itog = Set-AuthenticodeSignature -FilePath $f -Certificate $cert `
                    -HashAlgorithm SHA256 -TimestampServer $Metka -ErrorAction Stop
        if ($itog.Status -eq 'HashMismatch' -or -not $itog.SignerCertificate) {
            throw "подпись не легла на $(Split-Path $f -Leaf): $($itog.Status) $($itog.StatusMessage)"
        }
    }

    # Проверка своя, а не доверие коду выхода. Самоподписанный корень не в
    # доверенных, поэтому Valid ждать НЕЛЬЗЯ: приедет UnknownError с текстом
    # про недоверенный корень, и это норма. Проверяем то, что действительно
    # обязано быть: подпись есть, она наша, и метка времени проставлена.
    $p = Get-AuthenticodeSignature $f
    if (-not $p.SignerCertificate -or $p.SignerCertificate.Thumbprint -ne $Otpechatok) {
        throw "$(Split-Path $f -Leaf): подписан не тем сертификатом или не подписан вовсе"
    }
    if (-not $p.TimeStamperCertificate) {
        throw "$(Split-Path $f -Leaf): нет метки времени, подпись умрёт вместе с сертификатом"
    }
    Write-Host ("  {0,-24} подписан, метка времени есть" -f (Split-Path $f -Leaf)) -ForegroundColor DarkGray
}
