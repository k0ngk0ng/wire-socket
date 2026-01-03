; Custom NSIS installer script for WireSocket on Windows
; Using nsExec for silent command execution (no cmd window popup)

; This runs BEFORE files are extracted - stop service and kill process first
!macro customInit
  ; Stop any existing service (ignore errors if not running)
  nsExec::ExecToLog 'net stop WireSocketClient'

  ; Force kill the process if still running
  nsExec::ExecToLog 'taskkill /F /IM wire-socket-client.exe'

  ; Delete the service registration
  nsExec::ExecToLog 'sc delete WireSocketClient'

  ; Wait for cleanup to complete
  Sleep 2000
!macroend

; This runs AFTER files are installed
!macro customInstall
  DetailPrint "Installing WireSocket Client Service..."

  ; Install the service using the backend binary
  nsExec::ExecToLog '"$INSTDIR\resources\bin\wire-socket-client.exe" -service install'
  Pop $0
  DetailPrint "Service install returned: $0"

  ; Start the service
  DetailPrint "Starting WireSocket Client Service..."
  nsExec::ExecToLog 'net start WireSocketClient'
  Pop $0
  DetailPrint "Service start returned: $0"

  DetailPrint "WireSocket installation complete!"
!macroend

!macro customUnInstall
  ; Stop the service
  DetailPrint "Stopping WireSocket service..."
  nsExec::ExecToLog 'net stop WireSocketClient'

  ; Force kill the process
  nsExec::ExecToLog 'taskkill /F /IM wire-socket-client.exe'

  ; Uninstall the service
  DetailPrint "Removing WireSocket service..."
  nsExec::ExecToLog '"$INSTDIR\resources\bin\wire-socket-client.exe" -service uninstall'

  ; Force delete service as fallback
  nsExec::ExecToLog 'sc delete WireSocketClient'

  ; Wait for cleanup
  Sleep 1000
!macroend
