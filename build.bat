@echo off
set CGO_ENABLED=1
set PATH=C:\msys64\mingw64\bin;%PATH%
echo Building Starlink Monitor...
go build -ldflags="-s -w -H=windowsgui" -o starlink_security_v1.0.1.exe main.go
if %ERRORLEVEL% EQU 0 (
    echo Build Successful: starlink_security_v1.0.1.exe created.
) else (
    echo Build Failed with error code %ERRORLEVEL%.
)
pause
