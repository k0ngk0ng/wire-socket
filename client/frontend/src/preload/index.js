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

  // Route settings
  getRouteSettings: () => ipcRenderer.invoke('api:getRouteSettings'),
  updateRouteSettings: (excludedRoutes) => ipcRenderer.invoke('api:updateRouteSettings', excludedRoutes),

  // Password management
  changePassword: (data) => ipcRenderer.invoke('api:changePassword', data),

  // App title/tooltip
  updateTitle: (serverAddress) => ipcRenderer.invoke('app:updateTitle', serverAddress),

  // SSO functions
  ssoGetProviders: (serverAddress) => ipcRenderer.invoke('sso:getProviders', serverAddress),
  ssoLogin: (serverAddress, providerId) => ipcRenderer.invoke('sso:login', { serverAddress, providerId }),
  ssoConnectWithToken: (serverAddress, token) => ipcRenderer.invoke('sso:connectWithToken', { serverAddress, token }),
  onSSOCallback: (callback) => ipcRenderer.on('sso:callback', (event, result) => callback(result)),
});
