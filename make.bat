@echo off
REM Helena dev convenience script — Windows .bat equivalent of the Makefile.
REM Usage:   make.bat <target>
REM Targets: run build test vet fmt lint tidy clean
REM Notes:   building cmd\helena needs a C toolchain (TDM-GCC or MSYS2 mingw-w64)
REM          on PATH because Fyne uses cgo + OpenGL.

setlocal EnableExtensions

set "APP=helena.exe"
set "PKG=.\cmd\helena"

if "%~1"=="" goto :run
if /I "%~1"=="run"   goto :run
if /I "%~1"=="build" goto :build
if /I "%~1"=="test"  goto :test
if /I "%~1"=="vet"   goto :vet
if /I "%~1"=="fmt"   goto :fmt
if /I "%~1"=="lint"  goto :lint
if /I "%~1"=="tidy"  goto :tidy
if /I "%~1"=="clean" goto :clean
goto :usage

:run
go run %PKG%
goto :end

:build
if not exist bin mkdir bin
go build -o bin\%APP% %PKG%
goto :end

:test
go test ./...
goto :end

:vet
go vet ./...
goto :end

:fmt
gofmt -w .
goto :end

:lint
golangci-lint run
goto :end

:tidy
go mod tidy
goto :end

:clean
if exist bin  rmdir /S /Q bin
if exist dist rmdir /S /Q dist
goto :end

:usage
echo Usage: %~n0 ^<target^>
echo Targets: run build test vet fmt lint tidy clean
exit /b 1

:end
endlocal & exit /b %ERRORLEVEL%
