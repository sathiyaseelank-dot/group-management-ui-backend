#!/bin/bash
set -e

echo "=== ZTNA Project Reorganization ==="
echo "This will reorganize the repository structure"
echo ""

# Step 1: Backup
echo "Step 1: Creating backup..."
backup_dir="../ztna-backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup_dir"
cp -r . "$backup_dir/"
echo "✓ Backup created at $backup_dir"
echo ""

# Step 2: Remove old Go implementations
echo "Step 2: Removing old Go connector and tunneler..."
rm -rf backend/connector
rm -rf backend/tunneler
rm -f backend/go.mod backend/go.sum
echo "✓ Removed old Go implementations"
echo ""

# Step 3: Create new structure
echo "Step 3: Creating new directory structure..."
mkdir -p services/{controller,connector,tunneler}
mkdir -p apps/frontend
mkdir -p shared/{proto,configs}
mkdir -p docs
echo "✓ Created new directories"
echo ""

# Step 4: Move components
echo "Step 4: Moving components to new structure..."

# Move controller
echo "  - Moving controller..."
mv backend/controller/* services/controller/ 2>/dev/null || true

# Move connector-rs to connector
echo "  - Moving connector-rs to connector..."
mv backend/connector-rs/* services/connector/ 2>/dev/null || true

# Move tunneler-rs to tunneler
echo "  - Moving tunneler-rs to tunneler..."
mv backend/tunneler-rs/* services/tunneler/ 2>/dev/null || true

# Move frontend
echo "  - Moving frontend..."
mv frontend/* apps/frontend/ 2>/dev/null || true

# Move proto files
echo "  - Moving proto files..."
mv backend/proto/* shared/proto/ 2>/dev/null || true

echo "✓ Components moved"
echo ""

# Step 5: Clean up old directories
echo "Step 5: Cleaning up old directories..."
rm -rf backend/controller backend/connector-rs backend/tunneler-rs backend/proto
rmdir backend 2>/dev/null || true
rmdir frontend 2>/dev/null || true
echo "✓ Old directories removed"
echo ""

echo "=== Reorganization Complete ==="
echo ""
echo "New structure:"
echo "  services/controller/  - Go controller"
echo "  services/connector/   - Rust connector"
echo "  services/tunneler/    - Rust tunneler"
echo "  apps/frontend/        - React UI"
echo "  shared/proto/         - Protobuf definitions"
echo ""
echo "Next steps:"
echo "  1. Review changes: git status"
echo "  2. Update CI/CD paths: edit .github/workflows/*.yml"
echo "  3. Test builds"
