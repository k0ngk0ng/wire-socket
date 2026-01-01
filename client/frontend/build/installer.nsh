; Custom NSIS installer script for WireSocket on Windows

; This script runs during installation

!macro customInstall
  ; Set permissions for binaries
  DetailPrint "Setting up WireSocket binaries..."

  ; Install the backend as a Windows service
  DetailPrint "Installing WireSocket Client Service..."

  ; Stop any existing service (ignore errors)
  nsExec::ExecToLog 'net stop WireSocketClient'

  ; Force kill the process if still running
  nsExec::ExecToLog 'taskkill /F /IM wire-socket-client.exe'

  ; Force delete the service
  nsExec::ExecToLog 'sc delete WireSocketClient'

  ; Small wait to ensure cleanup is complete
  Sleep 500

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
