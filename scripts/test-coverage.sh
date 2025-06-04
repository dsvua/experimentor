#!/bin/bash

# Test coverage script for experimentor
# Runs tests and generates coverage reports

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Create coverage directory
COVERAGE_DIR="coverage"
mkdir -p "$COVERAGE_DIR"

print_status "Setting up test environment..."

# Set up envtest if needed
if ! command -v setup-envtest &> /dev/null; then
    print_warning "setup-envtest not found, installing..."
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
fi

# Setup test environment
export KUBEBUILDER_ASSETS="$(setup-envtest use --print path)"
if [ -z "$KUBEBUILDER_ASSETS" ]; then
    print_error "Failed to setup test environment"
    exit 1
fi

print_status "Running unit tests with coverage..."

# Run unit tests with coverage
go test ./... \
    -coverprofile="$COVERAGE_DIR/coverage.out" \
    -covermode=atomic \
    -race \
    -v

# Check if tests passed
if [ $? -ne 0 ]; then
    print_error "Tests failed"
    exit 1
fi

print_status "Generating coverage reports..."

# Generate HTML coverage report
go tool cover -html="$COVERAGE_DIR/coverage.out" -o "$COVERAGE_DIR/coverage.html"

# Generate function coverage report
go tool cover -func="$COVERAGE_DIR/coverage.out" > "$COVERAGE_DIR/coverage.txt"

# Print coverage summary
print_status "Coverage Summary:"
echo "===================="
tail -n 1 "$COVERAGE_DIR/coverage.txt"
echo "===================="

# Extract overall coverage percentage
COVERAGE_PERCENT=$(tail -n 1 "$COVERAGE_DIR/coverage.txt" | grep -o '[0-9.]*%' | head -1)
COVERAGE_NUM=$(echo "$COVERAGE_PERCENT" | grep -o '[0-9.]*' | head -1)

print_status "Detailed coverage report saved to: $COVERAGE_DIR/coverage.html"
print_status "Text coverage report saved to: $COVERAGE_DIR/coverage.txt"

# Check coverage threshold
THRESHOLD=75
if (( $(echo "$COVERAGE_NUM >= $THRESHOLD" | bc -l) )); then
    print_status "Coverage is above threshold ($THRESHOLD%): $COVERAGE_PERCENT"
else
    print_warning "Coverage is below threshold ($THRESHOLD%): $COVERAGE_PERCENT"
fi

# Run benchmark tests
print_status "Running benchmark tests..."
go test ./internal/controller -bench=. -benchmem -run=^$ > "$COVERAGE_DIR/benchmarks.txt" 2>&1 || true

if [ -f "$COVERAGE_DIR/benchmarks.txt" ]; then
    print_status "Benchmark results saved to: $COVERAGE_DIR/benchmarks.txt"
fi

print_status "Coverage analysis complete!"
print_status "Open $COVERAGE_DIR/coverage.html in your browser to view detailed coverage"