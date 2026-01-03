; Custom NSIS installer script for WireSocket on Windows
; Note: Errors from stopping/deleting non-existent service are expected and ignored

; This runs BEFORE files are extracted - stop service and kill process first
!macro customInit
  ; Stop any existing service (silently, ignore errors)
  nsExec::ExecToStack 'net stop WireSocketClient'
  Pop $0
  Pop $1

  ; Force kill the process if still running (silently)
  nsExec::ExecToStack 'taskkill /F /IM wire-socket-client.exe'
  Pop $0
  Pop $1

  ; Force delete the service (silently)
  nsExec::ExecToStack 'sc delete WireSocketClient'
  Pop $0
  Pop $1

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
  nsExec::ExecToStack 'net stop WireSocketClient'
  Pop $0
  Pop $1

  ; Force kill the process
  nsExec::ExecToStack 'taskkill /F /IM wire-socket-client.exe'
  Pop $0
  Pop $1

  DetailPrint "Removing WireSocket service..."
  nsExec::ExecToLog '"$INSTDIR\resources\bin\wire-socket-client.exe" -service uninstall'

  ; Fallback: force delete service (silently)
  nsExec::ExecToStack 'sc delete WireSocketClient'
  Pop $0
  Pop $1
!macroend
