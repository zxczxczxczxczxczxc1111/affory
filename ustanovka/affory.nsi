; Установщик Affory (NSIS 3, Unicode). Собирается ustanovka\sobrat-reliz.ps1,
; который кладёт бинари в ustanovka\sborka\ и передаёт версию через /DVERSIYA.
;
; Установщик нарочно тонкий: всё, что требует знаний о службе (каталог данных
; с разорванным наследованием, отпечатки, ключ Run, аварийный лист, снятие с
; вопросом про ключи), делает сама служба подкомандами install и uninstall
; (§4.4 спеки). Здесь только файлы, реестр «Программы и компоненты» и ярлыки.

Unicode true
!include "MUI2.nsh"
!include "x64.nsh"
!include "LogicLib.nsh"

!ifndef VERSIYA
  !define VERSIYA "0.0.0"
!endif
!ifndef SBORKA
  !define SBORKA "sborka"
!endif

; Деинсталлятор пишет сам установщик директивой WriteUninstaller, до сборки
; его не существует, и подписать его снаружи нечем. Единственный способ это
; !uninstfinalize: NSIS зовёт команду на готовый Uninstall.exe. Путь к скрипту
; приходит через /DPODPISAT: относительный тут считается от каталога NSIS, а не нашего. Отпечаток не
; секрет, в отличие от пароля, поэтому его можно передать аргументом.
!ifdef OTPECHATOK
  !ifndef SIGN_POWERSHELL
    !error "Signing requires the PowerShell executable selected by sobrat-reliz.ps1"
  !endif
  !uninstfinalize '"${SIGN_POWERSHELL}" -NoProfile -ExecutionPolicy Bypass -File "${PODPISAT}" -Fayly "%1" -Otpechatok ${OTPECHATOK}' = 0
!endif

Name "Affory ${VERSIYA}"
OutFile "vypusk\Affory-${VERSIYA}-setup.exe"
InstallDir "$PROGRAMFILES64\Affory"
RequestExecutionLevel admin
SetCompressor /SOLID lzma
BrandingText "Affory ${VERSIYA}"

!define MUI_ICON "..\cmd\affory-ui\ikonki\affory.ico"
!define MUI_UNICON "..\cmd\affory-ui\ikonki\affory.ico"
!define MUI_ABORTWARNING
!define MUI_FINISHPAGE_RUN "$INSTDIR\affory-ui.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Открыть Affory"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Russian"

!define KLYUCH_UDALENIYA "Software\Microsoft\Windows\CurrentVersion\Uninstall\Affory"

Function .onInit
  ${IfNot} ${RunningX64}
    MessageBox MB_ICONSTOP "Affory работает только на 64-разрядной Windows."
    Abort
  ${EndIf}
  SetRegView 64
  ; Ярлыки для всех пользователей: установщик и так требует администратора.
  SetShellVarContext all
FunctionEnd

Section "Affory" SEC_AFFORY
  SectionIn RO
  SetOutPath "$INSTDIR"
  ; Установка поверх стоящей версии. Запущенная служба держит affory-svc.exe
  ; и sing-box.exe, окно держит affory-ui.exe: File на занятом файле падает с
  ; «Невозможно записать» (так упало на хосте 03.09.2026). Сначала остановить,
  ; потом копировать. net stop синхронный и ждёт настоящей остановки; на машине
  ; без службы возвращает ошибку, и это не ошибка установки. Вывод обоих в
  ; журнал не идёт: строка «process not found» на чистой машине читается как
  ; поломка.
  InitPluginsDir
  File /oname=$PLUGINSDIR\affory-svc-setup.exe "${SBORKA}\affory-svc.exe"
  DetailPrint "Остановка Affory и восстановление её сетевых настроек"
  nsExec::ExecToLog '"$PLUGINSDIR\affory-svc-setup.exe" prepare-install'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "Не удалось подготовить Affory к установке (код $0). Установленные файлы оставлены на месте."
    Abort
  ${EndIf}
  nsExec::Exec 'taskkill /IM affory-ui.exe /F'
  Pop $0
  File "${SBORKA}\affory-svc.exe"
  File "${SBORKA}\affory-cli.exe"
  File "${SBORKA}\affory-ui.exe"
  File "${SBORKA}\sing-box.exe"
  ; Условие GPL: рядом с ядром обязан лежать текст лицензии и указание, где
  ; взять его исходники. Ядро это сборка sing-box, а не наш код.
  File "GPL-3.0.txt"
  File "YADRO-ISHODNIKI.txt"
  File /oname=LICENSE.opencck.txt "..\internal\katalog\LICENSE.opencck"
  File /oname=LICENSE.simple-icons.txt "..\cmd\affory-ui\frontend\src\assets\services\LICENSE.simple-icons.txt"
  File /oname=OFL-Manrope.txt "..\cmd\affory-ui\frontend\src\shrifty\OFL-Manrope.txt"

  ; Служба ставит себя сама: каталог данных, отпечатки, ключ Run, аварийный
  ; лист, запуск. Ставится поверх существующей (обновление), режим не снимается.
  DetailPrint "Установка службы AfforySvc"
  nsExec::ExecToLog '"$INSTDIR\affory-svc.exe" install'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "Служба Affory не установилась (код $0). Подробности в C:\ProgramData\Affory\log\sluzhba.log"
    Abort
  ${EndIf}

  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateDirectory "$SMPROGRAMS\Affory"
  CreateShortcut "$SMPROGRAMS\Affory\Affory.lnk" "$INSTDIR\affory-ui.exe"
  CreateShortcut "$SMPROGRAMS\Affory\Удалить Affory.lnk" "$INSTDIR\Uninstall.exe"

  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "DisplayName" "Affory"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "DisplayVersion" "${VERSIYA}"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "DisplayIcon" "$INSTDIR\affory-ui.exe"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKLM "${KLYUCH_UDALENIYA}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKLM "${KLYUCH_UDALENIYA}" "NoModify" 1
  WriteRegDWORD HKLM "${KLYUCH_UDALENIYA}" "NoRepair" 1
SectionEnd

Function un.onInit
  SetRegView 64
  SetShellVarContext all
FunctionEnd

Section "Uninstall"
  nsExec::Exec 'taskkill /IM affory-ui.exe /F'
  Pop $0

  ; Вопрос про ключи задаёт тот, кто снимает, а не служба: в тихом режиме (/S)
  ; ключи остаются, стирание необратимо и по умолчанию не делается.
  StrCpy $1 ""
  ${IfNot} ${Silent}
    MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Стереть ключи и настройки Affory (C:\ProgramData\Affory)?$\r$\nЭто необратимо. «Нет» оставит их для следующей установки." IDNO +2
      StrCpy $1 " --steret-klyuchi"
  ${EndIf}
  DetailPrint "Снятие службы AfforySvc"
  nsExec::ExecToLog '"$INSTDIR\affory-svc.exe" uninstall$1'
  Pop $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP "Не удалось остановить Affory и восстановить сеть (код $0). Файлы оставлены для повторной попытки."
    Abort
  ${EndIf}

  Delete "$INSTDIR\affory-svc.exe"
  Delete "$INSTDIR\affory-cli.exe"
  Delete "$INSTDIR\affory-ui.exe"
  Delete "$INSTDIR\sing-box.exe"
  Delete "$INSTDIR\GPL-3.0.txt"
  Delete "$INSTDIR\YADRO-ISHODNIKI.txt"
  Delete "$INSTDIR\LICENSE.opencck.txt"
  Delete "$INSTDIR\LICENSE.simple-icons.txt"
  Delete "$INSTDIR\OFL-Manrope.txt"
  Delete "$INSTDIR\ESLI-NET-INTERNETA.txt"
  Delete "$INSTDIR\*.ubrat"
  RMDir /r "$INSTDIR\novaya"
  RMDir /r "$INSTDIR\predydushchaya"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\Affory\Affory.lnk"
  Delete "$SMPROGRAMS\Affory\Удалить Affory.lnk"
  RMDir "$SMPROGRAMS\Affory"
  DeleteRegKey HKLM "${KLYUCH_UDALENIYA}"
SectionEnd
