const { app, BrowserWindow, ipcMain, Tray, Menu, dialog, nativeImage, globalShortcut } = require('electron');
const path = require('path');
const axios = require('axios');
const sudo = require('@vscode/sudo-prompt');
const fs = require('fs');
const { execSync } = require('child_process');

// Dev tools activation state
let devToolsActivated = false;

// Default port, will be updated if a port file exists
const DEFAULT_PORT = 41945;
const MAX_PORT_TRIES = 10;
let currentPort = DEFAULT_PORT;

function getApiBase() {
  return `http://127.0.0.1:${currentPort}`;
}

// Get the path to the port file
function getPortFilePath() {
  if (process.platform === 'win32') {
    return path.join(process.env.TEMP || 'C:\\Windows\\Temp', 'wiresocket-port');
  }
  return '/tmp/wiresocket-port';
}

// Read the port from the port file
function readPortFromFile() {
  try {
    const portFile = getPortFilePath();
    if (fs.existsSync(portFile)) {
      const content = fs.readFileSync(portFile, 'utf-8').trim();
      const port = parseInt(content, 10);
      if (!isNaN(port) && port > 0 && port < 65536) {
        return port;
      }
    }
  } catch (error) {
    console.log('Could not read port file:', error.message);
  }
  return DEFAULT_PORT;
}

// Try to find the running service by checking multiple ports
async function findServicePort() {
  // First check the port file
  const filePort = readPortFromFile();
  currentPort = filePort;

  if (await checkBackendService()) {
    console.log(`Found service on port ${currentPort} (from port file)`);
    return true;
  }

  // If port file port didn't work, try scanning ports
  for (let i = 0; i < MAX_PORT_TRIES; i++) {
    const port = DEFAULT_PORT + i;
    currentPort = port;
    if (await checkBackendService()) {
      console.log(`Found service on port ${currentPort} (by scanning)`);
      return true;
    }
  }

  // Reset to default
  currentPort = DEFAULT_PORT;
  return false;
}

let mainWindow = null;
let tray = null;
let isQuitting = false;

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
    const response = await axios.get(`${getApiBase()}/health`, { timeout: 2000 });
    return true;
  } catch (error) {
    return false;
  }
}

// Get backend service version
async function getBackendVersion() {
  try {
    const response = await axios.get(`${getApiBase()}/health`, { timeout: 2000 });
    return response.data?.version || null;
  } catch (error) {
    return null;
  }
}

// Get local binary version
function getLocalBinaryVersion() {
  try {
    const backendPath = getBackendPath();
    if (!fs.existsSync(backendPath)) return null;
    const result = execSync(`"${backendPath}" -version`, { encoding: 'utf-8', timeout: 5000 });
    // Parse "wire-socket-client version X.X.X" format
    const match = result.match(/version\s+(\S+)/);
    return match ? match[1] : null;
  } catch (error) {
    console.log('Could not get local binary version:', error.message);
    return null;
  }
}

// Restart service to load new version (macOS)
function restartService() {
  return new Promise((resolve, reject) => {
    const platform = process.platform;

    let command;
    if (platform === 'darwin') {
      command = 'launchctl kickstart -k system/WireSocketClient';
    } else if (platform === 'linux') {
      command = 'systemctl restart WireSocketClient';
    } else if (platform === 'win32') {
      command = 'net stop WireSocketClient && net start WireSocketClient';
    } else {
      resolve(); // Skip for unknown platforms
      return;
    }

    const options = { name: 'WireSocket VPN' };

    console.log('Restarting service to load new version...');

    sudo.exec(command, options, (error, stdout, stderr) => {
      if (error) {
        console.error('Service restart error:', error);
        // Don't reject, service might still work
      }
      resolve();
    });
  });
}

// Check if macOS launchd plist exists (without sudo)
function isMacOSServiceInstalled() {
  if (process.platform !== 'darwin') return false;
  return fs.existsSync('/Library/LaunchDaemons/WireSocketClient.plist');
}

// Check if macOS service is loaded (without sudo)
function isMacOSServiceLoaded() {
  if (process.platform !== 'darwin') return false;
  try {
    const result = execSync('launchctl list | grep WireSocketClient', { encoding: 'utf-8', stdio: ['pipe', 'pipe', 'pipe'] });
    return result.includes('WireSocketClient');
  } catch {
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
      // macOS: Build command based on current state
      const isInstalled = isMacOSServiceInstalled();
      const isLoaded = isMacOSServiceLoaded();

      if (!isInstalled) {
        // Need to install and load
        // Create config directory first
        command = `mkdir -p /var/lib/wiresocket && "${backendPath}" -service install && launchctl load /Library/LaunchDaemons/WireSocketClient.plist`;
      } else if (!isLoaded) {
        // Already installed, just need to load
        command = `mkdir -p /var/lib/wiresocket && launchctl load /Library/LaunchDaemons/WireSocketClient.plist`;
      } else {
        // Already loaded, try to kickstart
        command = `mkdir -p /var/lib/wiresocket && launchctl kickstart -k system/WireSocketClient 2>/dev/null || (launchctl stop WireSocketClient; launchctl start WireSocketClient)`;
      }
    } else if (platform === 'linux') {
      // Linux: Install service and start it
      command = `mkdir -p /var/lib/wiresocket && "${backendPath}" -service install && systemctl start WireSocketClient`;
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
        console.error('stderr:', stderr);
        // Check if it's just already installed/loaded
        if (stderr && (stderr.includes('already exists') || stderr.includes('already loaded') || stderr.includes('service exists'))) {
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
// NOTE: This is only used as a fallback for non-macOS or if installAndStartService fails
function startService() {
  return new Promise((resolve, reject) => {
    const platform = process.platform;

    let command;
    if (platform === 'darwin') {
      // For macOS, prefer launchctl kickstart
      command = 'launchctl kickstart -k system/WireSocketClient 2>/dev/null || launchctl start WireSocketClient';
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
        if (stderr && (stderr.includes('already loaded') || stderr.includes('already started') || stderr.includes('already running'))) {
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
  // First check if already running (try multiple ports)
  if (await findServicePort()) {
    console.log(`Backend service is already running on port ${currentPort}`);

    // Check if we need to restart for version update
    const runningVersion = await getBackendVersion();
    const localVersion = getLocalBinaryVersion();

    if (runningVersion && localVersion && runningVersion !== localVersion) {
      console.log(`Version mismatch: running ${runningVersion}, local ${localVersion}`);
      console.log('Restarting service to load new version...');

      try {
        await restartService();
        // Wait for service to restart
        await new Promise(resolve => setTimeout(resolve, 3000));
        if (await findServicePort()) {
          console.log('Service restarted successfully with new version');
        }
      } catch (error) {
        console.error('Failed to restart service:', error);
        // Continue with old version
      }
    }

    return true;
  }

  console.log('Backend service not running, attempting to start...');

  // On macOS, check the service state first and only prompt once
  if (process.platform === 'darwin') {
    try {
      await installAndStartService();
      // Wait for service to start and find its port
      await new Promise(resolve => setTimeout(resolve, 3000));
      if (await findServicePort()) {
        console.log(`Backend service started successfully on port ${currentPort}`);
        return true;
      }
      // If still not running, wait a bit more and try again
      await new Promise(resolve => setTimeout(resolve, 2000));
      if (await findServicePort()) {
        console.log(`Backend service started successfully (delayed) on port ${currentPort}`);
        return true;
      }
    } catch (error) {
      console.error('Failed to start service:', error);
      return false;
    }
    return false;
  }

  // For Linux/Windows, try the original logic
  // Try to start it first (might already be installed)
  try {
    await startService();
    // Wait a bit for service to start
    await new Promise(resolve => setTimeout(resolve, 2000));
    if (await findServicePort()) {
      console.log(`Backend service started successfully on port ${currentPort}`);
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
    if (await findServicePort()) {
      console.log(`Backend service installed and started successfully on port ${currentPort}`);
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

function getAppIconPath() {
  const isPackaged = app.isPackaged;
  if (isPackaged) {
    return path.join(process.resourcesPath, 'assets', 'icon-1024.png');
  } else {
    return path.join(__dirname, '../../assets/icon-1024.png');
  }
}

function createWindow() {
  const iconPath = getAppIconPath();

  mainWindow = new BrowserWindow({
    width: 800,
    height: 600,
    show: false, // Don't show until ready
    icon: iconPath,
    webPreferences: {
      preload: path.join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
    },
  });

  // Set dock icon on macOS
  if (process.platform === 'darwin' && app.dock) {
    app.dock.setIcon(iconPath);
  }

  mainWindow.loadFile(path.join(__dirname, '../../public/index.html'));

  // Show window when ready to prevent visual flash
  mainWindow.once('ready-to-show', () => {
    mainWindow.show();
  });

  mainWindow.on('closed', () => {
    mainWindow = null;
  });

  // Minimize to tray instead of close (all platforms)
  mainWindow.on('close', (event) => {
    if (!isQuitting) {
      event.preventDefault();
      mainWindow.hide();

      // On macOS, also hide from dock when minimized to tray
      if (process.platform === 'darwin') {
        app.dock.hide();
      }
    }
  });

  // Handle minimize to tray
  mainWindow.on('minimize', (event) => {
    event.preventDefault();
    mainWindow.hide();

    // On macOS, also hide from dock when minimized to tray
    if (process.platform === 'darwin') {
      app.dock.hide();
    }
  });
}

function createTray() {
  // Load tray icon from assets
  let iconPath;
  const isPackaged = app.isPackaged;

  if (process.platform === 'darwin') {
    // macOS: Use template image for menu bar
    if (isPackaged) {
      iconPath = path.join(process.resourcesPath, 'assets', 'tray-icon-mac.png');
    } else {
      iconPath = path.join(__dirname, '../../assets/tray-icon-mac.png');
    }
  } else {
    // Windows/Linux: Use color icon
    if (isPackaged) {
      iconPath = path.join(process.resourcesPath, 'assets', 'tray-icon.png');
    } else {
      iconPath = path.join(__dirname, '../../assets/tray-icon.png');
    }
  }

  let icon = nativeImage.createFromPath(iconPath);

  // macOS: Set as template image so it adapts to light/dark menu bar
  if (process.platform === 'darwin') {
    icon.setTemplateImage(true);
  }

  tray = new Tray(icon);

  // Update tray context menu
  updateTrayMenu();

  tray.setToolTip('WireSocket VPN');

  // Double-click to show window (Windows/Linux)
  tray.on('double-click', () => {
    showWindow();
  });

  // Single click to show window on macOS
  if (process.platform === 'darwin') {
    tray.on('click', () => {
      showWindow();
    });
  }
}

function showWindow() {
  if (mainWindow) {
    // Show dock icon on macOS
    if (process.platform === 'darwin') {
      app.dock.show();
    }
    mainWindow.show();
    mainWindow.focus();
  }
}

function updateTrayMenu(isConnected = false) {
  const contextMenu = Menu.buildFromTemplate([
    {
      label: 'Show WireSocket',
      click: () => {
        showWindow();
      },
    },
    { type: 'separator' },
    {
      label: isConnected ? 'Status: Connected' : 'Status: Disconnected',
      enabled: false,
    },
    { type: 'separator' },
    {
      label: 'Quit',
      click: () => {
        isQuitting = true;
        app.quit();
      },
    },
  ]);

  if (tray) {
    tray.setContextMenu(contextMenu);
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
  isQuitting = true;
});

// IPC Handlers

ipcMain.handle('api:checkService', async () => {
  return await checkBackendService();
});

ipcMain.handle('api:connect', async (event, credentials) => {
  try {
    const response = await axios.post(`${getApiBase()}/api/connect`, credentials);
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
    const response = await axios.post(`${getApiBase()}/api/disconnect`);
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
    const response = await axios.get(`${getApiBase()}/api/status`);
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
    const response = await axios.get(`${getApiBase()}/api/servers`);
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
    const response = await axios.post(`${getApiBase()}/api/servers`, server);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

// Route settings handlers
ipcMain.handle('api:getRouteSettings', async () => {
  try {
    const response = await axios.get(`${getApiBase()}/api/routes/settings`);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:updateRouteSettings', async (event, excludedRoutes) => {
  try {
    const response = await axios.put(`${getApiBase()}/api/routes/settings`, {
      excluded_routes: excludedRoutes
    });
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

// Password change handler
ipcMain.handle('api:changePassword', async (event, data) => {
  try {
    const response = await axios.post(`${getApiBase()}/api/change-password`, data);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

// Auth handlers (multi-server)
ipcMain.handle('api:auth:login', async (event, data) => {
  try {
    const response = await axios.post(`${getApiBase()}/api/auth/login`, data);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:auth:logout', async (event, data) => {
  try {
    const response = await axios.post(`${getApiBase()}/api/auth/logout`, data);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

// Update tray menu when connection status changes
ipcMain.handle('tray:updateStatus', async (event, isConnected) => {
  updateTrayMenu(isConnected);
  return { success: true };
});

// Dev tools activation handler
ipcMain.handle('devtools:activate', async () => {
  devToolsActivated = true;
  console.log('Dev tools activated - press Cmd+Option+I (macOS) or Ctrl+Shift+I (Windows/Linux) to open');

  // Auto-deactivate after 10 seconds if not used
  setTimeout(() => {
    devToolsActivated = false;
  }, 10000);

  return { success: true };
});

// Multi-tunnel handlers
ipcMain.handle('api:tunnels:getStatus', async () => {
  try {
    const response = await axios.get(`${getApiBase()}/api/tunnels`);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:tunnels:connect', async (event, data) => {
  try {
    const response = await axios.post(`${getApiBase()}/api/tunnels/connect`, data);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:tunnels:disconnect', async (event, tunnelId) => {
  try {
    const response = await axios.post(`${getApiBase()}/api/tunnels/disconnect`, { tunnel_id: tunnelId });
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

ipcMain.handle('api:tunnels:disconnectAll', async () => {
  try {
    const response = await axios.post(`${getApiBase()}/api/tunnels/disconnect-all`);
    return { success: true, data: response.data };
  } catch (error) {
    return {
      success: false,
      error: error.response?.data?.error || error.message
    };
  }
});

// Register keyboard shortcut for dev tools (only works when activated)
app.whenReady().then(() => {
  // Disable default dev tools shortcut by intercepting it
  const shortcut = process.platform === 'darwin' ? 'CommandOrControl+Option+I' : 'Control+Shift+I';

  globalShortcut.register(shortcut, () => {
    if (devToolsActivated && mainWindow) {
      mainWindow.webContents.toggleDevTools();
      devToolsActivated = false; // Reset after use
    }
  });
});
