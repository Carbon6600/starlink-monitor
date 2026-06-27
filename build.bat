@echo off
set CGO_ENABLED=1
set PATH=C:\msys64\mingw64\bin;%PATH%
echo Building Starlink Monitor...
echo Generating Windows resource file (icon)...
rsrc -ico app.ico -o rsrc.syso
go build -ldflags="-H=windowsgui" -o starlink_security_v1.0.0.exe .
if %ERRORLEVEL% EQU 0 (
    echo Build Successful: starlink_security_v1.0.0.exe created.
) else (
    echo Build Failed with error code %ERRORLEVEL%.
)
pause
