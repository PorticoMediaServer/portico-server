#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]
fn main(){
    #[cfg(target_os="linux")]
    portico_desktop::shell::configure_linux_webview();
    portico_desktop::shell::run();
}
