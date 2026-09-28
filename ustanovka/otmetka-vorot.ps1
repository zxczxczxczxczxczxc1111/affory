if ($PSVersionTable.PSVersion.Major -lt 6) { throw 'Запускать через pwsh' }
# Отметка ворот (Б2 аудита 1.6.1). Дот-сорсится из affory-stend\vorota.ps1 и
# из sobrat-reliz.ps1, своей работы не делает.
#
# Ворота и сборка жили порознь: выпуск собирался из любого дерева, в том числе
# из того, на котором ворота не гонялись или были красными. Теперь ворота при
# успехе пишут отметку с коммитом, а сборка без отметки на HEAD не идёт.
#
# Отметка лежит внутри каталога .git: она про этот клон, в историю не попадает
# и не меняет HEAD, на который ссылается.
#
# Поверх отмеченного коммита допускаются только служебные правки выпуска.
# Порядок выпуска такой: ворота, коммит VERSIYA, сборка, коммит
# YADRO-VYPUSKA.json (его пишет сама сборка), тег, пересборка. Требовать
# отметку ровно на HEAD значило бы гонять ворота трижды за выпуск ради двух
# файлов, которые кода не меняют.

$script:SluzhebnyeFaylyVypuska = @('VERSIYA', 'ustanovka/YADRO-VYPUSKA.json')

function Put-OtmetkiVorot([string]$Koren) {
    $put = & git -C $Koren rev-parse --git-path affory-vorota.json
    if ($LASTEXITCODE -ne 0 -or -not $put) { throw "не найден каталог git в $Koren" }
    if (-not [IO.Path]::IsPathRooted($put)) { $put = Join-Path $Koren $put }
    return $put
}

# Незакоммиченное в дереве, кроме служебных файлов выпуска. Неотслеживаемые
# файлы тоже считаются: новый .go вне истории попал бы в сборку.
function Lishnee-VDereve([string]$Koren) {
    $stroki = & git -C $Koren status --porcelain --untracked-files=all
    if ($LASTEXITCODE -ne 0) { throw 'git status не отработал' }
    @($stroki | Where-Object { $_ } | ForEach-Object { $_.Substring(3).Trim('"') } |
        Where-Object { $script:SluzhebnyeFaylyVypuska -notcontains $_ })
}

function Zapisat-OtmetkuVorot([string]$Koren) {
    $lishnee = Lishnee-VDereve $Koren
    if ($lishnee.Count -gt 0) {
        Write-Host ("отметка ворот НЕ поставлена: в дереве незакоммиченное ({0})" -f ($lishnee -join ', ')) -ForegroundColor Yellow
        return
    }
    $kommit = (& git -C $Koren rev-parse HEAD).Trim()
    if ($LASTEXITCODE -ne 0) { throw 'git rev-parse HEAD не отработал' }
    $otmetka = [ordered]@{ kommit = $kommit; vremya = (Get-Date).ToString('o') } | ConvertTo-Json
    [IO.File]::WriteAllText((Put-OtmetkiVorot $Koren), $otmetka, [Text.UTF8Encoding]::new($false))
    Write-Host "отметка ворот поставлена на $($kommit.Substring(0, 7))" -ForegroundColor DarkGray
}

# Бросает, если собирать нельзя. Причину называет целиком: сборку остановили,
# и человек должен понять, что сделать, а не гадать.
function Proverit-OtmetkuVorot([string]$Koren) {
    $put = Put-OtmetkiVorot $Koren
    if (-not (Test-Path $put)) {
        throw "ворота на этом клоне не пройдены: сначала affory-stend\vorota.ps1"
    }
    $otmetka = Get-Content $put -Raw -Encoding UTF8 | ConvertFrom-Json
    $head = (& git -C $Koren rev-parse HEAD).Trim()
    & git -C $Koren merge-base --is-ancestor $otmetka.kommit $head
    if ($LASTEXITCODE -ne 0) {
        throw "отметка ворот на $($otmetka.kommit.Substring(0, 7)), а HEAD $($head.Substring(0, 7)) от неё не происходит: прогнать ворота заново"
    }
    $izmeneno = @(& git -C $Koren diff --name-only "$($otmetka.kommit)..$head" |
        Where-Object { $_ -and $script:SluzhebnyeFaylyVypuska -notcontains $_ })
    if ($izmeneno.Count -gt 0) {
        throw ("после ворот изменён код ({0}): прогнать ворота заново" -f ($izmeneno -join ', '))
    }
    $lishnee = Lishnee-VDereve $Koren
    if ($lishnee.Count -gt 0) {
        throw ("в дереве незакоммиченное ({0}): выпуск собирается только из истории" -f ($lishnee -join ', '))
    }
    Write-Host "ворота пройдены на $($otmetka.kommit.Substring(0, 7)) ($($otmetka.vremya))" -ForegroundColor DarkGray
}
