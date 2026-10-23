use super::Surface;
use crate::boundary::{Error,Result};
use std::ffi::{c_char,c_void,CStr};
type Id=*mut c_void;
type Sel=*mut c_void;
#[repr(C)]#[derive(Clone,Copy)]struct Point{x:f64,y:f64}
#[repr(C)]#[derive(Clone,Copy,Default)]struct Rect{x:f64,y:f64,width:f64,height:f64}
#[link(name="objc")]
unsafe extern "C" {fn sel_registerName(name:*const c_char)->Sel;fn objc_msgSend();#[cfg(target_arch="x86_64")]fn objc_msgSend_stret();}
unsafe fn selector(name:&CStr)->Sel{unsafe{sel_registerName(name.as_ptr())}}
unsafe fn frame(view:Id)->Rect{
    #[cfg(target_arch="x86_64")]
    unsafe{let mut value=Rect::default();let get:unsafe extern "C" fn(*mut Rect,Id,Sel)=std::mem::transmute(objc_msgSend_stret as *const ());get(&mut value,view,selector(c"frame"));value}
    #[cfg(not(target_arch="x86_64"))]
    unsafe{let get:unsafe extern "C" fn(Id,Sel)->Rect=std::mem::transmute(objc_msgSend as *const ());get(view,selector(c"frame"))}
}
pub fn title_hidden(window:&tauri::WebviewWindow,hidden:bool)->Result<()> {
    let native=window.ns_window().map_err(|_|Error::WindowUnavailable)?;
    if native.is_null(){return Err(Error::WindowUnavailable);}
    unsafe{let set:unsafe extern "C" fn(Id,Sel,isize)=std::mem::transmute(objc_msgSend as *const ());set(native,selector(c"setTitleVisibility:"),if hidden{1}else{0});}
    Ok(())
}
pub fn position(window:&tauri::WebviewWindow,surface:Surface,controls:bool,fullscreen:bool)->Result<()> {
    let native=window.ns_window().map_err(|_|Error::WindowUnavailable)?;
    if native.is_null(){return Err(Error::WindowUnavailable);}
    unsafe{
        let button:unsafe extern "C" fn(Id,Sel,usize)->Id=std::mem::transmute(objc_msgSend as *const ());
        let get:unsafe extern "C" fn(Id,Sel)->Id=std::mem::transmute(objc_msgSend as *const ());
        let boolean:unsafe extern "C" fn(Id,Sel,bool)=std::mem::transmute(objc_msgSend as *const ());
        let flag:unsafe extern "C" fn(Id,Sel)->bool=std::mem::transmute(objc_msgSend as *const ());
        let point:unsafe extern "C" fn(Id,Sel,Point)=std::mem::transmute(objc_msgSend as *const ());
        let hidden=fullscreen||(!matches!(surface,Surface::Pages)&&!controls);
        let center=if matches!(surface,Surface::Pages){38.0}else{54.0};
        for index in 0..3{
            let view=button(native,selector(c"standardWindowButton:"),index);
            if view.is_null(){return Err(Error::WindowUnavailable);}
            boolean(view,selector(c"setHidden:"),hidden);
            if hidden{continue;}
            let parent=get(view,selector(c"superview"));if parent.is_null(){return Err(Error::WindowUnavailable);}
            let size=frame(view);let parent_size=frame(parent);
            let y=if flag(parent,selector(c"isFlipped")){center-size.height/2.0}else{parent_size.height-center-size.height/2.0};
            point(view,selector(c"setFrameOrigin:"),Point{x:26.0+index as f64*20.0,y});
        }
    }
    Ok(())
}
