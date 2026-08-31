Unicode true
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!define PRODUCT_NAME "Portico Media Server"
!define PRODUCT_PUBLISHER "Justin Ehler"
!define PRODUCT_WEB_SITE "https://getportico.tv"

Name "${PRODUCT_NAME}"
OutFile "${OUTPUT_FILE}"
InstallDir "$PROGRAMFILES64\Portico Media Server"
InstallDirRegKey HKLM "Software\Portico Media Server" "InstallDir"

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetOutPath "$INSTDIR"
  File /r "${STAGE_DIR}\*"
  WriteRegStr HKLM "Software\Portico Media Server" "InstallDir" "$INSTDIR"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "Publisher" "${PRODUCT_PUBLISHER}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Run" "Portico Media Server" '"$INSTDIR\portico-desktop.exe"'
  ReadEnvStr $0 "ProgramData"
  StrCmp $0 "" 0 +2
  StrCpy $0 "C:\ProgramData"
  StrCpy $0 "$0\Portico Media Server"
  CreateDirectory "$0"
  nsExec::ExecToLog 'sc.exe stop PorticoMediaServer'
  nsExec::ExecToLog 'sc.exe delete PorticoMediaServer'
  nsExec::ExecToLog 'sc.exe create PorticoMediaServer binPath= "\"$INSTDIR\portico-media-server.exe\"" start= auto DisplayName= "Portico Media Server"'
  nsExec::ExecToLog 'sc.exe description PorticoMediaServer "Portico personal media server"'
  nsExec::ExecToLog 'sc.exe failure PorticoMediaServer reset= 86400 actions= restart/5000/restart/15000/restart/60000'
  nsExec::ExecToLog 'sc.exe failureflag PorticoMediaServer 1'
  nsExec::ExecToLog 'reg.exe add "HKLM\SYSTEM\CurrentControlSet\Services\PorticoMediaServer" /v Environment /t REG_MULTI_SZ /s "|" /d "PORTICO_APP_DATA=$0|PORTICO_WEB_DIST=$INSTDIR\web|PORTICO_FFMPEG_PATH=$INSTDIR\bin\ffmpeg.exe|PORTICO_FFPROBE_PATH=$INSTDIR\bin\ffprobe.exe|PORTICO_ENVIRONMENT=production|PORTICO_HOSTED_API_AUTHORITY=https://api.getportico.tv" /f'
  nsExec::ExecToLog 'sc.exe start PorticoMediaServer'
  ExecShell "open" "$INSTDIR\portico-desktop.exe"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'sc.exe stop PorticoMediaServer'
  nsExec::ExecToLog 'sc.exe delete PorticoMediaServer'
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server"
  DeleteRegKey HKLM "Software\Portico Media Server"
  DeleteRegValue HKLM "Software\Microsoft\Windows\CurrentVersion\Run" "Portico Media Server"
  RMDir /r "$INSTDIR"
SectionEnd
