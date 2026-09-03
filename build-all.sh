#!/bin/bash

# Cross-Platform Build Script
# This script builds the application for multiple platforms

set -e

echo "=================================="
echo "Cross-Platform Build Script"
echo "=================================="
echo ""

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Step 1: Build frontend
echo -e "${BLUE}[Step 1/2] Building frontend...${NC}"
cd "$(dirname "$0")/frontend"
if [ ! -d "node_modules" ]; then
    echo "Installing frontend dependencies..."
    npm install
fi
echo "Building frontend..."
npm run build
if [ ! -d "dist" ]; then
    echo "❌ Frontend build failed - dist directory not found"
    exit 1
fi
echo -e "${GREEN}✅ Frontend built successfully${NC}"
echo ""

# Step 2: Build for multiple platforms
echo -e "${BLUE}[Step 2/2] Building Go binaries for multiple platforms...${NC}"
cd ..

# Create release directory
mkdir -p release
rm -rf release/*

# Build targets: GOOS GOARCH output_name
# Keep this list Bash 3 compatible for the default macOS /bin/bash.
TARGETS="
linux amd64 fishing-platform-linux-amd64
linux arm64 fishing-platform-linux-arm64
windows amd64 fishing-platform-windows-amd64.exe
windows arm64 fishing-platform-windows-arm64.exe
darwin amd64 fishing-platform-macos-amd64
darwin arm64 fishing-platform-macos-arm64
"

# Build for each platform
while read -r os arch binary; do
    if [ -z "$os" ]; then
        continue
    fi

    GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -ldflags="-s -w" -o "release/${binary}" main.go
    if [ -f "release/${binary}" ]; then
        SIZE=$(du -h "release/${binary}" | cut -f1)
        echo -e "${GREEN}✅ Built for $os ($arch) - $SIZE${NC}"
    else
        echo -e "${YELLOW}❌ Build failed for $os${NC}"
    fi

	agent_binary="${binary/fishing-platform/fishing-agent}"
	GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -ldflags="-s -w" -o "release/${agent_binary}" ./agent
	if [ -f "release/${agent_binary}" ]; then
		SIZE=$(du -h "release/${agent_binary}" | cut -f1)
		echo -e "${GREEN}✅ Built Agent for $os ($arch) - $SIZE${NC}"
	else
		echo -e "${YELLOW}❌ Agent build failed for $os${NC}"
	fi
done <<EOF
$TARGETS
EOF

cp config.yaml.example release/config.yaml.example
cp agent.yaml.example release/agent.yaml.example

echo ""
echo "=================================="
echo -e "${GREEN}✅ Cross-platform build completed!${NC}"
echo "=================================="
echo ""
echo "Release files:"
ls -lh release/
echo ""
echo "To run on each platform:"
echo "  Linux AMD64:    ./release/fishing-platform-linux-amd64"
echo "  Linux ARM64:    ./release/fishing-platform-linux-arm64"
echo "  Windows AMD64:  release\\fishing-platform-windows-amd64.exe"
echo "  Windows ARM64:  release\\fishing-platform-windows-arm64.exe"
echo "  macOS AMD64:    ./release/fishing-platform-macos-amd64"
echo "  macOS ARM64:    ./release/fishing-platform-macos-arm64"
echo ""
