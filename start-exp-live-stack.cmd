@echo off
REM Starts the full exp live stack (server + web + Pi) without loading a user PowerShell profile.
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0start-exp-live-stack.ps1" %*
