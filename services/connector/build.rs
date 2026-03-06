use std::process::Command;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Check if protoc is available
    let has_protoc = Command::new("protoc")
        .arg("--version")
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false);

    if has_protoc {
        // Protoc is available, compile the proto files
        tonic_build::configure()
            .build_server(true)
            .build_client(true)
            .build_transport(false)
            .compile_protos(&["../../shared/proto/controller.proto"], &["../../shared/proto"])?;
    } else {
        // Protoc is not available (e.g., during cross-compilation)
        // Use pre-generated proto file
        println!("cargo:warning=protoc not found, using pre-generated proto code");
        // Copy the pre-generated file to OUT_DIR so tonic::include_proto can find it
        let out_dir = std::env::var("OUT_DIR")?;
        let dest_path = std::path::Path::new(&out_dir).join("controller.v1.rs");
        std::fs::copy("src/proto/controller.v1.rs", dest_path)?;
    }

    println!("cargo:rerun-if-changed=../../shared/proto/controller.proto");
    println!("cargo:rerun-if-changed=../../shared/proto");
    Ok(())
}
