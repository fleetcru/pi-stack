@echo off
REM Starts the exp pi-server without loading a user PowerShell profile.
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0start-exp-server.ps1" %*
exit /b %errorlevel%
