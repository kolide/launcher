#!/bin/bash

# This script uses several tools to create ssh keys. We attempt to be
# as exhaustive as possible to create a wide range of things to test.
#
# each function also generates a json spec file that describes various
# attributes of a key, plus the name of the key described. This spec file is
# consumed by golang tests, and the output of the keyidentifier package is
# compared against these expected values.

set -e
#set -x

function rand {
    dd if=/dev/random bs=1 count=16 2> /dev/null  | base64 | sed 's/[^0-9a-zA-Z]//g'
}

DATA_DIR="fingerprints" # TODO: change me

# generate some keys and the corresponding json spec files for testing fingerprints
function makeOpensshKeyAndSpec {
    type=$1
    bits=$2
    encrypted=$3
    format="openssh7"

    keyfile=$(rand)
    keypath="$DATA_DIR/$keyfile"


    if [ $encrypted == true ]; then
        ssh-keygen -t $type -b $bits  -C "" -f $keypath -P "$(rand)"
        cmd='ssh-keygen -t $type -b $bits  -C \"\" -f $keypath -P $(rand)'
    else
        ssh-keygen -t $type -b $bits  -C "" -f $keypath -P ""
        cmd='ssh-keygen -t $type -b $bits  -C \"\" -f $keypath -P \"\"'
    fi

    fingerprint=$(ssh-keygen -l -f $keypath | awk '{print $2}')
    md5fingerprint=$(ssh-keygen -l -E md5 -f $keypath | awk '{print $2}' | sed 's/^MD5://')

    cat <<EOF > $keypath.json
    {
      "ExpectedFingerprintSHA256": "$fingerprint",
      "ExpectedFingerprintMD5": "$md5fingerprint",
      "Type": "$type",
      "Bits": $bits,
      "Encrypted": $encrypted,
      "KeyPath": "$keyfile",
      "command": "$cmd"
    }
EOF
}

# ssh.com style
#
# Note that openssh's ssh-keygen proportes to convert (using `-e`) but
# empirically this does not work. So we use puttygen (`brew install
# putty` to generate these)

# check if puttygen is installed
hash puttygen 2>/dev/null || \
    { echo >&2 "puttygen must be installed to generate test data for putty keys. use 'brew install putty' to install puttygen on macos"; exit 1; }

function makePuttyKeyAndSpecFile {
    type=$1
    bits=$2
    format=$3
    encrypted=$4

    if [ "$type" == "rsa1" ]; then
        format="ssh1"
    fi

    if [ "$format" == "putty" ]; then
        format=""
    else
        format="-$format"
    fi

    keyfile=$(rand)
    keypath="$DATA_DIR/$keyfile"

    if [ $encrypted == true ]; then
        puttygen -t $type -b $bits -C "" -o $keypath -O private$format --new-passphrase <(rand)
        cmd='puttygen -t $type -b $bits -C \"\" -o $keypath -O private --new-passphrase <(rand)'
    else
        puttygen -t $type -b $bits -C "" -o $keypath -O private$format --new-passphrase <(cat /dev/null)
        cmd='puttygen -t $type -b $bits -C "" -o $keypath -O private --new-passphrase <(cat /dev/null)'
    fi

    fingerprint="" # puttygen doesn't seem to support sha256 fingerprints
    md5fingerprint=$(puttygen -l $keypath | awk '{print $3}')

    cat <<EOF > $keypath.json
    {
      "ExpectedFingerprintSHA256": "$fingerprint",
      "ExpectedFingerprintMD5": "$md5fingerprint",
      "Type": "$type",
      "Bits": $bits,
      "Encrypted": $encrypted,
      "KeyPath": "$keyfile",
      "command": "$cmd"
    }
EOF
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

# -------------------------------------------------------------------------------------------
# Actually make all the keys now that the functions have been defined
# -------------------------------------------------------------------------------------------

makeOpensshKeyAndSpec rsa 1024 true
makeOpensshKeyAndSpec rsa 1024 false

makeOpensshKeyAndSpec rsa 2048 true
makeOpensshKeyAndSpec rsa 2048 false

makeOpensshKeyAndSpec rsa 4096 true
makeOpensshKeyAndSpec rsa 4096 false

makeOpensshKeyAndSpec dsa 1024 true
makeOpensshKeyAndSpec dsa 1024 false

makeOpensshKeyAndSpec ecdsa 256 true
makeOpensshKeyAndSpec ecdsa 256 false

makeOpensshKeyAndSpec ecdsa 521 true
makeOpensshKeyAndSpec ecdsa 521 false


# don't gen the non-openssh keys yet
exit 1

# rsa1 is only supported in the old old old format
makePuttyKeyAndSpecFile rsa1 1024 putty true
makePuttyKeyAndSpecFile rsa1 1024 putty false

makePuttyKeyAndSpecFile rsa1 2048 putty true
makePuttyKeyAndSpecFile rsa1 2048 putty false

for format in openssh openssh-new sshcom putty; do
    makePuttyKeyAndSpecFile rsa 1024 $format true
    makePuttyKeyAndSpecFile rsa 1024 $format false

    makePuttyKeyAndSpecFile rsa 2048 $format true
    makePuttyKeyAndSpecFile rsa 2048 $format false

    makePuttyKeyAndSpecFile rsa 4096 $format true
    makePuttyKeyAndSpecFile rsa 4096 $format false

    makePuttyKeyAndSpecFile dsa 1024 $format true
    makePuttyKeyAndSpecFile dsa 1024 $format false
done

for format in openssh openssh-new putty; do

    makePuttyKeyAndSpecFile ecdsa 256 $format true
    makePuttyKeyAndSpecFile ecdsa 256 $format false

    makePuttyKeyAndSpecFile ecdsa 521 $format true
    makePuttyKeyAndSpecFile ecdsa 521 $format false

    makePuttyKeyAndSpecFile ed25519 256 $format true
    makePuttyKeyAndSpecFile ed25519 256 $format false
done

# openssl genpkey -algorithm RSA -out private_key.pem -pkeyopt rsa_keygen_bits:2048
# openssl genpkey -algorithm RSA -pass pass:password -out private_key_enc.pem -pkeyopt rsa_keygen_bits:2048


