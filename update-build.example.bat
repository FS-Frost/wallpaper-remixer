@echo off
setlocal
set "DEST=C:\path\to\wallpaper-remixer"
set "EXE=%DEST%\remixer.exe"
set "REPO=%~dp0"

echo [1/3] Stopping running instance...
if exist "%EXE%" "%EXE%" -stop
taskkill /IM remixer.exe /F >nul 2>&1

echo [2/3] Building...
set GOOS=windows
set GOARCH=amd64
pushd "%REPO%"
go build -ldflags "-X 'main.sourceDir=%REPO%'" -o "%EXE%" .
set "BUILD_ERROR=%errorlevel%"
popd
if not "%BUILD_ERROR%"=="0" (
  echo Build failed.
  exit /b 1
)

echo [3/3] Relaunching in background...
cd /d "%DEST%"
wscript.exe "%REPO%windows-utils\invisible.vbs" "%EXE%"
echo Done.
