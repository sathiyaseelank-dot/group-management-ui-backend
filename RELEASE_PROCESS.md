# How the Release Process Works

## 📦 Complete Release Flow

### Step 1: You Create a Git Tag

```bash
# Create a version tag
git tag v1.0.0

# Push the tag to GitHub
git push origin v1.0.0
```

### Step 2: GitHub Actions Triggers Automatically

When GitHub sees a tag starting with `v*`, it triggers **three workflows**:

#### Workflow 1: Connector Release
**File:** `.github/workflows/release-connector-rs.yml`

```
Trigger: Tag pushed (v*)
    ↓
Build Job (Parallel):
    ├─ Build for x86_64 (amd64)
    │   ├─ Checkout code
    │   ├─ Install Rust
    │   ├─ Install protoc
    │   ├─ Build: cargo build --release
    │   └─ Output: connector-linux-amd64
    │
    └─ Build for aarch64 (arm64)
        ├─ Checkout code
        ├─ Install Rust + cross-compile tools
        ├─ Install protoc
        ├─ Build: cargo build --release
        └─ Output: connector-linux-arm64
    ↓
Release Job:
    ├─ Download both artifacts
    └─ Upload to GitHub Release
```

#### Workflow 2: Agent Release
**File:** `.github/workflows/release-agent-rs.yml`

```
Trigger: Tag pushed (v*)
    ↓
Build Job (Parallel):
    ├─ Build for x86_64 (amd64)
    │   └─ Output: agent-linux-amd64
    │
    └─ Build for aarch64 (arm64)
        └─ Output: agent-linux-arm64
    ↓
Release Job:
    ├─ Download both artifacts
    └─ Upload to GitHub Release
```

#### Workflow 3: Client Release
**File:** `.github/workflows/release-client-rs.yml`

```
Trigger: Tag pushed (v*)
    ↓
Build Job (Parallel):
    ├─ Build for x86_64 (amd64)
    │   └─ Output: ztna-client-linux-amd64
    │
    └─ Build for aarch64 (arm64)
        └─ Output: ztna-client-linux-arm64
    ↓
Release Job:
    ├─ Download both artifacts
    └─ Upload to GitHub Release
```

### Step 3: GitHub Creates Release

GitHub automatically:
1. Creates a new release page for tag `v1.0.0`
2. Uploads 6 binaries:
   - `connector-linux-amd64`
   - `connector-linux-arm64`
   - `agent-linux-amd64`
   - `agent-linux-arm64`
   - `ztna-client-linux-amd64`
   - `ztna-client-linux-arm64`

### Step 4: Deployment Scripts Download Binaries

When someone runs your deployment script:

```bash
curl -fsSL https://raw.githubusercontent.com/.../scripts/setup.sh | sudo bash
```

**What happens:**

```
setup.sh:
    ↓
1. Detect OS and Architecture
    ├─ OS: linux
    └─ Arch: amd64 or arm64
    ↓
2. Build Download URL
    https://github.com/vairabarath/zero-trust/
    releases/latest/download/connector-linux-amd64
    ↓
3. Download Binary
    curl/wget → /tmp/grpcconnector2
    ↓
4. Install Binary
    install -m 0755 /tmp/grpcconnector2 /usr/bin/grpcconnector2
    ↓
5. Configure Service
    ├─ Create config: /etc/grpcconnector2/connector.conf
    ├─ Download CA cert
    └─ Install systemd service
    ↓
6. Start Service
    systemctl enable grpcconnector2
    systemctl start grpcconnector2
```

## 🔄 Complete Flow Diagram

```
Developer                GitHub Actions              GitHub Release           User Server
    │                           │                           │                      │
    │ git tag v1.0.0            │                           │                      │
    │ git push origin v1.0.0    │                           │                      │
    ├──────────────────────────>│                           │                      │
    │                           │                           │                      │
    │                           │ Trigger workflows         │                      │
    │                           │ (connector + agent)       │                      │
    │                           │                           │                      │
    │                           │ Build amd64 + arm64       │                      │
    │                           │ (6 binaries total)        │                      │
    │                           │                           │                      │
    │                           │ Upload binaries           │                      │
    │                           ├──────────────────────────>│                      │
    │                           │                           │                      │
    │                           │                           │ Release created      │
    │                           │                           │ v1.0.0               │
    │                           │                           │                      │
    │                           │                           │                      │
    │                           │                           │   curl setup.sh      │
    │                           │                           │<─────────────────────│
    │                           │                           │                      │
    │                           │                           │   Download binary    │
    │                           │                           │   /releases/latest/  │
    │                           │                           │   download/...       │
    │                           │                           ├─────────────────────>│
    │                           │                           │                      │
    │                           │                           │                      │ Install
    │                           │                           │                      │ Configure
    │                           │                           │                      │ Start
    │                           │                           │                      │ ✅ Running
```

## 📝 Key Points

### Binary Naming Convention

**In GitHub Release:**
- `connector-linux-amd64` (Rust connector, x86_64)
- `connector-linux-arm64` (Rust connector, ARM64)
- `agent-linux-amd64` (Rust agent, x86_64)
- `agent-linux-arm64` (Rust agent, ARM64)
- `ztna-client-linux-amd64` (Rust client, x86_64)
- `ztna-client-linux-arm64` (Rust client, ARM64)

**After Installation:**
- `/usr/bin/connector`
- `/usr/bin/agent` (renamed)
- `/usr/bin/ztna-client`

### Why "latest" Works

```bash
# Deployment script uses:
releases/latest/download/connector-linux-amd64

# GitHub automatically redirects "latest" to the newest release
# So v1.0.0 → v1.0.1 → v1.0.2 automatically
```

### Workflow Triggers

**Automatic (on tag push):**
```bash
git tag v1.0.0
git push origin v1.0.0
# ✅ Workflows run automatically
```

**Manual (workflow_dispatch):**
```
GitHub UI → Actions → Select workflow → Run workflow
# ✅ Can trigger manually without tag
```

## 🎯 When to Release

### Release New Binaries When:

✅ **Code Changes**
```bash
# Made changes to connector/agent code
git add services/connector/src/main.rs
git commit -m "feat: add new feature"
git tag v1.0.1
git push origin v1.0.1
```

✅ **Dependency Updates**
```bash
# Updated Cargo.toml dependencies
cd services/connector
cargo update
git commit -am "chore: update dependencies"
git tag v1.0.2
git push origin v1.0.2
```

✅ **Bug Fixes**
```bash
git commit -m "fix: resolve connection issue"
git tag v1.0.3
git push origin v1.0.3
```

### Don't Release When:

❌ **Documentation changes only**
❌ **Directory reorganization** (like we just did)
❌ **CI/CD workflow updates**
❌ **README updates**

## 🔍 Checking Release Status

### View Releases
```
https://github.com/vairabarath/zero-trust/releases
```

### Check Workflow Status
```
https://github.com/vairabarath/zero-trust/actions
```

### Test Download
```bash
# Test if binary is downloadable
curl -I https://github.com/vairabarath/zero-trust/releases/latest/download/connector-linux-amd64
# Should return: HTTP/2 302 (redirect to actual file)
```

## 🚀 Release Checklist

### Before Releasing:

- [ ] Code changes committed
- [ ] Tests pass locally
- [ ] Version number decided (v1.0.x)
- [ ] CHANGELOG updated (optional)

### Release Steps:

```bash
# 1. Create tag
git tag v1.0.0

# 2. Push tag
git push origin v1.0.0

# 3. Wait for GitHub Actions (5-10 minutes)
# Check: https://github.com/vairabarath/zero-trust/actions

# 4. Verify release created
# Check: https://github.com/vairabarath/zero-trust/releases

# 5. Test deployment (optional)
curl -fsSL https://raw.githubusercontent.com/vairabarath/zero-trust/main/scripts/setup.sh | sudo bash
```

### After Release:

- [ ] Verify binaries uploaded (4 files)
- [ ] Test deployment script
- [ ] Update documentation if needed
- [ ] Notify team

## 🐛 Troubleshooting

### Workflow Fails?

**Check:**
1. GitHub Actions logs
2. Build errors in workflow
3. Proto file paths correct
4. Dependencies available

**Fix:**
```bash
# Test build locally first
cd services/connector
cargo build --release

cd services/agent
cargo build --release
```

### Binary Not Found?

**Check:**
1. Release created on GitHub?
2. Binary name matches script?
3. Using correct repository URL?

### Deployment Script Fails?

**Check:**
1. Binary downloadable?
2. Permissions correct?
3. Environment variables set?

## 📊 Summary

**Release Process:**
1. Developer pushes tag → `git push origin v1.0.0`
2. GitHub Actions builds binaries (4 total)
3. GitHub creates release with binaries
4. Users run deployment script
5. Script downloads from `/releases/latest/`
6. Binary installed and service started

**Current Status:**
- ✅ Workflows configured
- ✅ Paths updated
- ✅ Ready for next release
- ❌ No release needed yet (no code changes)

**Next Release:**
When you have code changes, just:
```bash
git tag v1.0.0
git push origin v1.0.0
```

That's it! 🎉
