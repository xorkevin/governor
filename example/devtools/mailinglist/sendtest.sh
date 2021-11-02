#!/bin/sh

cat ./devtools/mailinglist/testmail.eml | mailcat fmt | mailcat send -s localhost:2525 -i 'kevin@governor.dev.localhost' -o 'xorkevin.test@lists.governor.dev.localhost'
