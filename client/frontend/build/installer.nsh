; Custom NSIS installer script for WireSocket on Windows

; This script runs during installation

!macro customInstall
  ; Set permissions for binaries
  DetailPrint "Setting up WireSocket binaries..."

  ; Install the backend as a Windows service
  DetailPrint "Installing WireSocket Client Service..."

  ; First, try to stop any existing service
  nsExec::ExecToLog 'net stop WireSocketClient'

  ; Wait for service to fully stop
  Sleep 2000

  ; Delete any existing service
  nsExec::ExecToLog 'sc delete WireSocketClient'

  ; Wait for service to be deleted
  Sleep 1000

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

  DetailPrint "Removing WireSocket service..."
  nsExec::ExecToLog '"$INSTDIR\resources\bin\wire-socket-client.exe" -service uninstall'

  ; Fallback: try sc delete
  nsExec::ExecToLog 'sc delete WireSocketClient'
!macroend
