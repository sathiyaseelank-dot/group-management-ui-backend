use std::process::Command;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let has_protoc = Command::new("protoc")
        .arg("--version")
        .output()
        .map(|output| output.status.success())
        .unwrap_or(false);

    let proto_path = "../../shared/proto/controller.proto";
    let proto_exists = std::path::Path::new(proto_path).exists();

    let compiled = if has_protoc && proto_exists {
        match tonic_build::configure()
            .build_server(true)
            .build_client(true)
            .build_transport(false)
            .compile_protos(&[proto_path], &["../../shared/proto"])
        {
            Ok(()) => true,
            Err(e) => {
                println!("cargo:warning=protoc compilation failed ({e}), falling back to pre-generated code");
                false
            }
        }
    } else {
        false
    };

    if !compiled {
        println!("cargo:warning=using pre-generated proto code");
        let out_dir = std::env::var("OUT_DIR")?;
        let dest_path = std::path::Path::new(&out_dir).join("controller.v1.rs");
        std::fs::copy("src/proto/controller.v1.rs", dest_path)?;
    }

    println!("cargo:rerun-if-changed=../../shared/proto/controller.proto");
    println!("cargo:rerun-if-changed=../../shared/proto");
    Ok(())
}
