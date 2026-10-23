use crate::{audio::AudioOutput,boundary::{self,Command,Error,Observation,Result},mpv::{Api,Mpv}};
use std::{path::PathBuf,sync::{Arc,Mutex}};
use tauri::{Manager,WebviewWindow};
use crate::{equalizer::{EqualizerChange,EqualizerSnapshot},equalizer_store::EqualizerStore};

#[cfg(target_os="linux")]
#[path="surface_linux.rs"] mod surface;
#[cfg(target_os="macos")]
#[path="surface_macos.rs"] mod surface;
#[cfg(target_os="windows")]
#[path="surface_windows.rs"] mod surface;

pub struct EngineState(pub Mutex<std::result::Result<Arc<Mpv>,Error>>);
pub struct EqualizerState(pub Mutex<std::result::Result<EqualizerStore,Error>>);
pub struct DownloadsState(pub Arc<Mutex<std::result::Result<crate::download_commands::DownloadsSession,Error>>>);
fn caller(window:&WebviewWindow)->Result<()> {
    boundary::local_origin(window.label(),window.url().map_err(|_|Error::ForbiddenOrigin)?.as_str(),cfg!(debug_assertions))
}
#[tauri::command]
async fn desktop_downloads_bind(window:WebviewWindow,state:tauri::State<'_,DownloadsState>,engine:tauri::State<'_,EngineState>,scope:crate::download_commands::ViewerScope)->Result<String>{
    caller(&window)?;
    let state=state.0.clone();
    let native=engine.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().ok().cloned();
    tauri::async_runtime::spawn_blocking(move||{
        let mut guard=state.lock().map_err(|_|Error::DownloadsUnavailable)?;let session=guard.as_mut().map_err(|e|*e)?;
        if session.binding_changes(&scope)?{retire_offline(session,native.as_ref())?;}
        session.bind(scope)
    }).await.map_err(|_|Error::DownloadsUnavailable)?
}
#[tauri::command]
async fn desktop_downloads_release(window:WebviewWindow,state:tauri::State<'_,DownloadsState>,lease:String)->Result<()>{
    caller(&window)?;
    let state=state.0.clone();
    tauri::async_runtime::spawn_blocking(move||{state.lock().map_err(|_|Error::DownloadsUnavailable)?.as_mut().map_err(|e|*e)?.release(&lease);Ok(())}).await.map_err(|_|Error::DownloadsUnavailable)?
}
#[tauri::command]
async fn desktop_downloads_command(window:WebviewWindow,state:tauri::State<'_,DownloadsState>,lease:String,operation:crate::download_commands::DownloadOperation)->Result<crate::download_commands::DownloadReply>{
    caller(&window)?;
    let state=state.0.clone();
    tauri::async_runtime::spawn_blocking(move||state.lock().map_err(|_|Error::DownloadsUnavailable)?.as_mut().map_err(|e|*e)?.execute(&lease,operation)).await.map_err(|_|Error::DownloadsUnavailable)?
}
#[derive(serde::Deserialize)]
#[serde(rename_all="camelCase",deny_unknown_fields)]
struct LocalLoad {local_source_id:String,generation:u64,start_position_ms:f64,autoplay:bool}
#[tauri::command]
async fn desktop_load_local(window:WebviewWindow,state:tauri::State<'_,DownloadsState>,engine:tauri::State<'_,EngineState>,lease:String,source:LocalLoad)->Result<()>{
    caller(&window)?;
    let downloads=state.0.clone();
    let native=engine.0.lock().map_err(|_|Error::EngineFailure)?.clone()?;
    tauri::async_runtime::spawn_blocking(move||{
        let mut guard=downloads.lock().map_err(|_|Error::DownloadsUnavailable)?;
        let session=guard.as_mut().map_err(|e|*e)?;
        let path=session.local_file(&lease,&source.local_source_id)?;
        let _=session.record_observation(&native.observation(),true);session.retire_local();
        native.load_local(&path,source.generation,source.start_position_ms,source.autoplay)?;
        if let Err(error)=session.begin_local(&lease,&source.local_source_id,source.generation){let _=native.execute(Command::Stop{generation:source.generation});return Err(error)}
        Ok(())
    }).await.map_err(|_|Error::DownloadsUnavailable)?
}
fn retire_offline(session:&mut crate::download_commands::DownloadsSession,native:Option<&Arc<Mpv>>)->Result<()>{
    if let (Some(generation),Some(native))=(session.active_generation(),native){
        if native.matches_generation(generation){let _=session.record_observation(&native.observation(),true);native.execute(Command::Stop{generation})?;}
    }
    session.retire_local();Ok(())
}
#[tauri::command]
async fn desktop_downloads_retire(window:WebviewWindow,state:tauri::State<'_,DownloadsState>,engine:tauri::State<'_,EngineState>,lease:String)->Result<()>{
    caller(&window)?;let downloads=state.0.clone();let native=engine.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().ok().cloned();
    tauri::async_runtime::spawn_blocking(move||{let mut guard=downloads.lock().map_err(|_|Error::DownloadsUnavailable)?;let session=guard.as_mut().map_err(|e|*e)?;if session.lease_current(&lease){retire_offline(session,native.as_ref())?;session.force_release(&lease);}Ok(())}).await.map_err(|_|Error::DownloadsUnavailable)?
}
#[tauri::command]
async fn desktop_command(window:WebviewWindow,state:tauri::State<'_,EngineState>,downloads:tauri::State<'_,DownloadsState>,command:Command)->Result<()> {
    caller(&window)?;
    let engine=state.0.lock().map_err(|_|Error::EngineFailure)?.clone()?;let downloads=downloads.0.clone();
    tauri::async_runtime::spawn_blocking(move||{
        let mut guard=downloads.lock().map_err(|_|Error::DownloadsUnavailable)?;
        if matches!(&command,Command::Load{..}|Command::Stop{..}){if let Ok(session)=guard.as_mut(){let _=session.record_observation(&engine.observation(),true);session.retire_local();}}
        engine.execute(command)
    }).await.map_err(|_|Error::EngineFailure)?
}
#[tauri::command]
async fn desktop_observe(window:WebviewWindow,state:tauri::State<'_,EngineState>,downloads:tauri::State<'_,DownloadsState>)->Result<Observation> {
    caller(&window)?;
    let engine=state.0.lock().map_err(|_|Error::EngineFailure)?.clone()?;let downloads=downloads.0.clone();
    tauri::async_runtime::spawn_blocking(move||{
        let observation=engine.observation();
        if let Ok(mut guard)=downloads.lock(){if let Ok(session)=guard.as_mut(){let _=session.record_observation(&observation,false);}}
        Ok(observation)
    }).await.map_err(|_|Error::EngineFailure)?
}
#[tauri::command]
fn desktop_audio_facts(window:WebviewWindow,state:tauri::State<EngineState>)->Result<crate::audio::AudioFacts> {
    caller(&window)?;
    Ok(state.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().map_err(|e|*e)?.audio_facts())
}
#[tauri::command]
fn desktop_equalizer(window:WebviewWindow,equalizer:tauri::State<EqualizerState>,engine:tauri::State<EngineState>)->Result<EqualizerSnapshot> {
    caller(&window)?;
    let store=equalizer.0.lock().map_err(|_|Error::PreferencesUnavailable)?;
    let saved=store.as_ref().map_err(|error|*error)?.read()?;
    let applied=engine.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().map(|native|native.equalizer_matches(&saved.settings)).unwrap_or(false);
    Ok(EqualizerSnapshot{revision:saved.revision,settings:saved.settings,applied})
}
#[tauri::command]
fn desktop_equalizer_change(window:WebviewWindow,equalizer:tauri::State<EqualizerState>,engine:tauri::State<EngineState>,change:EqualizerChange)->Result<EqualizerSnapshot> {
    caller(&window)?;
    let mut store=equalizer.0.lock().map_err(|_|Error::PreferencesUnavailable)?;
    let native=engine.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().ok().cloned();
    let saved=store.as_mut().map_err(|error|*error)?.change(change,|settings|native.as_ref().map(|player|player.apply_equalizer(settings)).unwrap_or(Ok(())))?;
    let applied=native.as_ref().map(|player|player.equalizer_matches(&saved.settings)).unwrap_or(false);
    Ok(EqualizerSnapshot{revision:saved.revision,settings:saved.settings,applied})
}
#[tauri::command]
fn desktop_window_state(window:WebviewWindow,state:tauri::State<Arc<crate::window::ChromeState>>)->Result<crate::window::WindowSnapshot>{
    caller(&window)?;crate::window::snapshot(&window,&state)
}
#[tauri::command]
fn desktop_window_action(window:WebviewWindow,action:crate::window::WindowAction)->Result<()>{
    caller(&window)?;crate::window::action(&window,action)
}
#[tauri::command]
async fn desktop_window_chrome(window:WebviewWindow,state:tauri::State<'_,Arc<crate::window::ChromeState>>,chrome:crate::window::ChromeUpdate)->Result<()>{
    caller(&window)?;
    let native=window.clone();let state=state.inner().clone();
    let (send,receive)=std::sync::mpsc::sync_channel(1);
    window.run_on_main_thread(move||{let result=crate::window::update(&native,state,chrome);let _=send.send(result);}).map_err(|_|Error::WindowUnavailable)?;
    tauri::async_runtime::spawn_blocking(move||receive.recv().unwrap_or(Err(Error::WindowUnavailable))).await.map_err(|_|Error::WindowUnavailable)?
}
#[tauri::command]
fn desktop_surface(window:WebviewWindow,state:tauri::State<EngineState>,bounds:crate::boundary::SurfaceBounds,generation:u64)->Result<()> {
    caller(&window)?;bounds.validate()?;
    let engine=state.0.lock().map_err(|_|Error::EngineFailure)?.clone()?;
    if !engine.matches_generation(generation){return Err(Error::StaleGeneration);}
    window.with_webview(move|webview|{if engine.matches_generation(generation){let ready=surface::bounds(&webview,bounds).is_ok()&&bounds.width>0.0&&bounds.height>0.0;engine.set_geometry_ready(ready);}}).map_err(|_|Error::EngineFailure)?;
    Ok(())
}

#[derive(serde::Deserialize)]
#[serde(rename_all="camelCase",deny_unknown_fields)]
struct UiProof{frames:u64,elapsed_ms:f64,longest_frame_ms:f64,width:u32,height:u32}
#[tauri::command]
fn desktop_spike_report(window:WebviewWindow,state:tauri::State<EngineState>,report:UiProof)->Result<()> {
    caller(&window)?;
    if !cfg!(debug_assertions)||report.frames>10000||!report.elapsed_ms.is_finite()||!report.longest_frame_ms.is_finite(){return Err(Error::InvalidCommand);}
    #[cfg(debug_assertions)]
    if let Some(path)=std::env::var_os("PORTICO_SPIKE_REPORT"){
        let engine=state.0.lock().map_err(|_|Error::EngineFailure)?;
        let (observation,audio)=match engine.as_ref(){Ok(e)=>(Some(e.observation()),Some(e.audio_facts())),Err(_)=>(None,None)};
        let value=serde_json::json!({"platform":std::env::consts::OS,"ui":{"frames":report.frames,"elapsedMs":report.elapsed_ms,"longestFrameMs":report.longest_frame_ms,"width":report.width,"height":report.height},"native":surface::proof(),"player":observation,"audio":audio});
        let path=PathBuf::from(path);let temp=path.with_extension("tmp");
        std::fs::write(&temp,serde_json::to_vec_pretty(&value).map_err(|_|Error::EngineFailure)?).map_err(|_|Error::EngineFailure)?;
        std::fs::rename(temp,path).map_err(|_|Error::EngineFailure)?;
    }
    Ok(())
}

fn library(app:&tauri::AppHandle)->Result<PathBuf> {
    // Debug proof only; release always loads its own native resource. No web-selected library.
    if cfg!(debug_assertions) {if let Some(path)=std::env::var_os("PORTICO_MPV_LIBRARY") {return Ok(path.into());}}
    let name=if cfg!(target_os="windows"){"mpv-2.dll"}else if cfg!(target_os="macos"){"libmpv.2.dylib"}else{"libmpv.so.2"};
    Ok(app.path().resource_dir().map_err(|_|Error::EngineUnavailable)?.join("native").join(name))
}

pub fn run() {
    let builder=tauri::Builder::default()
        .manage(EngineState(Mutex::new(Err(Error::EngineUnavailable))))
        .manage(EqualizerState(Mutex::new(Err(Error::PreferencesUnavailable))))
        .manage(DownloadsState(Arc::new(Mutex::new(Err(Error::DownloadsUnavailable)))))
        .manage(Arc::new(crate::window::ChromeState::default()))
        .invoke_handler(tauri::generate_handler![desktop_command,desktop_observe,desktop_audio_facts,desktop_surface,desktop_spike_report,desktop_equalizer,desktop_equalizer_change,desktop_window_state,desktop_window_action,desktop_window_chrome,desktop_downloads_bind,desktop_downloads_release,desktop_downloads_command,desktop_load_local,desktop_downloads_retire])
        .setup(|app| {
            // app_local_data_dir selects LocalAppData on Windows, not roaming/account state.
            let preference_error=||std::io::Error::other("device_preferences_unavailable");
            let path=app.path().app_local_data_dir().map_err(|_|preference_error())?.join("equalizer.sqlite3");
            let store=EqualizerStore::open(&path).map_err(|_|preference_error())?;
            *app.state::<EqualizerState>().0.lock().map_err(|_|preference_error())?=Ok(store);
            let downloads=app.path().app_local_data_dir().map_err(|_|preference_error())?.join("offline");
            *app.state::<DownloadsState>().0.lock().map_err(|_|preference_error())?=crate::download_commands::DownloadsSession::open(&downloads);
            let main=tauri::WebviewWindowBuilder::new(app,"main",tauri::WebviewUrl::App("index.html".into()))
                .title("Portico").inner_size(1440.0,900.0).min_inner_size(600.0,600.0)
                // Linux uses the desktop's title bar. Windows keeps OS controls until the
                // installed web caption host supplies its actual control rectangles.
                .decorations(true)
                .transparent(true)
                .on_navigation(|url|boundary::local_origin("main",url.as_str(),cfg!(debug_assertions)).is_ok())
                .on_page_load(|window,payload| {
                    if cfg!(debug_assertions)&&std::env::var_os("PORTICO_SPIKE_REPORT").is_some()&&payload.event()==tauri::webview::PageLoadEvent::Finished {
                        let _=window.eval(r#"setTimeout(()=>{let frames=0,start=performance.now(),last=start,longest=0;function measure(now){longest=Math.max(longest,now-last);last=now;frames++;if(now-start<5000)requestAnimationFrame(measure);else window.__TAURI_INTERNALS__.invoke('desktop_spike_report',{report:{frames,elapsedMs:now-start,longestFrameMs:longest,width:innerWidth,height:innerHeight}})}requestAnimationFrame(measure)},3000)"#);
                    }
                });
            let main=main.build()?;
            let handle=app.handle().clone();
            let lib=library(app.handle()).and_then(|p|Api::open(&p));
            main.with_webview(move |webview| {
                // Keep the same store->engine lock order as preference commands. A setting
                // saved while the webview starts must not be replaced by an earlier copy.
                let equalizer_state=handle.state::<EqualizerState>();
                let preferences=equalizer_state.0.lock();
                let result=(||->Result<Arc<Mpv>>{
                    let store=preferences.as_ref().map_err(|_|Error::PreferencesUnavailable)?;
                    let equalizer=store.as_ref().map_err(|error|*error)?.read()?.settings;
                    let native=lib.and_then(|api|surface::attach(&webview,api,&AudioOutput::default()))?;
                    native.apply_equalizer(&equalizer)?;Ok(native)
                })();
                #[cfg(debug_assertions)]
                if let Ok(engine)=&result {if let Some(path)=std::env::var_os("PORTICO_SPIKE_MEDIA"){
                    let ready=surface::bounds(&webview,crate::boundary::SurfaceBounds{x:0.0,y:0.0,width:1440.0,height:900.0}).is_ok();
                    engine.set_geometry_ready(ready);let _=engine.load_spike_fixture(&PathBuf::from(path));
                }}
                *handle.state::<EngineState>().0.lock().unwrap_or_else(|e|e.into_inner())=result;
            })?;
            #[cfg(feature="updater")]
            {
                // The public trust root and real endpoint are release inputs, not placeholders.
                let key=option_env!("PORTICO_DESKTOP_UPDATER_PUBLIC_KEY").ok_or("updater public key missing")?;
                let endpoint=option_env!("PORTICO_DESKTOP_UPDATER_ENDPOINT").ok_or("updater endpoint missing")?;
                if !endpoint.starts_with("https://") {return Err("updater endpoint must use HTTPS".into());}
                app.handle().plugin(tauri_plugin_updater::Builder::new().pubkey(key).endpoints(vec![endpoint.parse()?])?.build())?;
            }
            Ok(())
        });
    // Error text from native libraries is deliberately not printed.
    if builder.run(tauri::generate_context!()).is_err(){eprintln!("desktop_start_failed");}
}
