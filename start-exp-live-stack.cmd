@echo off
REM Starts the full exp live stack (server + web + Pi) without loading a user PowerShell profile.
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0start-exp-live-stack.ps1" %*
exit /b %errorlevel%
