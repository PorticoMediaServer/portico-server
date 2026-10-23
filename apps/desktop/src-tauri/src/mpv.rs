//! The C client API is the only native engine boundary. No shell or generic command is exported.
use crate::audio::{AudioFacts, AudioOutput};
use crate::boundary::{Command, Error, Observation, Result, Source};
use crate::equalizer::{EqualizerSettings, FILTER_LABEL};
use libloading::Library;
use std::ffi::{c_char, c_int, c_void, CStr, CString};
use std::path::Path;
use std::ptr;
use std::sync::{Arc, Mutex,atomic::{AtomicBool,AtomicU64,Ordering}};

type Handle = *mut c_void;
type Create = unsafe extern "C" fn() -> Handle;
type Initialize = unsafe extern "C" fn(Handle) -> c_int;
type Destroy = unsafe extern "C" fn(Handle);
type Set = unsafe extern "C" fn(Handle, *const c_char, *const c_char) -> c_int;
type Cmd = unsafe extern "C" fn(Handle, *const *const c_char) -> c_int;
type Get = unsafe extern "C" fn(Handle, *const c_char) -> *mut c_char;
type Free = unsafe extern "C" fn(*mut c_void);
type ApiVersion = unsafe extern "C" fn() -> u64;
#[repr(C)]struct Event{id:c_int,error:c_int,reply:u64,data:*mut c_void}
type Wait=unsafe extern "C" fn(Handle,f64)->*const Event;

pub struct Api {
    _library: Library,
    create: Create,
    initialize: Initialize,
    destroy: Destroy,
    option: Set,
    property: Set,
    command: Cmd,
    get: Get,
    free: Free,
    wait: Wait,
    pub version: u64,
}
impl Api {
    /// `path` comes from the native launcher/package, never from a web command.
    pub fn open(path: &Path) -> Result<Arc<Self>> {
        unsafe {
            let lib = Library::new(path).map_err(|_| Error::EngineUnavailable)?;
            macro_rules! symbol { ($name:literal, $t:ty) => { *lib.get::<$t>(concat!($name,"\0").as_bytes()).map_err(|_| Error::EngineUnavailable)? }; }
            let version = symbol!("mpv_client_api_version", ApiVersion)();
            if version >> 16 != 2 { return Err(Error::EngineUnavailable); }
            Ok(Arc::new(Self { create:symbol!("mpv_create",Create), initialize:symbol!("mpv_initialize",Initialize),
                destroy:symbol!("mpv_terminate_destroy",Destroy), option:symbol!("mpv_set_option_string",Set),
                property:symbol!("mpv_set_property_string",Set), command:symbol!("mpv_command",Cmd),
                get:symbol!("mpv_get_property_string",Get), free:symbol!("mpv_free",Free),wait:symbol!("mpv_wait_event",Wait), _library:lib,version }))
        }
    }
    pub unsafe fn symbol<T: Copy>(&self, name: &[u8]) -> Result<T> {
        unsafe { self._library.get::<T>(name).map(|x|*x).map_err(|_|Error::EngineUnavailable) }
    }
}

#[derive(Clone, Copy)]
pub enum VideoTarget { Headless, OpenGl, NativeView(usize) }
pub struct Mpv {
    pub api: Arc<Api>,
    handle: Handle,
    generation: Mutex<u64>,
    exclusive: bool,
    loaded:AtomicBool,
    surface_ready:AtomicBool,
    frame_drawn:AtomicBool,
    geometry_ready:AtomicBool,
    expected_url:Mutex<String>,
    failed:AtomicBool,
    events:Mutex<()>,
    newest_generation:AtomicU64,
    equalizer:Mutex<EqualizerSettings>,
    equalizer_applied:AtomicBool,
    configured_spdif:String,
}
// libmpv explicitly permits client-API calls from any thread; the render context is separately
// owned by the GTK/main thread. The handle outlives every context through Arc<Mpv>.
unsafe impl Send for Mpv {}
unsafe impl Sync for Mpv {}
fn c(value: &str) -> Result<CString> { CString::new(value).map_err(|_|Error::InvalidCommand) }
fn status(value: c_int) -> Result<()> { if value < 0 { Err(Error::EngineFailure) } else { Ok(()) } }

impl Mpv {
    pub fn new(api: Arc<Api>, output: &AudioOutput, target: VideoTarget) -> Result<Arc<Self>> {
        let handle = unsafe { (api.create)() };
        if handle.is_null() { return Err(Error::EngineUnavailable); }
        let engine=Arc::new(Self {api,handle,generation:Mutex::new(0),exclusive:output.exclusive,loaded:AtomicBool::new(false),surface_ready:AtomicBool::new(false),frame_drawn:AtomicBool::new(false),geometry_ready:AtomicBool::new(false),expected_url:Mutex::new(String::new()),failed:AtomicBool::new(false),events:Mutex::new(()),newest_generation:AtomicU64::new(0),equalizer:Mutex::new(EqualizerSettings::default()),equalizer_applied:AtomicBool::new(true),configured_spdif:output.passthrough.join(",")});
        for (key,value) in [("config","no"),("load-scripts","no"),("ytdl","no"),("osc","no"),
            ("input-default-bindings","no"),("input-vo-keyboard","no"),("terminal","no"),("msg-level","all=no"),
            ("idle","yes"),("keep-open","yes"),("access-references","no"),("load-unsafe-playlists","no"),
            ("demuxer-lavf-o","protocol_whitelist=[http,https,tcp,tls,crypto]"),("hwdec","auto-safe"),
            ("audio-client-name","Portico"),("sub-auto","no"),("audio-file-auto","no"),("autoload-files","no")] {
            engine.option(key,value)?;
        }
        for (key,value) in output.options(std::env::consts::OS)? { engine.option(key,&value)?; }
        match target {
            VideoTarget::Headless => {engine.option("vo","null")?;engine.option("ao","null")?;}
            VideoTarget::OpenGl => {engine.option("vo","libmpv")?;}
            VideoTarget::NativeView(parent) => {
                engine.option("vo","gpu-next")?;engine.option("wid",&parent.to_string())?;
                #[cfg(target_os="macos")]
                {engine.option("gpu-api","vulkan")?;engine.option("gpu-context","macvk")?;engine.option("target-colorspace-hint","yes")?;}
                #[cfg(target_os="windows")]
                {engine.option("gpu-api","d3d11")?;engine.option("gpu-context","d3d11")?;
                 engine.option("d3d11-output-mode","composition")?;engine.option("d3d11-output-csp","auto")?;}
            }
        }
        status(unsafe {(engine.api.initialize)(handle)})?;
        Ok(engine)
    }
    fn option(&self,name:&str,value:&str)->Result<()> {status(unsafe{(self.api.option)(self.handle,c(name)?.as_ptr(),c(value)?.as_ptr())})}
    fn set(&self,name:&str,value:&str)->Result<()> {status(unsafe{(self.api.property)(self.handle,c(name)?.as_ptr(),c(value)?.as_ptr())})}
    fn command(&self,args:&[&str])->Result<()> {
        let values=args.iter().map(|x|c(x)).collect::<Result<Vec<_>>>()?;
        let mut pointers=values.iter().map(|x|x.as_ptr()).collect::<Vec<_>>();pointers.push(ptr::null());
        status(unsafe{(self.api.command)(self.handle,pointers.as_ptr())})
    }
    pub fn property(&self,name:&str)->Option<String> {
        let key=c(name).ok()?;
        unsafe {let p=(self.api.get)(self.handle,key.as_ptr());if p.is_null(){return None;}
            let value=CStr::from_ptr(p).to_string_lossy().into_owned();(self.api.free)(p.cast());Some(value)}
    }
    fn number(&self,name:&str)->Option<f64> {self.property(name)?.parse().ok().filter(|x:&f64|x.is_finite())}
    fn flag(&self,name:&str)->bool {self.property(name).as_deref()==Some("yes")}
    pub fn handle(&self)->Handle {self.handle}
    /// App-wide DSP. No output identity participates in lookup or persistence. The labelled
    /// filter survives source changes and is shared by music/video and every physical route.
    pub fn apply_equalizer(&self,next:&EqualizerSettings)->Result<()> {
        next.validate()?;
        let _transport=self.generation.lock().map_err(|_|Error::EngineFailure)?;
        let mut previous=self.equalizer.lock().map_err(|_|Error::EngineFailure)?;
        // A failed rollback leaves an unknown DSP chain. Keep playback fenced until the
        // engine is recreated from the persisted setting rather than guessing its state.
        if !self.equalizer_applied.load(Ordering::Acquire){return Err(Error::EngineFailure);}
        if *previous==*next{return Ok(());}
        self.equalizer_applied.store(false,Ordering::Release);
        if self.write_equalizer(&previous,next).is_err(){
            let restored=self.write_equalizer(next,&previous).is_ok();
            self.equalizer_applied.store(restored,Ordering::Release);
            if !restored{let _=self.set("pause","yes");}
            return Err(Error::EngineFailure);
        }
        *previous=next.clone();self.equalizer_applied.store(true,Ordering::Release);Ok(())
    }
    fn write_equalizer(&self,previous:&EqualizerSettings,next:&EqualizerSettings)->Result<()> {
        if let Some(filter)=next.filter()? {
            // Bitstream cannot pass through an equalizer. Enabling EQ requires decoded PCM.
            self.set("audio-spdif","")?;
            self.command(&["af","add",&filter])?;
        } else {
            if previous.enabled{
                self.command(&["af","remove",&format!("@{FILTER_LABEL}")])?;
                self.set("audio-spdif",&self.configured_spdif)?;
            }
        }
        Ok(())
    }
    pub fn equalizer_matches(&self,settings:&EqualizerSettings)->bool {
        self.equalizer.lock().map(|current|self.equalizer_applied.load(Ordering::Acquire)&&*current==*settings).unwrap_or(false)
    }
    pub fn execute(&self,command:Command)->Result<()> {
        // Hold generation through the native command; stale async work can never affect the next source.
        let mut generation=self.generation.lock().map_err(|_|Error::EngineFailure)?;
        match command {
            Command::Load{source} => {
                if !self.equalizer_applied.load(Ordering::Acquire){return Err(Error::EngineFailure);}
                source.validate()?;
                if source.generation<=self.newest_generation.load(Ordering::Acquire){return Err(Error::StaleGeneration);}
                self.newest_generation.store(source.generation,Ordering::Release);
                self.drain();self.loaded.store(false,Ordering::Release);self.frame_drawn.store(false,Ordering::Release);self.failed.store(false,Ordering::Release);
                *self.expected_url.lock().map_err(|_|Error::EngineFailure)?=source.url.clone();
                self.load(&source)?;*generation=source.generation;Ok(())
            }
            other => {
                let g=match &other {Command::Play{generation}|Command::Pause{generation}|Command::Seek{generation,..}|Command::Rate{generation,..}|Command::Volume{generation,..}|Command::Stop{generation}=>*generation,_=>unreachable!()};
                if *generation==0 || *generation!=g {return Err(Error::StaleGeneration);}
                match other {
                    Command::Play{..} if self.equalizer_applied.load(Ordering::Acquire)=>self.set("pause","no"),
                    Command::Play{..}=>Err(Error::EngineFailure),Command::Pause{..}=>self.set("pause","yes"),
                    Command::Seek{milliseconds,..} if milliseconds.is_finite()&&(0.0..=1e12).contains(&milliseconds)=>self.command(&["seek",&(milliseconds/1000.0).to_string(),"absolute+exact"]),
                    Command::Rate{rate,..} if rate.is_finite()&&(0.25..=4.0).contains(&rate)=>self.set("speed",&rate.to_string()),
                    Command::Volume{volume,muted,..} if volume.is_finite()&&(0.0..=1.0).contains(&volume)=>{self.set("volume",&(volume*100.0).to_string())?;self.set("mute",if muted{"yes"}else{"no"})},
                    Command::Stop{..}=>{self.command(&["stop"])?;*generation=0;self.loaded.store(false,Ordering::Release);self.frame_drawn.store(false,Ordering::Release);Ok(())},
                    _=>Err(Error::InvalidCommand)
                }
            }
        }
    }
    fn load(&self,source:&Source)->Result<()> {
        self.set("pause",if source.autoplay{"no"}else{"yes"})?;
        // loadfile argv is a null-terminated vector, never parsed as a command string.
        self.command(&["loadfile",&source.url,"replace","-1",&format!("start={}",source.start_position_ms/1000.0)])
    }
    #[cfg(debug_assertions)]
    pub fn load_spike_fixture(&self,path:&Path)->Result<()> {
        if !path.is_absolute() || !path.is_file(){return Err(Error::InvalidSource);}
        *self.expected_url.lock().map_err(|_|Error::EngineFailure)?=path.to_str().ok_or(Error::InvalidSource)?.into();
        self.command(&["loadfile",path.to_str().ok_or(Error::InvalidSource)?,"replace"])?;
        self.set("loop-file","inf")?;
        *self.generation.lock().map_err(|_|Error::EngineFailure)?=1;Ok(())
    }
    pub fn observation(&self)->Observation {
        let generation=*self.generation.lock().unwrap_or_else(|x|x.into_inner());
        self.drain();
        let state=if generation==0{"idle"}else if self.failed.load(Ordering::Acquire){"error"}else if self.flag("eof-reached"){"ended"}else if !self.loaded.load(Ordering::Acquire)||self.flag("idle-active"){"loading"}
            else if self.flag("paused-for-cache"){"buffering"}else if self.flag("pause"){"paused"}else{"playing"};
        Observation{generation,state:state.into(),position_ms:self.number("time-pos").unwrap_or(0.0).max(0.0)*1000.0,
            rate:self.number("speed").unwrap_or(1.0),volume:self.number("volume").unwrap_or(100.0)/100.0,
            muted:self.flag("mute"),dropped_frames:self.number("frame-drop-count").unwrap_or(0.0) as u64,
            native_surface_active:!matches!(state,"idle"|"loading"|"ended"|"error")&&self.surface_ready.load(Ordering::Acquire)&&self.geometry_ready.load(Ordering::Acquire)&&self.frame_drawn.load(Ordering::Acquire)&&self.loaded.load(Ordering::Acquire)&&self.number("width").unwrap_or(0.0)>0.0}
    }
    fn drain(&self){
        let _lock=self.events.lock().unwrap_or_else(|e|e.into_inner());
        for _ in 0..256{unsafe{
            let event=(self.api.wait)(self.handle,0.0);if event.is_null()||(*event).id==0{break;}
            if (*event).id==8 {let same=self.expected_url.lock().map(|u|self.property("path").as_deref()==Some(u.as_str())).unwrap_or(false);self.loaded.store(same,Ordering::Release);}
            if (*event).id==7&&!(*event).data.is_null(){let fields=(*event).data as *const c_int;if *fields==4{self.failed.store(true,Ordering::Release);}}
        }}
    }
    pub fn set_surface_ready(&self,value:bool){self.surface_ready.store(value,Ordering::Release);if !value{self.frame_drawn.store(false,Ordering::Release);}}
    pub fn set_geometry_ready(&self,value:bool){self.geometry_ready.store(value,Ordering::Release);}
    pub fn matches_generation(&self,generation:u64)->bool{generation>0&&self.generation.lock().map(|g|*g==generation).unwrap_or(false)}
    pub fn frame_drawn(&self){self.frame_drawn.store(true,Ordering::Release);}
    pub fn audio_facts(&self)->AudioFacts {
        let (equalizer_enabled,equalizer_applied)=self.equalizer.lock().map(|settings|(settings.enabled,self.equalizer_applied.load(Ordering::Acquire))).unwrap_or((false,false));
        AudioFacts{requested_exclusive:self.exclusive,source_rate:self.number("audio-params/samplerate").map(|x|x as u64),
            output_rate:self.number("audio-out-params/samplerate").map(|x|x as u64),output_channels:self.number("audio-out-params/channel-count").map(|x|x as u64),
            output_format:self.property("audio-out-params/format"),bit_exact_capture_verified:false,
            equalizer_enabled,equalizer_applied,
            bit_perfect_excluded_by_equalizer:equalizer_enabled||!equalizer_applied}
    }
    /// Native proof runner only: output is an owned CLI-selected scratch file. Not registered IPC.
    pub fn capture_pcm(api:Arc<Api>,input:&Path,output:&Path,rate:u32,channels:u8)->Result<()> {
        Self::capture_playlist(api,&[input],output,rate,channels)
    }
    pub fn capture_playlist(api:Arc<Api>,inputs:&[&Path],output:&Path,rate:u32,channels:u8)->Result<()> {
        if inputs.is_empty() || inputs.len()>32{return Err(Error::InvalidCommand);}
        let handle=unsafe{(api.create)()};if handle.is_null(){return Err(Error::EngineUnavailable);}
        let e=Self{api,handle,generation:Mutex::new(1),exclusive:false,loaded:AtomicBool::new(false),surface_ready:AtomicBool::new(false),frame_drawn:AtomicBool::new(false),geometry_ready:AtomicBool::new(false),expected_url:Mutex::new(String::new()),failed:AtomicBool::new(false),events:Mutex::new(()),newest_generation:AtomicU64::new(0),equalizer:Mutex::new(EqualizerSettings::default()),equalizer_applied:AtomicBool::new(true),configured_spdif:String::new()};
        for (k,v) in [("config","no"),("load-scripts","no"),("ytdl","no"),("terminal","no"),("msg-level","all=no"),
            ("vo","null"),("ao","pcm"),("audio-format","s32"),("audio-display","no"),("gapless-audio","yes"),("replaygain","no"),("volume","100"),("pause","yes")] {e.option(k,v)?;}
        e.option("ao-pcm-file",output.to_str().ok_or(Error::InvalidCommand)?)?;e.option("ao-pcm-waveheader","no")?;
        e.option("audio-samplerate",&rate.to_string())?;e.option("audio-channels",&channels.to_string())?;
        status(unsafe{(e.api.initialize)(handle)})?;
        for (i,input) in inputs.iter().enumerate(){e.command(&["loadfile",input.to_str().ok_or(Error::InvalidCommand)?,if i==0{"replace"}else{"append"}])?;}
        e.set("pause","no")?;
        let start=std::time::Instant::now();
        while start.elapsed().as_secs()<30 {
            if (e.flag("idle-active")&&start.elapsed().as_millis()>1000)
                || e.flag("eof-reached")&&e.number("playlist-pos")==Some((inputs.len()-1) as f64) {return Ok(());}
            std::thread::sleep(std::time::Duration::from_millis(10));
        }
        Err(Error::EngineFailure)
    }
}
impl Drop for Mpv {fn drop(&mut self){unsafe{(self.api.destroy)(self.handle)}}}
