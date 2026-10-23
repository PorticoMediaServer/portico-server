use crate::{audio::AudioOutput,boundary::{self,Command,Error,Observation,Result},mpv::{Api,Mpv}};
use std::{path::PathBuf,sync::{Arc,Mutex}};
use tauri::{Manager,WebviewWindow};

#[cfg(target_os="linux")]
#[path="surface_linux.rs"] mod surface;
#[cfg(target_os="macos")]
#[path="surface_macos.rs"] mod surface;
#[cfg(target_os="windows")]
#[path="surface_windows.rs"] mod surface;

pub struct EngineState(pub Mutex<std::result::Result<Arc<Mpv>,Error>>);
fn caller(window:&WebviewWindow)->Result<()> {
    boundary::local_origin(window.label(),window.url().map_err(|_|Error::ForbiddenOrigin)?.as_str(),cfg!(debug_assertions))
}
#[tauri::command]
fn desktop_command(window:WebviewWindow,state:tauri::State<EngineState>,command:Command)->Result<()> {
    caller(&window)?;
    let engine=state.0.lock().map_err(|_|Error::EngineFailure)?.clone()?;
    engine.execute(command)
}
#[tauri::command]
fn desktop_observe(window:WebviewWindow,state:tauri::State<EngineState>)->Result<Observation> {
    caller(&window)?;
    Ok(state.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().map_err(|e|*e)?.observation())
}
#[tauri::command]
fn desktop_audio_facts(window:WebviewWindow,state:tauri::State<EngineState>)->Result<crate::audio::AudioFacts> {
    caller(&window)?;
    Ok(state.0.lock().map_err(|_|Error::EngineFailure)?.as_ref().map_err(|e|*e)?.audio_facts())
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
        .invoke_handler(tauri::generate_handler![desktop_command,desktop_observe,desktop_audio_facts,desktop_surface,desktop_spike_report])
        .setup(|app| {
            let main=tauri::WebviewWindowBuilder::new(app,"main",tauri::WebviewUrl::App("index.html".into()))
                .title("Portico").inner_size(1440.0,900.0).min_inner_size(360.0,480.0)
                .transparent(true)
                .on_navigation(|url|boundary::local_origin("main",url.as_str(),cfg!(debug_assertions)).is_ok())
                .on_page_load(|window,payload| {
                    if cfg!(debug_assertions)&&std::env::var_os("PORTICO_SPIKE_REPORT").is_some()&&payload.event()==tauri::webview::PageLoadEvent::Finished {
                        let _=window.eval(r#"setTimeout(()=>{let frames=0,start=performance.now(),last=start,longest=0;function measure(now){longest=Math.max(longest,now-last);last=now;frames++;if(now-start<5000)requestAnimationFrame(measure);else window.__TAURI_INTERNALS__.invoke('desktop_spike_report',{report:{frames,elapsedMs:now-start,longestFrameMs:longest,width:innerWidth,height:innerHeight}})}requestAnimationFrame(measure)},3000)"#);
                    }
                })
                .build()?;
            let handle=app.handle().clone();
            let lib=library(app.handle()).and_then(|p|Api::open(&p));
            main.with_webview(move |webview| {
                let result=lib.and_then(|api|surface::attach(&webview,api,&AudioOutput::default()));
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
