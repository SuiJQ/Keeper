#!/usr/bin/env bash
set -euo pipefail

# Network proxy and port forwarding example.
# Prerequisites: built ./bin/keeper, config.json with network settings

KEEPER="./bin/keeper"

# Start an agent with port forwarding
${KEEPER} create net-agent
${KEEPER} start net-agent

# Example: forward host port 8080 to container port 80
# This requires Keeper to support port forwarding configuration.
# Consult config.json for:
# - network_forward_max_connections
# - network_forward_connect_timeout

echo "==> inspect networking"
${KEEPER} inspect net-agent --verbose || true

echo "==> stop agent"
${KEEPER} stop net-agent
${KEEPER} destroy net-agent
