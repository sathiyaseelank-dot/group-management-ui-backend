use std::process::Command;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let has_protoc = Command::new("protoc")
        .arg("--version")
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false);

    if has_protoc {
        tonic_build::configure()
            .build_server(false)
            .build_client(true)
            .build_transport(false)
            .compile_protos(&["../../shared/proto/controller.proto"], &["../../shared/proto"])?;
    } else {
        println!("cargo:warning=protoc not found, using pre-generated proto code");
        let out_dir = std::env::var("OUT_DIR")?;
        let dest_path = std::path::Path::new(&out_dir).join("controller.v1.rs");
        std::fs::copy("src/proto/controller.v1.rs", dest_path)?;
    }

    println!("cargo:rerun-if-changed=../../shared/proto/controller.proto");
    println!("cargo:rerun-if-changed=../../shared/proto");
    Ok(())
}
