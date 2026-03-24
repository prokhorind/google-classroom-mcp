#!/bin/bash
set -e
cd "$(dirname "$0")"
go build -o classroom-mcp .
exec ./classroom-mcp
