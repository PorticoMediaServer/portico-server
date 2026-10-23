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
async fn desktop_downloads_command(window:WebviewWindow,state:tauri::State<'_,DownloadsState>,engine:tauri::State<'_,EngineState>,lease:String,operation:crate::download_commands::DownloadOperation)->Result<crate::download_commands::DownloadReply>{
    caller(&window)?;
    let state=state.0.clone();
    let native=engine.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().ok().cloned();
    tauri::async_runtime::spawn_blocking(move||{
        let mut guard=state.lock().map_err(|_|Error::DownloadsUnavailable)?;let session=guard.as_mut().map_err(|e|*e)?;
        let can_remove=matches!(&operation,crate::download_commands::DownloadOperation::Revoke{..}|crate::download_commands::DownloadOperation::Remove{..}|crate::download_commands::DownloadOperation::GroupAction{..});
        if can_remove {if let Some(native)=native.as_ref(){let _=session.record_observation(&native.observation(),true);}}
        let reply=session.execute(&lease,operation)?;
        if can_remove&&!session.active_file_valid(){retire_offline(session,native.as_ref())?;}
        Ok(reply)
    }).await.map_err(|_|Error::DownloadsUnavailable)?
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
async fn desktop_audio_facts(window:WebviewWindow,state:tauri::State<'_,EngineState>)->Result<crate::audio::AudioFacts> {
    caller(&window)?;
    let native=state.0.lock().map_err(|_|Error::EngineFailure)?.clone()?;
    tauri::async_runtime::spawn_blocking(move||Ok(native.audio_facts())).await.map_err(|_|Error::EngineFailure)?
}
#[tauri::command]
async fn desktop_equalizer(window:WebviewWindow)->Result<EqualizerSnapshot> {
    caller(&window)?;
    let app=window.app_handle().clone();
    tauri::async_runtime::spawn_blocking(move||{
        let equalizer=app.state::<EqualizerState>();
        let store=equalizer.0.lock().map_err(|_|Error::PreferencesUnavailable)?;
        let saved=store.as_ref().map_err(|error|*error)?.read()?;
        let engine=app.state::<EngineState>();
        let native=engine.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().ok().cloned();
        let applied=native.as_ref().map(|player|player.equalizer_matches(&saved.settings)).unwrap_or(false);
        Ok(EqualizerSnapshot{revision:saved.revision,settings:saved.settings,applied})
    }).await.map_err(|_|Error::PreferencesUnavailable)?
}
#[tauri::command]
async fn desktop_equalizer_change(window:WebviewWindow,change:EqualizerChange)->Result<EqualizerSnapshot> {
    caller(&window)?;
    let app=window.app_handle().clone();
    tauri::async_runtime::spawn_blocking(move||{
        // Resolve the renderer only after taking the store lock, as initialization does.
        // A request queued before initialization must apply to the renderer now present.
        let equalizer=app.state::<EqualizerState>();
        let mut store=equalizer.0.lock().map_err(|_|Error::PreferencesUnavailable)?;
        let engine=app.state::<EngineState>();
        let native=engine.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().ok().cloned();
        let saved=store.as_mut().map_err(|error|*error)?.change(change,|settings|native.as_ref().map(|player|player.apply_equalizer(settings)).unwrap_or(Ok(())))?;
        let applied=native.as_ref().map(|player|player.equalizer_matches(&saved.settings)).unwrap_or(false);
        Ok(EqualizerSnapshot{revision:saved.revision,settings:saved.settings,applied})
    }).await.map_err(|_|Error::PreferencesUnavailable)?
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
async fn desktop_surface(window:WebviewWindow,state:tauri::State<'_,EngineState>,bounds:crate::boundary::SurfaceBounds,generation:u64)->Result<()> {
    caller(&window)?;bounds.validate()?;
    let engine=state.0.lock().map_err(|_|Error::EngineFailure)?.clone()?;
    if !engine.matches_generation(generation){return Err(Error::StaleGeneration);}
    let (send,receive)=std::sync::mpsc::sync_channel(1);
    window.with_webview(move|webview|{
        let result=if engine.matches_generation(generation){let result=surface::bounds(&webview,bounds);engine.set_geometry_ready(result.is_ok()&&bounds.width>0.0&&bounds.height>0.0);result}else{Err(Error::StaleGeneration)};
        let _=send.send(result);
    }).map_err(|_|Error::EngineFailure)?;
    tauri::async_runtime::spawn_blocking(move||receive.recv().unwrap_or(Err(Error::EngineFailure))).await.map_err(|_|Error::EngineFailure)?
}

#[derive(serde::Deserialize)]
#[serde(rename_all="camelCase",deny_unknown_fields)]
struct UiProof{frames:u64,elapsed_ms:f64,longest_frame_ms:f64,width:u32,height:u32}
#[tauri::command]
async fn desktop_spike_report(window:WebviewWindow,state:tauri::State<'_,EngineState>,report:UiProof)->Result<()> {
    caller(&window)?;
    if !cfg!(debug_assertions)||report.frames>10000||!report.elapsed_ms.is_finite()||!report.longest_frame_ms.is_finite(){return Err(Error::InvalidCommand);}
    #[cfg(debug_assertions)]
    if let Some(path)=std::env::var_os("PORTICO_SPIKE_REPORT"){
        let engine=state.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().ok().cloned();
        let (send,receive)=std::sync::mpsc::sync_channel(1);window.with_webview(move|_|{let _=send.send(surface::proof());}).map_err(|_|Error::EngineFailure)?;
        let value=tauri::async_runtime::spawn_blocking(move||{
            let native=receive.recv().map_err(|_|Error::EngineFailure)?;
            let (observation,audio)=match engine.as_ref(){Some(e)=>(Some(e.observation()),Some(e.audio_facts())),None=>(None,None)};
            Ok(serde_json::json!({"platform":std::env::consts::OS,"ui":{"frames":report.frames,"elapsedMs":report.elapsed_ms,"longestFrameMs":report.longest_frame_ms,"width":report.width,"height":report.height},"native":native,"player":observation,"audio":audio}))
        }).await.map_err(|_|Error::EngineFailure)??;
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
fn local_data(app:&tauri::AppHandle)->Result<PathBuf>{
    #[cfg(debug_assertions)]
    if let Some(path)=std::env::var_os("PORTICO_SPIKE_DATA"){
        let path=PathBuf::from(path);if !path.is_absolute(){return Err(Error::InvalidCommand)}return Ok(path)
    }
    app.path().app_local_data_dir().map_err(|_|Error::PreferencesUnavailable)
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
            let data=local_data(app.handle()).map_err(|_|preference_error())?;
            let path=data.join("equalizer.sqlite3");
            let store=EqualizerStore::open(&path).map_err(|_|preference_error())?;
            *app.state::<EqualizerState>().0.lock().map_err(|_|preference_error())?=Ok(store);
            let downloads=data.join("offline");
            *app.state::<DownloadsState>().0.lock().map_err(|_|preference_error())?=crate::download_commands::DownloadsSession::open(&downloads);
            let main=tauri::WebviewWindowBuilder::new(app,"main",tauri::WebviewUrl::App("index.html".into()))
                .title("Portico").inner_size(1440.0,900.0).min_inner_size(600.0,600.0)
                // Linux uses the desktop's title bar. Windows keeps OS controls until the
                // installed web caption host supplies its actual control rectangles.
                .decorations(true)
                .incognito(cfg!(debug_assertions)&&std::env::var_os("PORTICO_SPIKE_DATA").is_some())
                .transparent(true)
                .on_navigation(|url|boundary::local_origin("main",url.as_str(),cfg!(debug_assertions)).is_ok())
                .on_page_load(|window,payload| {
                    crate::spike::record(if payload.event()==tauri::webview::PageLoadEvent::Finished{"page-finished"}else{"page-started"},0);
                    if cfg!(debug_assertions)&&std::env::var_os("PORTICO_SPIKE_REPORT").is_some()&&payload.event()==tauri::webview::PageLoadEvent::Finished {
                        if std::env::var_os("PORTICO_SPIKE_MEDIA").is_some(){let _=window.eval(r#"window.__porticoNativeProbe=setInterval(()=>window.__TAURI_INTERNALS__.invoke('desktop_observe').then(o=>o.generation?window.__TAURI_INTERNALS__.invoke('desktop_surface',{generation:o.generation,bounds:{x:0,y:0,width:innerWidth,height:innerHeight}}):undefined).catch(()=>{}),200);setTimeout(()=>clearInterval(window.__porticoNativeProbe),9000)"#);}
                        let _=window.eval(r#"setTimeout(()=>{let frames=0,start=performance.now(),last=start,longest=0;function measure(now){longest=Math.max(longest,now-last);last=now;frames++;if(now-start<5000)requestAnimationFrame(measure);else window.__TAURI_INTERNALS__.invoke('desktop_spike_report',{report:{frames,elapsedMs:now-start,longestFrameMs:longest,width:innerWidth,height:innerHeight}})}requestAnimationFrame(measure)},3000)"#);
                    }
                });
            let main=main.build()?;
            let handle=app.handle().clone();
            let lib=library(app.handle()).and_then(|p|Api::open(&p));
            #[cfg(target_os="macos")]
            {
                let window=main.clone();
                tauri::async_runtime::spawn_blocking(move||{
                    crate::spike::record("worker-started",0);
                    // macvk initializes Cocoa through DispatchQueue.main.sync. Never wait for
                    // its client API on AppKit's main thread, including the first VO load.
                    let native=lib.and_then(|api|Mpv::new(api,&AudioOutput::default(),crate::mpv::VideoTarget::NativeView(0)));
                    crate::spike::record("worker-engine",if native.is_ok(){0}else{-1});
                    let equalizer=handle.state::<EqualizerState>();let preferences=equalizer.0.lock();
                    let result=native.and_then(|native|{
                        let store=preferences.as_ref().map_err(|_|Error::PreferencesUnavailable)?.as_ref().map_err(|e|*e)?;
                        native.apply_equalizer(&store.read()?.settings)?;Ok(native)
                    });
                    *handle.state::<EngineState>().0.lock().unwrap_or_else(|e|e.into_inner())=result.clone();
                    drop(preferences);
                    if let Ok(native)=result{
                        let attached=native.clone();
                        let _=window.with_webview(move|webview|{let _=surface::attach_existing(&webview,&attached);});
                        #[cfg(debug_assertions)]
                        if let Some(path)=std::env::var_os("PORTICO_SPIKE_MEDIA"){let _=native.load_spike_fixture(&PathBuf::from(path));}
                    }
                });
            }
            #[cfg(not(target_os="macos"))]
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
                    engine.set_geometry_ready(ready);
                    let native=engine.clone();
                    // Client API calls may wait for the GL renderer. Never make them on
                    // the thread that owns and services that render context.
                    tauri::async_runtime::spawn_blocking(move||{let _=native.load_spike_fixture(&PathBuf::from(path));});
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
