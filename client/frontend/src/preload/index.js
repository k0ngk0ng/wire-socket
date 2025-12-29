const { contextBridge, ipcRenderer } = require('electron');

// Expose protected methods to renderer process
contextBridge.exposeInMainWorld('electronAPI', {
  connect: (credentials) => ipcRenderer.invoke('api:connect', credentials),
  disconnect: () => ipcRenderer.invoke('api:disconnect'),
  getStatus: () => ipcRenderer.invoke('api:getStatus'),
  getServers: () => ipcRenderer.invoke('api:getServers'),
  addServer: (server) => ipcRenderer.invoke('api:addServer', server),
  checkService: () => ipcRenderer.invoke('api:checkService'),
  updateTrayStatus: (isConnected) => ipcRenderer.invoke('tray:updateStatus', isConnected),
  onServiceStatus: (callback) => ipcRenderer.on('service:status', (event, status) => callback(status)),
  activateDevTools: () => ipcRenderer.invoke('devtools:activate'),

  // Route settings
  getRouteSettings: () => ipcRenderer.invoke('api:getRouteSettings'),
  updateRouteSettings: (excludedRoutes) => ipcRenderer.invoke('api:updateRouteSettings', excludedRoutes),

  // Password management
  changePassword: (data) => ipcRenderer.invoke('api:changePassword', data),
});
