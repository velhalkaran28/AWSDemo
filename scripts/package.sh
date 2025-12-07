#!/bin/sh
set -e

echo ""
echo "PACKAGING Golang binaries..."
for model in cmd/*; do
    # If directory
    if [ -d "$model" ]; then
        echo ... cmd $(basename $model)
        for package in $model/*; do
            echo ... ... package $(basename $package)
            # If a main.go file exists
            if [ -f "./cmd/$(basename $model)/$(basename $package)/main.go" ]; then
                CGO_ENABLED=0 GOOS=linux go build -o bootstrap ./cmd/$(basename $model)/$(basename $package)
                mkdir -p bin/$(basename $model)/$(basename $package) && zip -r bin/$(basename $model)/$(basename $package)/$(basename $model)-$(basename $package).zip bootstrap ./template/*
            else
                echo "... ... ... no main found (ignoring)"
            fi
        done
    fi
done