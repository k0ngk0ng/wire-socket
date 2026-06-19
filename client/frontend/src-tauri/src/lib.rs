use reqwest::{Client, Method};
use serde::Serialize;
use serde_json::{json, Value};
use std::{fs, path::PathBuf, process::Command, sync::Mutex, time::Duration};
use tauri::menu::{Menu, MenuBuilder, MenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{Emitter, Manager};
use tauri_plugin_deep_link::DeepLinkExt;
use tauri_plugin_opener::OpenerExt;

const SSO_PROTOCOL: &str = "wiresocket";
const TRAY_ID: &str = "wiresocket-tray";
const DEFAULT_PORT: u16 = 41945;
const MAX_PORT_TRIES: u16 = 10;

#[derive(Default)]
struct AppState {
    current_port: Mutex<u16>,
    connected_server: Mutex<Option<String>>,
}

#[derive(Serialize, Clone)]
struct ServiceStatus {
    running: bool,
}

#[derive(Serialize)]
struct ApiResult {
    success: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    data: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

impl ApiResult {
    fn ok(data: Value) -> Self {
        Self {
            success: true,
            data: Some(data),
            error: None,
        }
    }

    fn err(error: impl Into<String>) -> Self {
        Self {
            success: false,
            data: None,
            error: Some(error.into()),
        }
    }
}

fn platform_dir() -> &'static str {
    if cfg!(target_os = "macos") {
        "darwin"
    } else if cfg!(target_os = "windows") {
        "win32"
    } else {
        "linux"
    }
}

#[cfg(target_os = "macos")]
fn disable_macos_app_nap() {
    use objc2_foundation::{NSActivityOptions, NSProcessInfo, NSString};

    let reason = NSString::from_str("Keep WireSocket VPN available while hidden in the menu bar");
    let activity = NSProcessInfo::processInfo().beginActivityWithOptions_reason(
        NSActivityOptions::UserInitiatedAllowingIdleSystemSleep
            | NSActivityOptions::AutomaticTerminationDisabled
            | NSActivityOptions::SuddenTerminationDisabled,
        &reason,
    );
    std::mem::forget(activity);
}

#[cfg(not(target_os = "macos"))]
fn disable_macos_app_nap() {}

fn backend_binary_name() -> &'static str {
    if cfg!(target_os = "windows") {
        "wire-socket-client.exe"
    } else if cfg!(all(target_os = "macos", target_arch = "aarch64")) {
        "wire-socket-client-arm64"
    } else {
        "wire-socket-client"
    }
}

fn backend_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    if cfg!(debug_assertions) {
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        return Ok(manifest_dir
            .parent()
            .ok_or("invalid src-tauri layout")?
            .join("resources")
            .join("bin")
            .join(platform_dir())
            .join(backend_binary_name()));
    }

    let resource_dir = app
        .path()
        .resource_dir()
        .map_err(|e| format!("failed to resolve resource directory: {e}"))?;
    Ok(resource_dir
        .join("bin")
        .join(platform_dir())
        .join(backend_binary_name()))
}

fn port_file_path() -> PathBuf {
    if cfg!(target_os = "windows") {
        let base = std::env::var_os("ProgramData")
            .map(PathBuf::from)
            .unwrap_or_else(|| PathBuf::from(r"C:\ProgramData"));
        return base.join("WireSocket").join("wiresocket-port");
    }
    PathBuf::from("/tmp/wiresocket-port")
}

fn read_port_from_file() -> Option<u16> {
    let content = fs::read_to_string(port_file_path()).ok()?;
    let port = content.trim().parse::<u16>().ok()?;
    if port > 0 {
        Some(port)
    } else {
        None
    }
}

fn current_port(state: &AppState) -> u16 {
    *state.current_port.lock().unwrap_or_else(|p| p.into_inner())
}

fn set_current_port(state: &AppState, port: u16) {
    *state.current_port.lock().unwrap_or_else(|p| p.into_inner()) = port;
}

fn api_base(state: &AppState) -> String {
    format!("http://127.0.0.1:{}", current_port(state))
}

fn http_client(timeout: Duration) -> Result<Client, String> {
    Client::builder()
        .timeout(timeout)
        .build()
        .map_err(|e| format!("failed to create HTTP client: {e}"))
}

async fn check_backend_at(port: u16) -> bool {
    let Ok(client) = http_client(Duration::from_secs(2)) else {
        return false;
    };
    let url = format!("http://127.0.0.1:{port}/health");
    client
        .get(url)
        .send()
        .await
        .map(|r| r.status().is_success())
        .unwrap_or(false)
}

async fn find_service_port(state: &AppState) -> bool {
    if let Some(port) = read_port_from_file() {
        set_current_port(state, port);
        if check_backend_at(port).await {
            return true;
        }
    }

    for offset in 0..MAX_PORT_TRIES {
        let port = DEFAULT_PORT + offset;
        set_current_port(state, port);
        if check_backend_at(port).await {
            return true;
        }
    }

    set_current_port(state, DEFAULT_PORT);
    false
}

async fn backend_version(state: &AppState) -> Option<String> {
    let client = http_client(Duration::from_secs(2)).ok()?;
    let resp = client
        .get(format!("{}/health", api_base(state)))
        .send()
        .await
        .ok()?;
    let data = resp.json::<Value>().await.ok()?;
    data.get("version")
        .and_then(Value::as_str)
        .map(str::to_string)
}

fn local_backend_version(app: &tauri::AppHandle) -> Option<String> {
    let path = backend_path(app).ok()?;
    if !path.exists() {
        return None;
    }

    let output = Command::new(path).arg("-version").output().ok()?;
    let text = String::from_utf8_lossy(&output.stdout);
    text.split_whitespace()
        .collect::<Vec<_>>()
        .windows(2)
        .find(|w| w[0] == "version")
        .map(|w| w[1].to_string())
}

fn shell_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "'\\''"))
}

fn applescript_quote(value: &str) -> String {
    format!("\"{}\"", value.replace('\\', "\\\\").replace('"', "\\\""))
}

fn powershell_single_quote(value: &str) -> String {
    format!("'{}'", value.replace('\'', "''"))
}

fn run_elevated(command: &str) -> Result<(), String> {
    let status = if cfg!(target_os = "macos") {
        Command::new("osascript")
            .arg("-e")
            .arg(format!(
                "do shell script {} with administrator privileges",
                applescript_quote(command)
            ))
            .status()
    } else if cfg!(target_os = "windows") {
        let arg = format!("/C {command}");
        Command::new("powershell")
            .args([
                "-NoProfile",
                "-ExecutionPolicy",
                "Bypass",
                "-Command",
                &format!(
                    "Start-Process -FilePath 'cmd.exe' -ArgumentList {} -Verb RunAs -Wait",
                    powershell_single_quote(&arg)
                ),
            ])
            .status()
    } else {
        Command::new("pkexec")
            .arg("sh")
            .arg("-c")
            .arg(command)
            .status()
            .or_else(|_| {
                Command::new("sudo")
                    .arg("sh")
                    .arg("-c")
                    .arg(command)
                    .status()
            })
    }
    .map_err(|e| format!("failed to run elevated command: {e}"))?;

    if status.success() {
        Ok(())
    } else {
        Err(format!("elevated command failed with status: {status}"))
    }
}

#[cfg(target_os = "macos")]
fn macos_service_installed() -> bool {
    PathBuf::from("/Library/LaunchDaemons/WireSocketClient.plist").exists()
}

#[cfg(target_os = "macos")]
fn macos_run_at_load_enabled() -> bool {
    let Ok(content) = fs::read_to_string("/Library/LaunchDaemons/WireSocketClient.plist") else {
        return false;
    };
    let Some(key_pos) = content.find("<key>RunAtLoad</key>") else {
        return false;
    };
    let Some(true_pos) = content[key_pos..].find("<true/>") else {
        return false;
    };
    true_pos < 200
}

#[cfg(target_os = "macos")]
fn macos_service_loaded() -> bool {
    Command::new("launchctl")
        .arg("list")
        .output()
        .map(|out| String::from_utf8_lossy(&out.stdout).contains("WireSocketClient"))
        .unwrap_or(false)
}

fn install_and_start_service(app: &tauri::AppHandle, reinstall: bool) -> Result<(), String> {
    let backend = backend_path(app)?;
    if !backend.exists() {
        return Err(format!("backend binary not found: {}", backend.display()));
    }
    let backend = backend.to_string_lossy().to_string();
    let backend_q = shell_quote(&backend);

    let command = if cfg!(target_os = "macos") {
        #[cfg(target_os = "macos")]
        {
            let installed = macos_service_installed();
            let loaded = macos_service_loaded();
            let has_run_at_load = macos_run_at_load_enabled();
            if reinstall || !installed || !has_run_at_load {
                let mut cmd = String::new();
                if loaded {
                    cmd.push_str("launchctl unload /Library/LaunchDaemons/WireSocketClient.plist 2>/dev/null; ");
                }
                if installed {
                    cmd.push_str(&format!("{backend_q} -service uninstall 2>/dev/null; "));
                }
                cmd.push_str(&format!(
                    "mkdir -p /var/lib/wire-socket && {backend_q} -service install && launchctl load /Library/LaunchDaemons/WireSocketClient.plist"
                ));
                cmd
            } else if !loaded {
                "mkdir -p /var/lib/wire-socket && launchctl load /Library/LaunchDaemons/WireSocketClient.plist".to_string()
            } else {
                "mkdir -p /var/lib/wire-socket && launchctl kickstart -k system/WireSocketClient 2>/dev/null || (launchctl stop WireSocketClient; launchctl start WireSocketClient)".to_string()
            }
        }
        #[cfg(not(target_os = "macos"))]
        {
            unreachable!()
        }
    } else if cfg!(target_os = "windows") {
        if reinstall {
            format!(
                "net stop WireSocketClient 2>NUL & \"{backend}\" -service uninstall 2>NUL & \"{backend}\" -service install && net start WireSocketClient"
            )
        } else {
            format!("\"{backend}\" -service install 2>NUL & net start WireSocketClient")
        }
    } else {
        if reinstall {
            format!(
                "systemctl stop WireSocketClient 2>/dev/null; {backend_q} -service uninstall 2>/dev/null; mkdir -p /var/lib/wire-socket && {backend_q} -service install && systemctl start WireSocketClient"
            )
        } else {
            format!(
                "mkdir -p /var/lib/wire-socket && {backend_q} -service install 2>/dev/null || true; systemctl start WireSocketClient"
            )
        }
    };

    run_elevated(&command)
}

async fn ensure_service_running(app: tauri::AppHandle) -> bool {
    let state = app.state::<AppState>();
    if find_service_port(&state).await {
        if let (Some(running), Some(local)) =
            (backend_version(&state).await, local_backend_version(&app))
        {
            if running != local {
                let _ = install_and_start_service(&app, true);
                tokio::time::sleep(Duration::from_secs(3)).await;
                return find_service_port(&state).await;
            }
        }
        return true;
    }

    if install_and_start_service(&app, false).is_err() {
        return false;
    }
    tokio::time::sleep(Duration::from_secs(3)).await;
    find_service_port(&state).await
}

async fn request_backend(
    state: tauri::State<'_, AppState>,
    method: Method,
    path: &str,
    body: Option<Value>,
    timeout: Duration,
) -> ApiResult {
    let Ok(client) = http_client(timeout) else {
        return ApiResult::err("failed to create HTTP client");
    };

    let mut req = client.request(method, format!("{}{}", api_base(&state), path));
    if let Some(body) = body {
        req = req.json(&body);
    }

    match req.send().await {
        Ok(resp) => {
            let status = resp.status();
            let data = resp.json::<Value>().await.unwrap_or(Value::Null);
            if status.is_success() {
                ApiResult::ok(data)
            } else {
                ApiResult::err(
                    data.get("error")
                        .and_then(Value::as_str)
                        .unwrap_or_else(|| status.as_str())
                        .to_string(),
                )
            }
        }
        Err(e) => ApiResult::err(e.to_string()),
    }
}

fn show_main_window(app: &tauri::AppHandle) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.unminimize();
        let _ = window.show();
        let _ = window.set_focus();
    }
}

fn tray_status_label(app: &tauri::AppHandle, connected: bool) -> String {
    let state = app.state::<AppState>();
    let server = state
        .connected_server
        .lock()
        .unwrap_or_else(|p| p.into_inner())
        .clone();

    match (connected, server) {
        (true, Some(server)) if !server.is_empty() => format!("Connected: {server}"),
        (true, _) => "Status: Connected".to_string(),
        (false, _) => "Status: Disconnected".to_string(),
    }
}

fn create_tray_menu(app: &tauri::AppHandle, connected: bool) -> Result<Menu<tauri::Wry>, String> {
    let show_item = MenuItem::with_id(app, "show", "Show WireSocket", true, None::<&str>)
        .map_err(|e| e.to_string())?;
    let status_item = MenuItem::with_id(
        app,
        "status",
        tray_status_label(app, connected),
        false,
        None::<&str>,
    )
    .map_err(|e| e.to_string())?;
    let quit_item =
        MenuItem::with_id(app, "quit", "Quit", true, None::<&str>).map_err(|e| e.to_string())?;

    MenuBuilder::new(app)
        .item(&show_item)
        .separator()
        .item(&status_item)
        .separator()
        .item(&quit_item)
        .build()
        .map_err(|e| e.to_string())
}

fn update_tray_menu(app: &tauri::AppHandle, connected: bool) -> Result<(), String> {
    let menu = create_tray_menu(app, connected)?;
    if let Some(tray) = app.tray_by_id(TRAY_ID) {
        tray.set_menu(Some(menu)).map_err(|e| e.to_string())?;
    }
    Ok(())
}

fn handle_sso_url(app: &tauri::AppHandle, url_str: &str) -> bool {
    if !url_str.starts_with(&format!("{SSO_PROTOCOL}://")) {
        return false;
    }

    let parsed = match url::Url::parse(url_str) {
        Ok(url) => url,
        Err(e) => {
            let _ = app.emit(
                "sso-callback",
                json!({ "success": false, "error": format!("invalid callback URL: {e}") }),
            );
            return true;
        }
    };

    let token = parsed
        .query_pairs()
        .find(|(key, _)| key == "token")
        .map(|(_, value)| value.to_string());
    let error = parsed
        .query_pairs()
        .find(|(key, _)| key == "error")
        .map(|(_, value)| value.to_string());

    let payload = if let Some(token) = token {
        json!({ "success": true, "token": token })
    } else {
        json!({ "success": false, "error": error.unwrap_or_else(|| "missing token".to_string()) })
    };

    let _ = app.emit("sso-callback", payload);
    show_main_window(app);
    true
}

#[tauri::command]
async fn check_service(state: tauri::State<'_, AppState>) -> Result<bool, String> {
    Ok(find_service_port(&state).await)
}

#[tauri::command]
async fn connect(
    state: tauri::State<'_, AppState>,
    credentials: Value,
) -> Result<ApiResult, String> {
    Ok(request_backend(
        state,
        Method::POST,
        "/api/connect",
        Some(credentials),
        Duration::from_secs(30),
    )
    .await)
}

#[tauri::command]
async fn disconnect(state: tauri::State<'_, AppState>) -> Result<ApiResult, String> {
    Ok(request_backend(
        state,
        Method::POST,
        "/api/disconnect",
        Some(json!({})),
        Duration::from_secs(10),
    )
    .await)
}

#[tauri::command]
async fn get_status(state: tauri::State<'_, AppState>) -> Result<ApiResult, String> {
    Ok(request_backend(
        state,
        Method::GET,
        "/api/status",
        None,
        Duration::from_secs(8),
    )
    .await)
}

#[tauri::command]
async fn get_route_settings(state: tauri::State<'_, AppState>) -> Result<ApiResult, String> {
    Ok(request_backend(
        state,
        Method::GET,
        "/api/routes/settings",
        None,
        Duration::from_secs(10),
    )
    .await)
}

#[tauri::command]
async fn update_route_settings(
    state: tauri::State<'_, AppState>,
    excluded_routes: Vec<String>,
) -> Result<ApiResult, String> {
    Ok(request_backend(
        state,
        Method::PUT,
        "/api/routes/settings",
        Some(json!({ "excluded_routes": excluded_routes })),
        Duration::from_secs(10),
    )
    .await)
}

#[tauri::command]
async fn change_password(
    state: tauri::State<'_, AppState>,
    data: Value,
) -> Result<ApiResult, String> {
    Ok(request_backend(
        state,
        Method::POST,
        "/api/change-password",
        Some(data),
        Duration::from_secs(30),
    )
    .await)
}

#[tauri::command]
async fn update_tray_status(app: tauri::AppHandle, is_connected: bool) -> Result<bool, String> {
    update_tray_menu(&app, is_connected)?;
    Ok(true)
}

#[tauri::command]
async fn update_title(
    app: tauri::AppHandle,
    state: tauri::State<'_, AppState>,
    server_address: Option<String>,
) -> Result<bool, String> {
    *state
        .connected_server
        .lock()
        .unwrap_or_else(|p| p.into_inner()) = server_address.clone();

    let title = server_address
        .as_ref()
        .filter(|s| !s.is_empty())
        .map(|s| format!("WireSocket - {s}"))
        .unwrap_or_else(|| "WireSocket VPN".to_string());

    if let Some(window) = app.get_webview_window("main") {
        window.set_title(&title).map_err(|e| e.to_string())?;
    }
    update_tray_menu(&app, server_address.is_some())?;
    Ok(true)
}

#[tauri::command]
async fn activate_dev_tools() -> ApiResult {
    ApiResult::ok(json!({ "message": "dev tools are available in debug builds" }))
}

#[tauri::command]
async fn sso_get_providers(server_address: String) -> ApiResult {
    let Ok(client) = http_client(Duration::from_secs(10)) else {
        return ApiResult::err("failed to create HTTP client");
    };
    match client
        .get(format!("{server_address}/api/auth/providers"))
        .send()
        .await
    {
        Ok(resp) => {
            let status = resp.status();
            let data = resp.json::<Value>().await.unwrap_or(Value::Null);
            if status.is_success() {
                ApiResult::ok(data)
            } else {
                ApiResult::err(
                    data.get("error")
                        .and_then(Value::as_str)
                        .unwrap_or_else(|| status.as_str())
                        .to_string(),
                )
            }
        }
        Err(e) => ApiResult::err(e.to_string()),
    }
}

#[tauri::command]
async fn sso_login(
    app: tauri::AppHandle,
    server_address: String,
    provider_id: String,
) -> ApiResult {
    let callback_url = format!("{SSO_PROTOCOL}://auth/callback");
    let sso_url = format!(
        "{server_address}/api/auth/sso/{provider_id}?redirect_uri={}",
        urlencoding::encode(&callback_url)
    );

    match app.opener().open_url(&sso_url, None::<String>) {
        Ok(_) => ApiResult::ok(json!({})),
        Err(e) => ApiResult::err(e.to_string()),
    }
}

#[tauri::command]
async fn sso_connect_with_token(
    state: tauri::State<'_, AppState>,
    server_address: String,
    token: String,
) -> Result<ApiResult, String> {
    Ok(request_backend(
        state,
        Method::POST,
        "/api/connect",
        Some(json!({
            "server_address": server_address,
            "tunnel_url": server_address,
            "token": token
        })),
        Duration::from_secs(30),
    )
    .await)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    disable_macos_app_nap();

    let mut builder = tauri::Builder::default();

    #[cfg(any(target_os = "macos", target_os = "windows", target_os = "linux"))]
    {
        builder = builder.plugin(tauri_plugin_single_instance::init(|app, args, _cwd| {
            for arg in args {
                if handle_sso_url(app, &arg) {
                    return;
                }
            }
            show_main_window(app);
        }));
    }

    builder
        .plugin(tauri_plugin_deep_link::init())
        .plugin(tauri_plugin_opener::init())
        .manage(AppState {
            current_port: Mutex::new(DEFAULT_PORT),
            connected_server: Mutex::new(None),
        })
        .invoke_handler(tauri::generate_handler![
            check_service,
            connect,
            disconnect,
            get_status,
            get_route_settings,
            update_route_settings,
            change_password,
            update_tray_status,
            update_title,
            activate_dev_tools,
            sso_get_providers,
            sso_login,
            sso_connect_with_token
        ])
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                api.prevent_close();
                let _ = window.hide();
            }
        })
        .setup(|app| {
            #[cfg(any(target_os = "linux", all(debug_assertions, windows)))]
            {
                let _ = app.deep_link().register_all();
            }

            app.deep_link().on_open_url({
                let app_handle = app.handle().clone();
                move |event| {
                    for url in event.urls() {
                        if handle_sso_url(&app_handle, url.as_str()) {
                            break;
                        }
                    }
                }
            });

            let menu = create_tray_menu(app.handle(), false)
                .map_err(|e| std::io::Error::new(std::io::ErrorKind::Other, e))?;
            let mut tray_builder = TrayIconBuilder::with_id(TRAY_ID)
                .tooltip("WireSocket VPN")
                .menu(&menu)
                .show_menu_on_left_click(true)
                .on_menu_event(|app, event| match event.id.0.as_str() {
                    "show" => show_main_window(app),
                    "quit" => {
                        let app_handle = app.clone();
                        tauri::async_runtime::spawn(async move {
                            let state = app_handle.state::<AppState>();
                            let _ = request_backend(
                                state,
                                Method::POST,
                                "/api/disconnect",
                                Some(json!({})),
                                Duration::from_secs(5),
                            )
                            .await;
                            app_handle.exit(0);
                        });
                    }
                    _ => {}
                })
                .on_tray_icon_event(|tray, event| match event {
                    TrayIconEvent::Click {
                        button: MouseButton::Left,
                        button_state: MouseButtonState::Up,
                        ..
                    } => show_main_window(tray.app_handle()),
                    _ => {}
                });

            if let Some(icon) = app.default_window_icon() {
                tray_builder = tray_builder.icon(icon.clone());
                #[cfg(target_os = "macos")]
                {
                    tray_builder = tray_builder.icon_as_template(false);
                }
            }
            let _tray = tray_builder.build(app)?;

            let app_handle = app.handle().clone();
            tauri::async_runtime::spawn(async move {
                let running = ensure_service_running(app_handle.clone()).await;
                let _ = app_handle.emit("service-status", ServiceStatus { running });
            });

            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running WireSocket");
}
