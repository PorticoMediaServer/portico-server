//! AppKit owns the Metal-backed NSView below WKWebView. The native VO owns its HDR swapchain.
//! EDR is requested, not reported as measured: monitor/OS/Metal validation is a separate spike gate.
use crate::{audio::AudioOutput,boundary::{Error,Result},mpv::{Api,Mpv,VideoTarget}};
use std::{ffi::{c_char,c_void},sync::Arc};

type Id=*mut c_void;
type Sel=*mut c_void;
pub fn proof()->serde_json::Value{serde_json::json!({"output":"Metal-backed NSView under WKWebView","hdrVerified":false,"renderedFrames":null})}
#[repr(C)]#[derive(Clone,Copy)]struct Point{x:f64,y:f64}
#[repr(C)]#[derive(Clone,Copy)]struct Size{width:f64,height:f64}
#[repr(C)]#[derive(Clone,Copy)]struct Rect{origin:Point,size:Size}
#[link(name="objc")]
unsafe extern "C" {fn objc_getClass(name:*const c_char)->Id;fn sel_registerName(name:*const c_char)->Sel;fn objc_msgSend();}
#[link(name="AppKit",kind="framework")]
unsafe extern "C" {}
#[link(name="QuartzCore",kind="framework")]
unsafe extern "C" {}

unsafe fn sel(name:&std::ffi::CStr)->Sel{unsafe{sel_registerName(name.as_ptr())}}
unsafe fn get(object:Id,name:&std::ffi::CStr)->Id{unsafe{let f:unsafe extern "C" fn(Id,Sel)->Id=std::mem::transmute(objc_msgSend as *const ());f(object,sel(name))}}
unsafe fn boolean(object:Id,name:&std::ffi::CStr,value:bool){unsafe{let f:unsafe extern "C" fn(Id,Sel,bool)=std::mem::transmute(objc_msgSend as *const ());f(object,sel(name),value)}}
unsafe fn void(object:Id,name:&std::ffi::CStr){unsafe{let f:unsafe extern "C" fn(Id,Sel)=std::mem::transmute(objc_msgSend as *const ());f(object,sel(name))}}

pub fn attach(webview:&tauri::webview::PlatformWebview,api:Arc<Api>,output:&AudioOutput)->Result<Arc<Mpv>> {
    unsafe {
        let web=webview.inner();if web.is_null(){return Err(Error::Unsupported);}
        let parent=get(web,c"superview");if parent.is_null(){return Err(Error::Unsupported);}
        let bounds:unsafe extern "C" fn(Id,Sel)->Rect=std::mem::transmute(objc_msgSend as *const ());
        let rect=bounds(parent,sel(c"bounds"));
        let allocated=get(objc_getClass(c"NSView".as_ptr()),c"alloc");
        let init:unsafe extern "C" fn(Id,Sel,Rect)->Id=std::mem::transmute(objc_msgSend as *const ());
        let view=init(allocated,sel(c"initWithFrame:"),rect);if view.is_null(){return Err(Error::EngineUnavailable);}
        boolean(view,c"setWantsLayer:",true);
        let layer=get(objc_getClass(c"CAMetalLayer".as_ptr()),c"layer");
        boolean(layer,c"setWantsExtendedDynamicRangeContent:",true);
        let set:unsafe extern "C" fn(Id,Sel,Id)=std::mem::transmute(objc_msgSend as *const ());set(view,sel(c"setLayer:"),layer);
        let sizing:unsafe extern "C" fn(Id,Sel,usize)=std::mem::transmute(objc_msgSend as *const ());sizing(view,sel(c"setAutoresizingMask:"),18);
        let add:unsafe extern "C" fn(Id,Sel,Id,isize,Id)=std::mem::transmute(objc_msgSend as *const ());add(parent,sel(c"addSubview:positioned:relativeTo:"),view,-1,web);
        let result=Mpv::new(api,output,VideoTarget::NativeView(view as usize));
        // Parent retains the NSView; balance the allocation on success and failure.
        if result.is_err(){void(view,c"removeFromSuperview");}
        void(view,c"release");
        result
    }
}
pub fn bounds(_webview:&tauri::webview::PlatformWebview,_bounds:crate::boundary::SurfaceBounds)->Result<()> {
    // No current-frame callback from the native Metal VO has been integrated yet. Keep its
    // active flag false; never clear web paint based only on an allocated NSView.
    Err(Error::Unsupported)
}
