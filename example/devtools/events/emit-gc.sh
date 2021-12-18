#!/bin/sh

set -e

ROOT=${0%/*}

"$ROOT/publish.sh" 'governor.sys.gc' "{\"timestamp\":$(date '+%s')}"
