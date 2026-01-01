; Custom NSIS installer script for WireSocket on Windows

; This runs BEFORE files are extracted - stop service and kill process first
!macro customInit
  ; Stop any existing service (ignore errors)
  nsExec::ExecToLog 'net stop WireSocketClient'

  ; Force kill the process if still running
  nsExec::ExecToLog 'taskkill /F /IM wire-socket-client.exe'

  ; Force delete the service
  nsExec::ExecToLog 'sc delete WireSocketClient'

  ; Wait for cleanup
  Sleep 1000
!macroend

; This runs AFTER files are installed
!macro customInstall
  DetailPrint "Installing WireSocket Client Service..."

  ; Install the service using the backend binary
  nsExec::ExecToLog '"$INSTDIR\resources\bin\wire-socket-client.exe" -service install'

  ; Start the service
  DetailPrint "Starting WireSocket Client Service..."
  nsExec::ExecToLog 'net start WireSocketClient'

  DetailPrint "WireSocket installation complete!"
!macroend

!macro customUnInstall
  ; Stop and remove service if it exists
  DetailPrint "Stopping WireSocket service..."
  nsExec::ExecToLog 'net stop WireSocketClient'

  ; Force kill the process
  nsExec::ExecToLog 'taskkill /F /IM wire-socket-client.exe'

  DetailPrint "Removing WireSocket service..."
  nsExec::ExecToLog '"$INSTDIR\resources\bin\wire-socket-client.exe" -service uninstall'

  ; Fallback: force delete service
  nsExec::ExecToLog 'sc delete WireSocketClient'
!macroend
