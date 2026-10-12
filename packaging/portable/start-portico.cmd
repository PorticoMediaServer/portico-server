@echo off
setlocal
if not defined PORTICO_WEB_DIR set "PORTICO_WEB_DIR=%~dp0web"
if not defined PORTICO_STATE_DIR set "PORTICO_STATE_DIR=%~dp0state"
"%~dp0portico-server.exe" %*
exit /b %errorlevel%
