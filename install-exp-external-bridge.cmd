@echo off
REM Installs the external-session bridge extension for Pi.
powershell.exe -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0install-exp-external-bridge.ps1" %*
