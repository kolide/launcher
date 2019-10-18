#!/bin/bash


# FIXME: this entire file should be deleted and merged with make.sh
set -e

function rand {
    dd if=/dev/random bs=1 count=16 2> /dev/null  | base64
}

function makeUUID {
    # od -x /dev/urandom | head -1 | awk '{OFS="-"; srand($6); sub(/./,"4",$5); sub(/./,substr("89ab",rand()*4,1),$6); print $2$3,$4,$5,$6,$7$8$9}'
    ruby -r securerandom -e 'print SecureRandom.uuid'
}

# generate some keys and the corresponding json spec files for testing fingerprints
function makeOpensshKeyAndSpec {
    type=$1
    bits=$2
    format="openssh7"

    keyfile=$(makeUUID)
    keypath="fingerprints/$keyfile"

    ssh-keygen -t $type -b $bits  -C "" -f $keypath -P /dev/null
    ssh-keygen -t $type -b $bits  -C "" -f $keypath-encrypted -P "$(rand)"

    for key in $keyfile $keyfile-encrypted; do
        fingerprint=$(ssh-keygen -l -f fingerprints/$key | awk '{print $2}')
        md5fingerprint=$(ssh-keygen -l -E md5 -f fingerprints/$key | awk '{print $2}' | sed 's/^MD5://')

        encrypted=false
        if [ "$key" == "$keyfile-encrypted" ]; then
            encrypted=true
        fi
        echo "{ \"ExpectedFingerprintSHA256\": \"$fingerprint\",\
\"ExpectedFingerprintMD5\": \"$md5fingerprint\",\
\"Type\": \"$type\",\
\"Bits\": $bits,\
\"Encrypted\": $encrypted,\
 \"KeyPath\": \"$key\"}" > fingerprints/$key.json
    done
}

makeOpensshKeyAndSpec rsa 1024
makeOpensshKeyAndSpec rsa 2048
makeOpensshKeyAndSpec rsa 4096
makeOpensshKeyAndSpec dsa 1024
makeOpensshKeyAndSpec ecdsa 256
# makeOpensshKeyAndSpec ecdsa 521


function makePuttyKeyAndSpec {
    type=$1
    bits=$2
    format="putty"

    keyfile=$type-$bits-$format
    keypath=fingerprints/$keyfile

    puttygen -t $type -b $bits -C "" -o $keypath -O private --new-passphrase /dev/null
    puttygen -t $type -b $bits -C "" -o $keypath-encrypted -O private --new-passphrase <(rand)

    # fingerprint=$(puttygen -l $keypath | cut -d' ' -f3)
    # echo "{ \"ExpectedFingerprint\": \"$fingerprint\", \"KeyPath\": \"$keyfile\"}" > $keypath.json

    for key in $keyfile $keyfile-encrypted; do
        fingerprint=$(puttygen -l "fingerprints/$key" | cut -d' ' -f3)
        echo "{ \"ExpectedFingerprint\": \"$fingerprint\", \"KeyPath\": \"$key\"}" > "fingerprints/$key.json"
    done
}

# makePuttyKeyAndSpec rsa 1024
# makePuttyKeyAndSpec rsa1 2048
# makePuttyKeyAndSpec dsa 2048
# makePuttyKeyAndSpec ecdsa 256
# makePuttyKeyAndSpec ed25519 256
