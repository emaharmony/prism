@echo off
setlocal

rem Resolve the repository root relative to this launcher so the scheduled task
rem keeps using the current Prism config and durable binary.
for %%I in ("%~dp0..\..") do set "PRISM_ROOT=%%~fI"
cd /d "%PRISM_ROOT%"
if not exist "logs" mkdir "logs"

"%PRISM_ROOT%\bin\prism.exe" serve --config "%PRISM_ROOT%\prism.yaml" >> "%PRISM_ROOT%\logs\prism-serve.log" 2>&1
