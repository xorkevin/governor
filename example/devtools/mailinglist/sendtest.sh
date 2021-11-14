#!/bin/sh

ROOT=${0%/*}

cat "$ROOT/testmail.eml" \
  | mailcat fmt \
  | mailcat send \
      -s localhost:2525 \
      -i 'return-path@mail.governor.dev.localhost' \
      -o 'xorkevin.test@lists.governor.dev.localhost' \
      --dkim-selector tests \
      --dkim-keyfile "$ROOT/dkimkey.pem"
