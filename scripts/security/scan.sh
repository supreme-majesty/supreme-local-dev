#!/bin/bash
set -e

# Usage: ./scripts/security/scan.sh

if ! command -v trivy &> /dev/null; then
    echo "Trivy not found. Please install trivy."
    exit 1
fi

echo "Running Trivy Filesystem Scan..."
trivy fs . --severity HIGH,CRITICAL --ignore-unfixed

echo "Scan complete."
