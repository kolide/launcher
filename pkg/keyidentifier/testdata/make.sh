#!/bin/bash

# This script uses several tools to create ssh keys. We attempt to be
# as exhaustive as possible to create a wide range of things to test.
#
# This puts them into a somewhat ugly filesystem hierarchy. The intent
# is to group things to make it simpler for tools to test
# groups. The layout should straightforward.



set -e
#set -x

function rand {
    dd if=/dev/random bs=1 count=16 2> /dev/null  | base64
}

# My openssh is so new, it only makes the new format
function makeOpensshKey {
    type=$1
    bits=$2
    os="openssh7"
    dir_fragment="$type/$bits/openssh-new"
    mkdir -p {encrypted,plaintext}"/$dir_fragment"
    ssh-keygen -t $type -b $bits  -C "" -f "plaintext/$dir_fragment/seph-$os" -P ""
    ssh-keygen -t $type -b $bits  -C "" -f "encrypted/$dir_fragment/seph-$os" -P "$(rand)"
}

makeOpensshKey rsa 1024
makeOpensshKey rsa 2048
makeOpensshKey rsa 4096

makeOpensshKey dsa 1024

makeOpensshKey ecdsa 256
makeOpensshKey ecdsa 521

# ed25519 is fixed legth, bits is ignored
makeOpensshKey ed25519 2048

# ssh.com style
#
# Note that openssh's ssh-keygen proportes to convert (using `-e`) but
# empirically this does not work. So we use puttygen (`brew install
# putty` to generate these)

# check if puttygen is installed
hash puttygen 2>/dev/null || \
    { echo >&2 "puttygen must be installed to generate test data for putty keys. use 'brew install putty' to install puttygen on macos"; exit 1; }

function makePuttyKeyPuttyFormat {
    type=$1
    bits=$2
    format="putty"

    if [ "$type" == "rsa1" ]; then
        format="ssh1"
    fi

    dir_fragment="$type/$bits/$format"
    mkdir -p {encrypted,plaintext}"/$dir_fragment"

    puttygen -t $type -b $bits -C "" -o "plaintext/$dir_fragment/seph-putty" -O private --new-passphrase /dev/null
    puttygen -t $type -b $bits -C "" -o "encrypted/$dir_fragment/seph-putty" -O private --new-passphrase <(rand)
}


function makePuttyKey {
    type=$1
    bits=$2
    formats="$3"

    makePuttyKeyPuttyFormat $type $bits

    for format in $3; do
        dir_fragment="$type/$bits/$format"
        mkdir -p {encrypted,plaintext}"/$dir_fragment"

        puttygen -t $type -b $bits -C "" -o "plaintext/$dir_fragment/seph-putty" -O private-$format --new-passphrase /dev/null
        puttygen -t $type -b $bits -C "" -o "encrypted/$dir_fragment/seph-putty" -O private-$format --new-passphrase <(rand)
    done
}

# rsa1 is only supported in the old old old format
makePuttyKey rsa1 1024
makePuttyKey rsa1 2048

makePuttyKey rsa 1024 "openssh openssh-new sshcom"
makePuttyKey rsa 2048 "openssh openssh-new sshcom"
makePuttyKey rsa 4096 "openssh openssh-new sshcom"

makePuttyKey dsa 1024 "openssh openssh-new sshcom"

makePuttyKey ecdsa 256 "openssh openssh-new"
makePuttyKey ecdsa 521 "openssh openssh-new"

makePuttyKey ed25519 256 "openssh openssh-new"

# openssl genpkey -algorithm RSA -out private_key.pem -pkeyopt rsa_keygen_bits:2048
# openssl genpkey -algorithm RSA -pass pass:password -out private_key_enc.pem -pkeyopt rsa_keygen_bits:2048


