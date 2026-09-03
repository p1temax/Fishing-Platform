#!/bin/bash

# Go Backend Setup Script
# This script helps set up the Go backend environment

set -e

echo "=================================="
echo "Go Backend Setup Script"
echo "=================================="
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Go is not installed!"
    echo ""
    echo "Please install Go 1.21 or higher:"
    echo ""
    echo "  macOS:"
    echo "    brew install go"
    echo ""
    echo "  Ubuntu/Debian:"
    echo "    sudo apt update && sudo apt install golang"
    echo ""
    echo "  Windows:"
    echo "    Download from: https://golang.org/dl/"
    echo ""
    exit 1
fi

# Check Go version
GO_VERSION=$(go version | awk '{print $3}')
echo "✅ Go is installed: $GO_VERSION"
echo ""

# Check Go version requirement
REQUIRED_VERSION="go1.21"
if [[ "$(printf '%s\n' "$REQUIRED_VERSION" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_VERSION" ]]; then
    echo "❌ Go version 1.21 or higher is required!"
    echo "   Current version: $GO_VERSION"
    echo ""
    exit 1
fi

echo "✅ Go version meets requirements (>= 1.21)"
echo ""

# Navigate to backend-go directory
cd "$(dirname "$0")"
echo "📁 Working directory: $(pwd)"
echo ""

# Install dependencies and generate go.sum
echo "📦 Installing dependencies..."
if go mod tidy; then
    echo "✅ Dependencies installed successfully"
else
    echo "❌ Failed to install dependencies"
    exit 1
fi
echo ""

# Create config.yaml file if it doesn't exist
if [ ! -f config.yaml ]; then
    echo "📝 Creating config.yaml from config.yaml.example..."
    cp config.yaml.example config.yaml
    echo "✅ config.yaml created"
    echo ""
    echo "⚠️  Please edit config.yaml to configure your environment"
    echo ""
else
    echo "✅ config.yaml already exists"
    echo ""
fi

# Create data directory
mkdir -p data
echo "✅ Data directory created/verified"
echo ""

echo "=================================="
echo "✅ Setup completed successfully!"
echo "=================================="
echo ""
echo "To run the application:"
echo "  go run ."
echo ""
echo "Or build and run:"
echo "  go build -o fishing-platform ."
echo "  ./fishing-platform"
echo ""
echo "Or use Docker:"
echo "  docker build -t fishing-platform-backend ."
echo "  docker run -p 8000:8000 -v \$(pwd)/data:/root/data fishing-platform-backend"
echo ""
