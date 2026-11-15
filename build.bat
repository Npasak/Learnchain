@echo off
title LearnChain - Build Single Binary (.exe)
setlocal

rem Ensure current working directory is the script's directory
cd /d "%~dp0"

rem Check if go is in PATH, if not add standard installation path
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    if exist "C:\Program Files\Go\bin\go.exe" (
        set "PATH=%PATH%;C:\Program Files\Go\bin"
    )
)

set CGO_ENABLED=0

echo ========================================================
echo    LearnChain SBT dApp - Builder (Option B: Zero-CGO)
echo ========================================================
echo.

echo [1/2] Updating dependencies (go mod tidy)...
go mod tidy
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Failed to run 'go mod tidy'.
    pause
    exit /b %ERRORLEVEL%
)

if "%APP_VERSION%"=="" set APP_VERSION=v1.0.0-local
for /f "tokens=*" %%i in ('powershell -NoProfile -Command "(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')"') do set BUILD_DATE=%%i

echo [2/2] Compiling single binary executable (LearnChain.exe %APP_VERSION%)...
go build -ldflags="-H=windowsgui -s -w -X elearning-sbt/version.Version=%APP_VERSION% -X elearning-sbt/version.BuildDate=%BUILD_DATE%" -o LearnChain.exe .
if %ERRORLEVEL% NEQ 0 (
    echo [ERROR] Compilation failed.
    pause
    exit /b %ERRORLEVEL%
)

echo.
echo ========================================================
echo [SUCCESS] Binary compiled successfully: LearnChain.exe (%APP_VERSION%)
echo Launching LearnChain.exe ...
echo ========================================================
echo.

LearnChain.exe
