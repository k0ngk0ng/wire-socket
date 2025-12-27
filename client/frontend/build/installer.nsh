; Custom NSIS installer script for WireSocket on Windows

; This script runs during installation

!macro customInstall
  ; Set permissions for binaries
  DetailPrint "Setting up WireSocket binaries..."

  ; Create a service for wire-socket-client
  ; Note: We use nssm (Non-Sucking Service Manager) if available
  ; Otherwise, provide instructions to user

  DetailPrint "WireSocket client backend will be installed as a service"
  DetailPrint "You may need to restart your computer for changes to take effect"
!macroend

!macro customUnInstall
  ; Stop and remove service if it exists
  DetailPrint "Removing WireSocket service..."
  nsExec::ExecToLog 'net stop "WireSocket Client"'
  nsExec::ExecToLog 'sc delete "WireSocket Client"'
!macroend
