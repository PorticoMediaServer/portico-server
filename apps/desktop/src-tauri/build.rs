fn main() {
    if std::env::var_os("CARGO_FEATURE_SHELL").is_some()&&std::env::var("CARGO_CFG_TARGET_OS").as_deref()==Ok("macos") {
        use std::{path::PathBuf,process::Command};
        println!("cargo:rerun-if-changed=src/network_macos.m");
        let out=PathBuf::from(std::env::var_os("OUT_DIR").expect("native output directory"));
        let object=out.join("portico-network.o");let library=out.join("libportico_network.a");
        let arch=match std::env::var("CARGO_CFG_TARGET_ARCH").as_deref(){Ok("aarch64")=>"arm64",Ok("x86_64")=>"x86_64",_=>panic!("unsupported desktop Mac architecture")};
        assert!(Command::new("xcrun").args(["clang","-arch",arch,"-fobjc-arc","-fblocks","-c","src/network_macos.m","-o"]).arg(&object).status().expect("Apple compiler").success(),"native network compiler failed");
        assert!(Command::new("xcrun").arg("ar").arg("rcs").arg(&library).arg(&object).status().expect("Apple archiver").success(),"native network archive failed");
        println!("cargo:rustc-link-search=native={}",out.display());
        println!("cargo:rustc-link-lib=static=portico_network");
        println!("cargo:rustc-link-lib=framework=Foundation");
        println!("cargo:rustc-link-arg=-Wl,-weak_framework,Network");
    }
    #[cfg(feature = "shell")]
    tauri_build::build();
}
