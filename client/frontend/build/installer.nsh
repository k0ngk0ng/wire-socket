; Custom NSIS installer script for WireSocket on Windows
; Using ExecWait for more reliable service management

; This runs BEFORE files are extracted - stop service and kill process first
!macro customInit
  ; Stop any existing service (ignore errors if not running)
  ExecWait 'net stop WireSocketClient' $0

  ; Force kill the process if still running
  ExecWait 'taskkill /F /IM wire-socket-client.exe' $0

  ; Delete the service registration
  ExecWait 'sc delete WireSocketClient' $0

  ; Wait for cleanup to complete
  Sleep 2000
!macroend

; This runs AFTER files are installed
!macro customInstall
  DetailPrint "Installing WireSocket Client Service..."

  ; Install the service using the backend binary
  ExecWait '"$INSTDIR\resources\bin\wire-socket-client.exe" -service install' $0
  DetailPrint "Service install returned: $0"

  ; Start the service
  DetailPrint "Starting WireSocket Client Service..."
  ExecWait 'net start WireSocketClient' $0
  DetailPrint "Service start returned: $0"

  DetailPrint "WireSocket installation complete!"
!macroend

!macro customUnInstall
  ; Stop the service
  DetailPrint "Stopping WireSocket service..."
  ExecWait 'net stop WireSocketClient' $0

  ; Force kill the process
  ExecWait 'taskkill /F /IM wire-socket-client.exe' $0

  ; Uninstall the service
  DetailPrint "Removing WireSocket service..."
  ExecWait '"$INSTDIR\resources\bin\wire-socket-client.exe" -service uninstall' $0

  ; Force delete service as fallback
  ExecWait 'sc delete WireSocketClient' $0

  ; Wait for cleanup
  Sleep 1000
!macroend
