#!/bin/sh
set -e

# Start nginx in background
nginx -g 'daemon off;' & NGINX_PID=$!

# Start backend
/app/releasehub & BACKEND_PID=$!

# Wait for either to exit
wait -n "$NGINX_PID" "$BACKEND_PID"
EXIT_CODE=$?

# Stop the other
kill "$NGINX_PID" "$BACKEND_PID" 2>/dev/null || true
exit "$EXIT_CODE"
