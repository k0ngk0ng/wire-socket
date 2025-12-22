const { app, BrowserWindow, ipcMain, Tray, Menu } = require('electron');
const path = require('path');
const axios = require('axios');

const API_BASE = 'http://127.0.0.1:41945';

let mainWindow = null;
let tray = null;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 800,
    height: 600,
    webPreferences: {
      preload: path.join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  mainWindow.loadFile(path.join(__dirname, '../../public/index.html'));

  mainWindow.on('closed', () => {
    mainWindow = null;
  });

  // Hide instead of close
  mainWindow.on('close', (event) => {
    if (!app.isQuitting) {
      event.preventDefault();
      mainWindow.hide();
    }
  });
}

function createTray() {
  // Note: You'll need to provide an icon file
  // tray = new Tray(path.join(__dirname, '../../public/icon.png'));

  const contextMenu = Menu.buildFromTemplate([
    {
      label: 'Show App',
      click: () => {
        if (mainWindow) {
          mainWindow.show();
        }
      },
    },
    {
      label: 'Quit',
      click: () => {
        app.isQuitting = true;
        app.quit();
      },
    },
  ]);

  if (tray) {
    tray.setContextMenu(contextMenu);
    tray.setToolTip('VPN Client');
  }
}

app.whenReady().then(() => {
  createWindow();
  createTray();

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow();
    }
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('before-quit', () => {
  app.isQuitting = true;
});

// IPC Handlers

ipcMain.handle('api:connect', async (event, credentials) => {
  try {
    const response = await axios.post(`${API_BASE}/api/connect`, credentials);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:disconnect', async () => {
  try {
    const response = await axios.post(`${API_BASE}/api/disconnect`);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:getStatus', async () => {
  try {
    const response = await axios.get(`${API_BASE}/api/status`);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:getServers', async () => {
  try {
    const response = await axios.get(`${API_BASE}/api/servers`);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:addServer', async (event, server) => {
  try {
    const response = await axios.post(`${API_BASE}/api/servers`, server);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});
