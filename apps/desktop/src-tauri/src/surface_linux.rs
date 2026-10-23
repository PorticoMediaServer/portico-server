//! GTK Overlay: GLArea below the unchanged transparent WebKitGTK widget. SDR only.
use crate::{audio::AudioOutput,boundary::{Error,Result},mpv::{Api,Mpv,VideoTarget}};
use gtk::{prelude::*,glib};
use std::{cell::RefCell,ffi::{c_char,c_int,c_void},ptr,rc::Rc,sync::Arc,time::Duration};
use webkit2gtk::WebViewExt;
use std::sync::{atomic::{AtomicU64,Ordering},OnceLock};
static FRAMES:AtomicU64=AtomicU64::new(0);
static COLORED:AtomicU64=AtomicU64::new(0);
static RENDERER:OnceLock<String>=OnceLock::new();
pub fn proof()->serde_json::Value{serde_json::json!({"renderedFrames":FRAMES.load(Ordering::Relaxed),"nonBlackFrames":COLORED.load(Ordering::Relaxed),"renderer":RENDERER.get(),"output":"SDR OpenGL","hdrVerified":false})}

#[repr(C)]struct Param{kind:c_int,data:*mut c_void}
#[repr(C)]struct Init{get:unsafe extern "C" fn(*mut c_void,*const c_char)->*mut c_void,context:*mut c_void}
#[repr(C)]struct Fbo{id:c_int,width:c_int,height:c_int,format:c_int}
type Create=unsafe extern "C" fn(*mut *mut c_void,*mut c_void,*mut Param)->c_int;
type Render=unsafe extern "C" fn(*mut c_void,*mut Param)->c_int;
type Free=unsafe extern "C" fn(*mut c_void);
type GetProc=unsafe extern "C" fn(*const c_char)->*mut c_void;
type GetInt=unsafe extern "C" fn(u32,*mut i32);

struct Context{raw:*mut c_void,engine:Arc<Mpv>,free:Free,gl:Box<libloading::Library>}
impl Drop for Context{fn drop(&mut self){unsafe{(self.free)(self.raw)}}}
unsafe extern "C" fn get_proc(ctx:*mut c_void,name:*const c_char)->*mut c_void {
    unsafe {
        let lib=&*(ctx as *const libloading::Library);
        if let Ok(get)=lib.get::<GetProc>(b"eglGetProcAddress\0"){let p=get(name);if !p.is_null(){return p;}}
        lib.get::<*mut c_void>(std::ffi::CStr::from_ptr(name).to_bytes_with_nul()).map(|x|*x).unwrap_or(ptr::null_mut())
    }
}
pub fn attach(webview:&tauri::webview::PlatformWebview,api:Arc<Api>,output:&AudioOutput)->Result<Arc<Mpv>> {
    let engine=Mpv::new(api,output,VideoTarget::OpenGl)?;
    let web=webview.inner();
    web.set_background_color(&gtk::gdk::RGBA::new(0.0,0.0,0.0,0.0));
    let parent=web.parent().ok_or(Error::Unsupported)?.downcast::<gtk::Box>().map_err(|_|Error::Unsupported)?;
    parent.remove(&web);
    let overlay=gtk::Overlay::new();let fixed=gtk::Fixed::builder().hexpand(true).vexpand(true).build();
    let area=gtk::GLArea::builder().auto_render(false).has_alpha(false).hexpand(true).vexpand(true).build();
    area.set_required_version(3,2);
    area.set_widget_name("portico-native-video");fixed.put(&area,0,0);
    overlay.add(&fixed);overlay.add_overlay(&web);parent.pack_start(&overlay,true,true,0);
    let context:Rc<RefCell<Option<Context>>>=Rc::new(RefCell::new(None));
    let current=context.clone();let source=engine.clone();
    area.connect_realize(move |area| {
        area.make_current();if area.error().is_some(){return;}
        let created=(||->Result<Context>{unsafe{
            let gl=Box::new(libloading::Library::new("libEGL.so.1").map_err(|_|Error::EngineUnavailable)?);
            let create=source.api.symbol::<Create>(b"mpv_render_context_create\0")?;
            let free=source.api.symbol::<Free>(b"mpv_render_context_free\0")?;
            let mut init=Init{get:get_proc,context:(&*gl as *const libloading::Library).cast_mut().cast()};
            let mut params=[Param{kind:1,data:c"opengl".as_ptr().cast_mut().cast()},Param{kind:2,data:(&mut init as *mut Init).cast()},Param{kind:0,data:ptr::null_mut()}];
            let mut raw=ptr::null_mut();if create(&mut raw,source.handle(),params.as_mut_ptr())<0{return Err(Error::EngineFailure);}
            source.set_surface_ready(true);Ok(Context{raw,engine:source.clone(),free,gl})
        }})();
        *current.borrow_mut()=created.ok();
    });
    let current=context.clone();
    area.connect_render(move |area,_|{
        if let Some(ctx)=current.borrow().as_ref(){unsafe{
            let mut id=0;
            let p=get_proc((&*ctx.gl as *const libloading::Library).cast_mut().cast(),c"glGetIntegerv".as_ptr());
            if p.is_null(){return glib::Propagation::Proceed;}
            let get:GetInt=std::mem::transmute(p);get(0x8CA6,&mut id);
            let p=get_proc((&*ctx.gl as *const libloading::Library).cast_mut().cast(),c"glGetString".as_ptr());
            if !p.is_null(){let get:unsafe extern "C" fn(u32)->*const c_char=std::mem::transmute(p);let name=get(0x1F01);if !name.is_null(){let _=RENDERER.set(std::ffi::CStr::from_ptr(name).to_string_lossy().into_owned());}}
            let scale=area.scale_factor();let mut target=Fbo{id,width:area.allocated_width()*scale,height:area.allocated_height()*scale,format:0};let mut flip=1;
            let mut params=[Param{kind:3,data:(&mut target as *mut Fbo).cast()},Param{kind:4,data:(&mut flip as *mut i32).cast()},Param{kind:0,data:ptr::null_mut()}];
            if let Ok(render)=ctx.engine.api.symbol::<Render>(b"mpv_render_context_render\0"){
                if render(ctx.raw,params.as_mut_ptr())>=0 {
                    FRAMES.fetch_add(1,Ordering::Relaxed);
                    ctx.engine.frame_drawn();
                    #[cfg(debug_assertions)]
                    {let p=get_proc((&*ctx.gl as *const libloading::Library).cast_mut().cast(),c"glReadPixels".as_ptr());
                     if !p.is_null(){let read:unsafe extern "C" fn(i32,i32,i32,i32,u32,u32,*mut c_void)=std::mem::transmute(p);let mut pixel=[0u8;4];
                       read(target.width/2,target.height/2,1,1,0x1908,0x1401,pixel.as_mut_ptr().cast());if pixel[..3].iter().any(|x|*x>12){COLORED.fetch_add(1,Ordering::Relaxed);}}}
                }
            }
        }}
        glib::Propagation::Stop
    });
    let current=context.clone();
    let source=engine.clone();
    area.connect_unrealize(move |area|{area.make_current();source.set_surface_ready(false);current.borrow_mut().take();});
    let weak=area.downgrade();
    glib::timeout_add_local(Duration::from_millis(16),move||{
        if let Some(area)=weak.upgrade(){area.queue_render();glib::ControlFlow::Continue}else{glib::ControlFlow::Break}
    });
    overlay.show_all();
    Ok(engine)
}
pub fn bounds(webview:&tauri::webview::PlatformWebview,bounds:crate::boundary::SurfaceBounds)->Result<()> {
    let overlay=webview.inner().parent().ok_or(Error::Unsupported)?.downcast::<gtk::Overlay>().map_err(|_|Error::Unsupported)?;
    let fixed=overlay.child().ok_or(Error::Unsupported)?.downcast::<gtk::Fixed>().map_err(|_|Error::Unsupported)?;
    let area=fixed.children().into_iter().find(|w|w.widget_name()=="portico-native-video").ok_or(Error::Unsupported)?;
    area.set_size_request(bounds.width.ceil().max(1.0) as i32,bounds.height.ceil().max(1.0) as i32);
    fixed.move_(&area,bounds.x.round() as i32,bounds.y.round() as i32);
    area.set_visible(bounds.width>0.0&&bounds.height>0.0);Ok(())
}
