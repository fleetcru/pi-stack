@echo off
REM Installs the external-session bridge extension for Pi.
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy RemoteSigned -File "%~dp0install-exp-external-bridge.ps1" %*
exit /b %errorlevel%
