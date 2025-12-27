const { contextBridge, ipcRenderer } = require('electron');

// Expose protected methods to renderer process
contextBridge.exposeInMainWorld('electronAPI', {
  connect: (credentials) => ipcRenderer.invoke('api:connect', credentials),
  disconnect: () => ipcRenderer.invoke('api:disconnect'),
  getStatus: () => ipcRenderer.invoke('api:getStatus'),
  getServers: () => ipcRenderer.invoke('api:getServers'),
  addServer: (server) => ipcRenderer.invoke('api:addServer', server),
  updateTrayStatus: (isConnected) => ipcRenderer.invoke('tray:updateStatus', isConnected),
});
