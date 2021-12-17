#!/bin/sh

set -e

ROOT=${0%/*}

inreplyto=$1
references=$2

args=
if [ ! -z "$inreplyto" ]; then
  if [ ! -z "$references" ]; then
    args=(-s "In-Reply-To: <$inreplyto>" -s "References: $references")
  else
    args=(-s "In-Reply-To: <$inreplyto>" -s "References: <$inreplyto>")
  fi
fi

cat "$ROOT/testmail.eml" \
  | mailcat fmt "${args[@]}" \
  | mailcat send \
      -s localhost:2525 \
      -i 'return-path@mail.governor.dev.localhost' \
      -o 'xorkevin.test@lists.governor.dev.localhost' \
      --dkim-selector tests \
      --dkim-keyfile "$ROOT/dkimkey.pem"
