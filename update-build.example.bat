@echo off
setlocal
set "DEST=C:\path\to\edge-remixer"
set "EXE=%DEST%\edge-remixer.exe"
set "REPO=%~dp0"

echo [1/3] Stopping running instance...
taskkill /IM edge-remixer.exe /F >nul 2>&1

echo [2/3] Building...
set GOOS=windows
set GOARCH=amd64
go build -o "%EXE%" "%REPO%main.go"
if errorlevel 1 (
  echo Build failed.
  exit /b 1
)

echo [3/3] Relaunching in background...
cd /d "%DEST%"
wscript.exe "%REPO%windows-utils\invisible.vbs"
echo Done.
