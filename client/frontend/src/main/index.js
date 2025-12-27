const { app, BrowserWindow, ipcMain, Tray, Menu } = require('electron');
const path = require('path');
const axios = require('axios');
const { spawn } = require('child_process');
const fs = require('fs');

const API_BASE = 'http://127.0.0.1:41945';

let mainWindow = null;
let tray = null;
let backendProcess = null;

// Get the path to the backend binary
function getBackendPath() {
  const isPackaged = app.isPackaged;
  const platform = process.platform;
  const arch = process.arch;

  let binaryName = 'wire-socket-client';
  if (platform === 'win32') {
    binaryName = 'wire-socket-client.exe';
  } else if (platform === 'darwin' && arch === 'arm64') {
    binaryName = 'wire-socket-client-arm64';
  }

  if (isPackaged) {
    // In packaged app, binary is in resources/bin/
    return path.join(process.resourcesPath, 'bin', binaryName);
  } else {
    // In development, binary is in resources/bin/{platform}/
    const platformDir = platform === 'darwin' ? 'darwin' : platform === 'win32' ? 'win32' : 'linux';
    return path.join(__dirname, '../../resources/bin', platformDir, binaryName);
  }
}

// Start the backend service
function startBackend() {
  const backendPath = getBackendPath();

  console.log('Backend path:', backendPath);

  // Check if backend binary exists
  if (!fs.existsSync(backendPath)) {
    console.error('Backend binary not found:', backendPath);
    return;
  }

  // Check if backend is already running
  axios.get(`${API_BASE}/health`)
    .then(() => {
      console.log('Backend already running');
    })
    .catch(() => {
      console.log('Starting backend...');

      backendProcess = spawn(backendPath, [], {
        detached: false,
        stdio: ['ignore', 'pipe', 'pipe'],
      });

      backendProcess.stdout.on('data', (data) => {
        console.log(`Backend: ${data}`);
      });

      backendProcess.stderr.on('data', (data) => {
        console.error(`Backend error: ${data}`);
      });

      backendProcess.on('error', (err) => {
        console.error('Failed to start backend:', err);
      });

      backendProcess.on('exit', (code) => {
        console.log(`Backend exited with code ${code}`);
        backendProcess = null;
      });
    });
}

// Stop the backend service
function stopBackend() {
  if (backendProcess) {
    console.log('Stopping backend...');
    backendProcess.kill();
    backendProcess = null;
  }
}

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
  // Start backend first
  startBackend();

  // Wait a bit for backend to start, then create window
  setTimeout(() => {
    createWindow();
    createTray();
  }, 1000);

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
  stopBackend();
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
