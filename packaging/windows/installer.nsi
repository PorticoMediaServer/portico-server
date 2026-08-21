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
  CreateDirectory "$PROGRAMDATA\Portico Media Server"
  nsExec::ExecToLog 'sc.exe stop PorticoMediaServer'
  nsExec::ExecToLog 'sc.exe delete PorticoMediaServer'
  nsExec::ExecToLog 'sc.exe create PorticoMediaServer binPath= "\"$INSTDIR\portico-media-server.exe\"" start= auto DisplayName= "Portico Media Server"'
  nsExec::ExecToLog 'sc.exe description PorticoMediaServer "Portico personal media server"'
  WriteRegMultiStr HKLM "SYSTEM\CurrentControlSet\Services\PorticoMediaServer" "Environment" "PORTICO_APP_DATA=$PROGRAMDATA\Portico Media Server$\0PORTICO_WEB_DIST=$INSTDIR\web$\0PORTICO_FFMPEG_PATH=$INSTDIR\bin\ffmpeg.exe$\0PORTICO_FFPROBE_PATH=$INSTDIR\bin\ffprobe.exe$\0"
  nsExec::ExecToLog 'sc.exe start PorticoMediaServer'
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'sc.exe stop PorticoMediaServer'
  nsExec::ExecToLog 'sc.exe delete PorticoMediaServer'
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server"
  DeleteRegKey HKLM "Software\Portico Media Server"
  RMDir /r "$INSTDIR"
SectionEnd
