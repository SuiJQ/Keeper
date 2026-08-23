#!/usr/bin/env bash
set -euo pipefail

# Snapshot and rollback example.
# Prerequisites: built ./bin/keeper

KEEPER="./bin/keeper"

${KEEPER} create snapshot-agent
${KEEPER} start snapshot-agent

${KEEPER} cp ./init-data snapshot-agent:/workspace/data/

echo "==> create snapshot"
SNAPSHOT_ID=$(${KEEPER} snapshot snapshot-agent || true)

echo "==> modify data"
${KEEPER} run snapshot-agent -- sh -c "echo changed > /workspace/data/marker.txt"

echo "==> rollback"
if [ -n "${SNAPSHOT_ID:-}" ]; then
  ${KEEPER} rollback snapshot-agent "${SNAPSHOT_ID}" || true
fi

echo "==> stop and cleanup"
${KEEPER} stop snapshot-agent
${KEEPER} destroy snapshot-agent
