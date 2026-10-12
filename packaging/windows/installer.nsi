Unicode true
!include "StrFunc.nsh"
${StrRep}
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!define PRODUCT_NAME "Portico Media Server"
!define PRODUCT_PUBLISHER "Justin Ehler"
!define PRODUCT_WEB_SITE "https://getportico.tv"
!define TASK_NAME "Portico Media Server"

Name "${PRODUCT_NAME}"
OutFile "${OUTPUT_FILE}"
InstallDir "$PROGRAMFILES64\Portico Media Server"
InstallDirRegKey HKLM "Software\Portico Media Server" "InstallDir"

Page directory
Page instfiles
UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  nsExec::ExecToLog 'schtasks.exe /End /TN "${TASK_NAME}"'
  SetOutPath "$INSTDIR"
  File /r "${STAGE_DIR}\*"
  File "${__FILEDIR__}\startup-task.xml"
  WriteRegStr HKLM "Software\Portico Media Server" "InstallDir" "$INSTDIR"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "DisplayName" "${PRODUCT_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "Publisher" "${PRODUCT_PUBLISHER}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "URLInfoAbout" "${PRODUCT_WEB_SITE}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  ReadEnvStr $0 "ProgramData"
  StrCmp $0 "" 0 +2
  StrCpy $0 "C:\ProgramData"
  StrCpy $0 "$0\Portico Media Server"
  CreateDirectory "$0"
  ; The server runs at startup as a system task; FFmpeg is found beside it in third_party\ffmpeg\bin.
  FileOpen $1 "$INSTDIR\start-portico.cmd" w
  FileWrite $1 '@echo off$\r$\n'
  FileWrite $1 'set "PORTICO_BIND=0.0.0.0:32500"$\r$\n'
  FileWrite $1 'set "PORTICO_STATE_DIR=$0"$\r$\n'
  FileWrite $1 'set "PORTICO_WEB_DIR=$INSTDIR\web"$\r$\n'
  FileWrite $1 'set "PORTICO_LOG_STDERR=off"$\r$\n'
  FileWrite $1 '"$INSTDIR\portico-server.exe" > "$0\server.log" 2>&1$\r$\n'
  FileClose $1
  ; Register XML directly: machine PowerShell script policies cannot block it.
  ; Escape the only XML metacharacter allowed in a Windows installation path.
  InitPluginsDir
  ${StrRep} $3 "$INSTDIR\start-portico.cmd" "&" "&amp;"
  FileOpen $1 "$INSTDIR\startup-task.xml" r
  FileOpen $2 "$PLUGINSDIR\startup-task.xml" w
  FileRead $1 $4
  ${StrRep} $4 $4 "UTF-8" "UTF-16"
  FileWriteUTF16LE /BOM $2 "$4"
task_xml_line:
  ClearErrors
  FileRead $1 $4
  IfErrors task_xml_done
  ${StrRep} $4 $4 "__PORTICO_LAUNCHER__" "$3"
  FileWriteUTF16LE $2 "$4"
  Goto task_xml_line
task_xml_done:
  FileClose $1
  FileClose $2
  nsExec::ExecToLog 'schtasks.exe /Create /TN "${TASK_NAME}" /XML "$PLUGINSDIR\startup-task.xml" /F'
  Pop $2
  StrCmp $2 "0" task_registered
  ; Keep the previous task definition if registration failed during an upgrade.
  nsExec::ExecToLog 'schtasks.exe /Run /TN "${TASK_NAME}"'
  MessageBox MB_ICONSTOP "Portico could not register its startup task." /SD IDOK
  Abort
task_registered:
  nsExec::ExecToLog 'schtasks.exe /Run /TN "${TASK_NAME}"'
  CreateDirectory "$SMPROGRAMS\Portico Media Server"
  WriteINIStr "$SMPROGRAMS\Portico Media Server\Portico Media Server.url" "InternetShortcut" "URL" "http://127.0.0.1:32500/"
SectionEnd

Section "Uninstall"
  nsExec::ExecToLog 'schtasks.exe /End /TN "${TASK_NAME}"'
  nsExec::ExecToLog 'schtasks.exe /Delete /TN "${TASK_NAME}" /F'
  RMDir /r "$SMPROGRAMS\Portico Media Server"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Portico Media Server"
  DeleteRegKey HKLM "Software\Portico Media Server"
  RMDir /r "$INSTDIR"
SectionEnd
