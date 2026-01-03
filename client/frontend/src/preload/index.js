const { contextBridge, ipcRenderer } = require('electron');

// Expose protected methods to renderer process
contextBridge.exposeInMainWorld('electronAPI', {
  // Core VPN functions
  connect: (credentials) => ipcRenderer.invoke('api:connect', credentials),
  disconnect: () => ipcRenderer.invoke('api:disconnect'),
  getStatus: () => ipcRenderer.invoke('api:getStatus'),
  checkService: () => ipcRenderer.invoke('api:checkService'),

  // Tray
  updateTrayStatus: (isConnected) => ipcRenderer.invoke('tray:updateStatus', isConnected),
  onServiceStatus: (callback) => ipcRenderer.on('service:status', (event, status) => callback(status)),

  // Dev tools
  activateDevTools: () => ipcRenderer.invoke('devtools:activate'),

  // Password management
  changePassword: (data) => ipcRenderer.invoke('api:changePassword', data),
});
