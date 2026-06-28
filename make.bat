@echo off
REM Helena dev convenience script — Windows .bat equivalent of the Makefile.
REM Usage:   make.bat <target>
REM Targets: run build test coverage coverage-html coverage-gate mutation vet fmt lint tidy clean website website-build
REM Notes:   building cmd\helena needs a C toolchain (TDM-GCC or MSYS2 mingw-w64)
REM          on PATH because Fyne uses cgo + OpenGL.

setlocal EnableExtensions

set "APP=helena.exe"
set "PKG=.\cmd\helena"

REM Phase 8 coverage gate: per-package floor for every internal/* except
REM internal/ui (UI tests deferred to Phase 11) and cmd/* (entrypoints).
set "COVERAGE_FLOOR=90"
set "COVERAGE_EXCLUDES=internal/ui,cmd,features,integration"
set "COVERAGE_PROFILE=coverage.out"
set "COVERAGE_HTML=coverage.html"

if "%~1"=="" goto :run
if /I "%~1"=="run"             goto :run
if /I "%~1"=="build"           goto :build
if /I "%~1"=="test"            goto :test
if /I "%~1"=="coverage"        goto :coverage
if /I "%~1"=="coverage-html"   goto :coverage-html
if /I "%~1"=="coverage-gate"   goto :coverage-gate
if /I "%~1"=="mutation"        goto :mutation
if /I "%~1"=="vet"             goto :vet
if /I "%~1"=="fmt"             goto :fmt
if /I "%~1"=="lint"            goto :lint
if /I "%~1"=="tidy"            goto :tidy
if /I "%~1"=="clean"           goto :clean
if /I "%~1"=="website"         goto :website
if /I "%~1"=="website-build"   goto :website-build
if /I "%~1"=="website-docker"  goto :website-docker
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

:coverage
go test ./... -coverprofile=%COVERAGE_PROFILE% -covermode=atomic
if errorlevel 1 goto :end
go run .\cmd\covergate -profile %COVERAGE_PROFILE% -exclude %COVERAGE_EXCLUDES%
goto :end

:coverage-html
if not exist %COVERAGE_PROFILE% (
    echo coverage profile %COVERAGE_PROFILE% not found - run "make coverage" first
    exit /b 1
)
go tool cover -html=%COVERAGE_PROFILE% -o %COVERAGE_HTML%
echo wrote %COVERAGE_HTML%
goto :end

:coverage-gate
go test ./... -coverprofile=%COVERAGE_PROFILE% -covermode=atomic
if errorlevel 1 goto :end
go run .\cmd\covergate -profile %COVERAGE_PROFILE% -exclude %COVERAGE_EXCLUDES% -floor %COVERAGE_FLOOR%
goto :end

REM Mutation testing via gremlins on the five load-bearing packages.
REM Auto-installs gremlins on first run if it isn't on the GOPATH bin.
:mutation
for /f "delims=" %%G in ('go env GOPATH') do set "GREMLINS=%%G\bin\gremlins.exe"
if not exist "%GREMLINS%" go install github.com/go-gremlins/gremlins/cmd/gremlins@latest
REM go clean -testcache before each package: gremlins re-uses go's
REM test cache for baseline timing; stale entries cause spurious
REM timeouts.
go clean -testcache
"%GREMLINS%" unleash --timeout-coefficient 6 .\internal\chain
go clean -testcache
"%GREMLINS%" unleash --timeout-coefficient 6 .\internal\storage
go clean -testcache
"%GREMLINS%" unleash --timeout-coefficient 6 .\internal\httpclient
go clean -testcache
"%GREMLINS%" unleash --timeout-coefficient 6 .\internal\scripting
go clean -testcache
"%GREMLINS%" unleash --timeout-coefficient 6 .\internal\auth
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
if exist %COVERAGE_PROFILE% del /Q %COVERAGE_PROFILE%
if exist %COVERAGE_HTML% del /Q %COVERAGE_HTML%
if exist website\_site rmdir /S /Q website\_site
if exist website\.jekyll-cache rmdir /S /Q website\.jekyll-cache
goto :end

REM Project website (Jekyll, website\). Needs Ruby + Bundler on PATH.
:website
where bundle >nul 2>&1
if errorlevel 1 ( echo bundler not found - install Ruby, then "gem install bundler" ^(see website\README.md^) & exit /b 1 )
echo Serving website at http://localhost:4000/helena/ - Ctrl-C to stop
pushd website
cmd /c "bundle check >nul 2>&1 || bundle install"
bundle exec jekyll serve --livereload
popd
goto :end

:website-build
where bundle >nul 2>&1
if errorlevel 1 ( echo bundler not found - install Ruby, then "gem install bundler" ^(or use "make.bat website-docker"^) & exit /b 1 )
pushd website
cmd /c "bundle check >nul 2>&1 || bundle install"
bundle exec jekyll build
popd
echo built website\_site
goto :end

REM Serve the website with no local Ruby, via the official ruby Docker image.
:website-docker
where docker >nul 2>&1
if errorlevel 1 ( echo docker not found - install Docker, or use "make.bat website" with Ruby & exit /b 1 )
echo Serving website via Docker at http://localhost:4000/helena/ - Ctrl-C to stop
docker run --rm -it -p 4000:4000 -v "%CD%\website:/site" -w /site ruby:3.3 sh -c "bundle install && bundle exec jekyll serve -H 0.0.0.0 --livereload --force_polling"
goto :end

:usage
echo Usage: %~n0 ^<target^>
echo Targets: run build test coverage coverage-html coverage-gate vet fmt lint tidy clean website website-build
exit /b 1

:end
endlocal & exit /b %ERRORLEVEL%
