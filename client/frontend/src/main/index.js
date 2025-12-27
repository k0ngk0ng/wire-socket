const { app, BrowserWindow, ipcMain, Tray, Menu, dialog } = require('electron');
const path = require('path');
const axios = require('axios');
const sudo = require('@vscode/sudo-prompt');
const fs = require('fs');

const API_BASE = 'http://127.0.0.1:41945';

let mainWindow = null;
let tray = null;

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
    return path.join(process.resourcesPath, 'bin', binaryName);
  } else {
    const platformDir = platform === 'darwin' ? 'darwin' : platform === 'win32' ? 'win32' : 'linux';
    return path.join(__dirname, '../../resources/bin', platformDir, binaryName);
  }
}

// Check if backend service is running
async function checkBackendService() {
  try {
    await axios.get(`${API_BASE}/health`, { timeout: 2000 });
    return true;
  } catch (error) {
    return false;
  }
}

// Install and start the backend service with elevated privileges
function installAndStartService() {
  return new Promise((resolve, reject) => {
    const backendPath = getBackendPath();
    const platform = process.platform;

    if (!fs.existsSync(backendPath)) {
      reject(new Error(`Backend binary not found: ${backendPath}`));
      return;
    }

    let command;
    if (platform === 'darwin') {
      // macOS: Install service and load it
      command = `"${backendPath}" -service install && launchctl load /Library/LaunchDaemons/WireSocketClient.plist`;
    } else if (platform === 'linux') {
      // Linux: Install service and start it
      command = `"${backendPath}" -service install && systemctl start WireSocketClient`;
    } else if (platform === 'win32') {
      // Windows: Install service and start it
      command = `"${backendPath}" -service install && net start WireSocketClient`;
    } else {
      reject(new Error(`Unsupported platform: ${platform}`));
      return;
    }

    const options = {
      name: 'WireSocket VPN',
    };

    console.log('Installing service with command:', command);

    sudo.exec(command, options, (error, stdout, stderr) => {
      if (error) {
        console.error('Service install error:', error);
        // Check if it's just already installed
        if (stderr && stderr.includes('already exists')) {
          resolve();
          return;
        }
        reject(error);
        return;
      }
      console.log('Service installed successfully:', stdout);
      resolve();
    });
  });
}

// Start the backend service (if already installed but not running)
function startService() {
  return new Promise((resolve, reject) => {
    const platform = process.platform;

    let command;
    if (platform === 'darwin') {
      command = 'launchctl load /Library/LaunchDaemons/WireSocketClient.plist 2>/dev/null || launchctl start WireSocketClient';
    } else if (platform === 'linux') {
      command = 'systemctl start WireSocketClient';
    } else if (platform === 'win32') {
      command = 'net start WireSocketClient';
    } else {
      reject(new Error(`Unsupported platform: ${platform}`));
      return;
    }

    const options = {
      name: 'WireSocket VPN',
    };

    sudo.exec(command, options, (error, stdout, stderr) => {
      if (error) {
        // Ignore errors if service is already running
        if (stderr && (stderr.includes('already loaded') || stderr.includes('already started'))) {
          resolve();
          return;
        }
        console.error('Service start error:', error);
        reject(error);
        return;
      }
      console.log('Service started:', stdout);
      resolve();
    });
  });
}

// Ensure backend service is running
async function ensureServiceRunning() {
  // First check if already running
  if (await checkBackendService()) {
    console.log('Backend service is already running');
    return true;
  }

  console.log('Backend service not running, attempting to start...');

  // Try to start it first (might already be installed)
  try {
    await startService();
    // Wait a bit for service to start
    await new Promise(resolve => setTimeout(resolve, 2000));
    if (await checkBackendService()) {
      console.log('Backend service started successfully');
      return true;
    }
  } catch (error) {
    console.log('Failed to start service, trying to install...');
  }

  // If start failed, try to install and start
  try {
    await installAndStartService();
    // Wait for service to start
    await new Promise(resolve => setTimeout(resolve, 3000));
    if (await checkBackendService()) {
      console.log('Backend service installed and started successfully');
      return true;
    }
  } catch (error) {
    console.error('Failed to install service:', error);
    return false;
  }

  return false;
}

// Show error dialog if service failed to start
function showServiceErrorDialog(error) {
  dialog.showMessageBox(mainWindow, {
    type: 'error',
    title: 'Service Error',
    message: `Failed to start the VPN service.\n\nError: ${error}\n\nPlease try running the app as administrator or install the service manually.`,
    buttons: ['Retry', 'Quit'],
  }).then((result) => {
    if (result.response === 0) {
      initializeService();
    } else {
      app.quit();
    }
  });
}

// Initialize service on app start
async function initializeService() {
  try {
    const success = await ensureServiceRunning();
    if (!success) {
      showServiceErrorDialog('Service failed to start');
    } else {
      // Notify renderer that service is available
      if (mainWindow) {
        mainWindow.webContents.send('service:status', { running: true });
      }
    }
  } catch (error) {
    showServiceErrorDialog(error.message);
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

app.whenReady().then(async () => {
  createWindow();
  createTray();

  // Initialize and ensure backend service is running
  await initializeService();

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

ipcMain.handle('api:checkService', async () => {
  return await checkBackendService();
});

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
