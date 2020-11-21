#!/bin/sh

cmdname=$1
cmddesc=$2

cmdwidth=16
colornorm='\033[m'
colorcmd='\033[1;34m'
colorsec='\033[1m'
colorsub='\033[32m'

printf "${colorsec}NAME${colornorm}\n"
printf "\t$cmdname - $cmddesc\n"

awkprg=$(cat <<'EOF'
/^[#]/ {printf "\n%s%s%s\n", colorsec, $2, colornorm}
/^[^#]/ {printf "\t%s%-*s%s %s\n", colorcmd, cmdwidth, $1, colornorm, $3}
/^[^#].*##.+##/ {printf "\t    Depends on %s%s%s\n", colorsub, $2, colornorm}
EOF
)

cat Makefile \
  | grep '^[^[:space:]].*:.*##\|^##[ ]*' \
  | sed 's/[ ]*##[ ]*/##/' \
  | sed 's/[ ]*:[ ]*/##/' \
  | awk -F '##' \
    -v cmdwidth="$cmdwidth" \
    -v colornorm="$colornorm" \
    -v colorcmd="$colorcmd" \
    -v colorsec="$colorsec" \
    -v colorsub="$colorsub" \
    "$awkprg"
