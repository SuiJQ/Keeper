#!/usr/bin/env bash
set -euo pipefail

# Basic Keeper workflow example.
# Prerequisites: built ./bin/keeper

KEEPER="./bin/keeper"

echo "==> create agent"
${KEEPER} create demo-agent

echo "==> start agent"
${KEEPER} start demo-agent

echo "==> status"
${KEEPER} status demo-agent

echo "==> inspect"
${KEEPER} inspect demo-agent

echo "==> copy file into agent"
${KEEPER} cp ./README.md demo-agent:/workspace/README.md

echo "==> run command inside agent"
${KEEPER} run demo-agent -- ls -la /workspace

echo "==> stop agent"
${KEEPER} stop demo-agent

echo "==> fork agent"
${KEEPER} fork demo-agent demo-agent-copy

echo "==> destroy copy"
${KEEPER} destroy demo-agent-copy

echo "==> destroy original"
${KEEPER} destroy demo-agent
