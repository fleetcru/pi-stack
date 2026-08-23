@echo off
REM Starts the exp pi-server without loading a user PowerShell profile.
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0start-exp-server.ps1" %*
