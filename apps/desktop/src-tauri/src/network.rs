//! Read-only OS network facts. Unknown never grants automatic unmetered transfer.
use serde::Serialize;

#[derive(Clone,Copy,Debug,Default,PartialEq,Eq,Serialize)]
#[serde(rename_all="lowercase")]
pub enum NetworkCost { #[default] Unknown, Metered, Unmetered }

fn network_manager_cost(state:Option<u32>,metered:Option<u32>)->NetworkCost {
    // NM_STATE_CONNECTED_LOCAL/SITE/GLOBAL. A local media server needs no Internet
    // connectivity probe. GUESS_YES/GUESS_NO remain unknown, not measured costs.
    if !state.is_some_and(|v|matches!(v,50|60|70)){return NetworkCost::Unknown}
    match metered {Some(1)=>NetworkCost::Metered,Some(2)=>NetworkCost::Unmetered,_=>NetworkCost::Unknown}
}

#[cfg(target_os="linux")]
pub fn current()->NetworkCost {
    use gtk::{gio,glib::{self,prelude::*}};
    let read=(||{
        let bus=gio::bus_get_sync(gio::BusType::System,gio::Cancellable::NONE).ok()?;
        let reply=bus.call_sync(Some("org.freedesktop.NetworkManager"),"/org/freedesktop/NetworkManager",
            "org.freedesktop.DBus.Properties","GetAll",Some(&("org.freedesktop.NetworkManager",).to_variant()),
            None,gio::DBusCallFlags::NO_AUTO_START,1000,gio::Cancellable::NONE).ok()?;
        let (values,)=reply.get::<(std::collections::HashMap<String,glib::Variant>,)>()?;
        Some(network_manager_cost(values.get("State").and_then(|v|v.get()),values.get("Metered").and_then(|v|v.get())))
    })();
    read.unwrap_or_default()
}

#[cfg(target_os="macos")]
unsafe extern "C" {fn portico_network_start();fn portico_network_stop();fn portico_network_cost()->u32;}
/// Called only on the platform main thread during owned application setup/teardown.
pub fn start(){#[cfg(target_os="macos")]unsafe{portico_network_start();}}
pub fn stop(){#[cfg(target_os="macos")]unsafe{portico_network_stop();}}
#[cfg(target_os="macos")]
pub fn current()->NetworkCost{match unsafe{portico_network_cost()}{1=>NetworkCost::Metered,2=>NetworkCost::Unmetered,_=>NetworkCost::Unknown}}
#[cfg(not(any(target_os="linux",target_os="macos")))]
pub fn current()->NetworkCost{NetworkCost::Unknown}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn only_explicit_connected_network_cost_grants_unmetered(){
        for state in [None,Some(0),Some(10),Some(20),Some(30),Some(40),Some(99)]{
            for metered in [None,Some(0),Some(1),Some(2),Some(3),Some(4)]{assert_eq!(network_manager_cost(state,metered),NetworkCost::Unknown);}
        }
        for state in [50,60,70]{
            assert_eq!(network_manager_cost(Some(state),Some(1)),NetworkCost::Metered);
            assert_eq!(network_manager_cost(Some(state),Some(2)),NetworkCost::Unmetered);
            for guess in [None,Some(0),Some(3),Some(4)]{assert_eq!(network_manager_cost(Some(state),guess),NetworkCost::Unknown);}
        }
    }
}
