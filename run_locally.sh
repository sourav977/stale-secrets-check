#!/bin/bash

# Function to display a progress bar
progress_bar() {
    local duration=$1
    local elapsed=0
    echo "waiting for $duration seconds==" 
    while [ $elapsed -lt $duration ]; do
        # Calculate remaining time
        remaining=$((duration - elapsed))

        # Print progress bar
        printf "\r["
        printf "%${elapsed}s" | tr ' ' '='
        printf "%${remaining}s" | tr ' ' ' '
        printf "] %d/%d seconds" $elapsed $duration

        # Print "=" in green color
        printf "\e[32m=\e[32m"

        sleep 1
        ((elapsed++))
    done
    echo ""
}

# Run 'make build'
make
make manifests
make bundle  
make docker-build
make install

# Display progress bar while waiting
echo "Waiting for 60 seconds..."
progress_bar 60

# Continuously check if cert-manager API is ready
echo "Checking if the cert-manager API is ready..."
while :
do
    output=$(cmctl check api)
    if [[ "$output" == *"The cert-manager API is ready"* ]]; then
        echo "The cert-manager API is ready"
        break
    fi
    echo "The cert-manager API is not ready. Waiting..."
    progress_bar 30
done

# Run 'make deploy'
make deploy
