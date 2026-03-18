import java.io.ByteArrayOutputStream

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "com.zerotrust.ztna"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.zerotrust.ztna"
        minSdk = 26
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"

        ndk {
            abiFilters += listOf("arm64-v8a", "x86_64")
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }

    // Include UniFFI-generated Kotlin sources
    sourceSets["main"].kotlin.srcDir("uniffi/ztna")

    // Include prebuilt Rust .so libraries
    sourceSets["main"].jniLibs.srcDirs("src/main/jniLibs")
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.ui)
    implementation(libs.androidx.ui.graphics)
    implementation(libs.androidx.ui.tooling.preview)
    implementation(libs.androidx.material3)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.browser)
    implementation(libs.kotlinx.coroutines.android)
    debugImplementation(libs.androidx.ui.tooling)

    // UniFFI generated bindings runtime
    implementation("net.java.dev.jna:jna:5.14.0@aar")
}

// ── Rust build tasks ────────────────────────────────────────────────────────

val rustProjectDir = rootProject.projectDir.parentFile.parentFile
    .resolve("services/ztna-client-mobile")

/**
 * Cross-compile the Rust mobile core for the two Android ABIs.
 * Requires: cargo-ndk  (`cargo install cargo-ndk`)
 *           Android NDK installed and ANDROID_NDK_HOME set
 */
tasks.register<Exec>("buildRustArm64") {
    group = "rust"
    description = "Cross-compile Rust core for arm64-v8a"
    workingDir = rustProjectDir
    commandLine(
        "cargo", "ndk",
        "--target", "aarch64-linux-android",
        "--android-platform", "26",
        "--", "build", "--release"
    )
}

tasks.register<Exec>("buildRustX86_64") {
    group = "rust"
    description = "Cross-compile Rust core for x86_64 (emulator)"
    workingDir = rustProjectDir
    commandLine(
        "cargo", "ndk",
        "--target", "x86_64-linux-android",
        "--android-platform", "26",
        "--", "build", "--release"
    )
}

tasks.register("buildRustAll") {
    group = "rust"
    description = "Cross-compile Rust core for all Android ABIs"
    dependsOn("buildRustArm64", "buildRustX86_64")
}

/**
 * Copy compiled .so libraries into jniLibs.
 */
tasks.register<Copy>("copyRustLibs") {
    group = "rust"
    description = "Copy compiled .so files into jniLibs"
    dependsOn("buildRustAll")

    val targetDir = rustProjectDir.resolve("target")
    from(targetDir.resolve("aarch64-linux-android/release/libztna.so")) {
        into("arm64-v8a")
    }
    from(targetDir.resolve("x86_64-linux-android/release/libztna.so")) {
        into("x86_64")
    }
    into(projectDir.resolve("src/main/jniLibs"))
}

/**
 * Generate UniFFI Kotlin bindings from ztna.udl.
 * Requires: uniffi-bindgen  (`cargo install uniffi-bindgen`)
 */
tasks.register<Exec>("generateUniffiBindings") {
    group = "rust"
    description = "Generate UniFFI Kotlin bindings"
    workingDir = rustProjectDir
    commandLine(
        "cargo", "run",
        "--manifest-path", rustProjectDir.resolve("Cargo.toml").absolutePath,
        "--bin", "uniffi-bindgen",
        "generate",
        "--library", rustProjectDir.resolve(
            "target/aarch64-linux-android/release/libztna.so"
        ).absolutePath,
        "--language", "kotlin",
        "--out-dir", projectDir.resolve("uniffi/ztna").absolutePath
    )
    dependsOn("buildRustArm64")
}

// Wire Rust compilation into the Android pre-build phase (optional, enable manually)
// tasks.named("preBuild") { dependsOn("copyRustLibs") }
