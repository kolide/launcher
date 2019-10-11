#!/usr/bin/env bash

# Script to be used for generating testing certs only for notary-server and notary-signer
# Will also create a root-ca and intermediate-ca

set -e

mkdir keys
cd keys

OPENSSLCNF=
for path in /etc/openssl/openssl.cnf /etc/ssl/openssl.cnf /usr/local/etc/openssl/openssl.cnf; do
    if [[ -e ${path} ]]; then
        OPENSSLCNF=${path}
    fi
done
if [[ -z ${OPENSSLCNF} ]]; then
    printf "Could not find openssl.cnf"
    exit 1
fi

# First generates root-ca
openssl genrsa -out "root-ca.key" 4096
openssl req -new -key "root-ca.key" -out "root-ca.csr" -sha256 \
        -subj '/C=US/ST=MA/L=Cambridge/O=Kolide/CN=Notary CA'

cat > "root-ca.cnf" <<EOL
[root_ca]
basicConstraints = critical,CA:TRUE,pathlen:1
keyUsage = critical, nonRepudiation, cRLSign, keyCertSign
subjectKeyIdentifier=hash
EOL

openssl x509 -req -days 3650 -in "root-ca.csr" -signkey "root-ca.key" -sha256 \
        -out "root-ca.crt" -extfile "root-ca.cnf" -extensions root_ca

# Then generate intermediate-ca
openssl genrsa -out "intermediate-ca.key" 4096
openssl req -new -key "intermediate-ca.key" -out "intermediate-ca.csr" -sha256 \
        -subj '/C=US/ST=MA/L=Cambridge/O=Kolide/CN=Notary Intermediate CA'

cat > "intermediate-ca.cnf" <<EOL
[intermediate_ca]
authorityKeyIdentifier=keyid,issuer
basicConstraints = critical,CA:TRUE,pathlen:0
extendedKeyUsage=serverAuth,clientAuth
keyUsage = critical, nonRepudiation, cRLSign, keyCertSign
subjectKeyIdentifier=hash
EOL

openssl x509 -req -days 3650 -in "intermediate-ca.csr" -sha256 \
        -CA "root-ca.crt" -CAkey "root-ca.key"  -CAcreateserial \
        -out "intermediate-ca.crt" -extfile "intermediate-ca.cnf" -extensions intermediate_ca

# Then generate notary-server
openssl genrsa -out "notary-server.key" 4096

openssl req -new -key "notary-server.key" -out "notary-server.csr" -sha256 \
        -subj '/C=US/ST=MA/L=Cambridge/O=Kolide/CN=notaryserver'

cat > "notary-server.cnf" <<EOL
[notary_server]
authorityKeyIdentifier=keyid,issuer
basicConstraints = critical,CA:FALSE
extendedKeyUsage=serverAuth,clientAuth
keyUsage = critical, digitalSignature, keyEncipherment
subjectAltName = DNS:notary-server, DNS:notaryserver, DNS:localhost, IP:127.0.0.1
subjectKeyIdentifier=hash
EOL

openssl x509 -req -days 750 -in "notary-server.csr" -sha256 \
        -CA "intermediate-ca.crt" -CAkey "intermediate-ca.key"  -CAcreateserial \
        -out "notary-server.crt" -extfile "notary-server.cnf" -extensions notary_server
# append the intermediate cert to this one to make it a proper bundle
cat "intermediate-ca.crt" >> "notary-server.crt"

# Then generate notary-signer
openssl genrsa -out "notary-signer.key" 4096

openssl req -new -key "notary-signer.key" -out "notary-signer.csr" -sha256 \
        -subj '/C=US/ST=MA/L=Cambridge/O=Kolide/CN=notary-signer'

cat > "notary-signer.cnf" <<EOL
[notary_signer]
authorityKeyIdentifier=keyid,issuer
basicConstraints = critical,CA:FALSE
extendedKeyUsage=serverAuth,clientAuth
keyUsage = critical, digitalSignature, keyEncipherment
subjectAltName = DNS:notary-signer, DNS:notarysigner, DNS:localhost, IP:127.0.0.1
subjectKeyIdentifier=hash
EOL

openssl x509 -req -days 750 -in "notary-signer.csr" -sha256 \
        -CA "intermediate-ca.crt" -CAkey "intermediate-ca.key"  -CAcreateserial \
        -out "notary-signer.crt" -extfile "notary-signer.cnf" -extensions notary_signer
# append the intermediate cert to this one to make it a proper bundle
cat "intermediate-ca.crt" >> "notary-signer.crt"

# Then generate notary-escrow
openssl genrsa -out "notary-escrow.key" 4096
# Use the existing notary-escrow key
openssl req -new -key "notary-escrow.key" -out "notary-escrow.csr" -sha256 \
        -subj '/C=US/ST=MA/L=Cambridge/O=Kolide/CN=notary-escrow'

cat > "notary-escrow.cnf" <<EOL
[notary_escrow]
authorityKeyIdentifier=keyid,issuer
basicConstraints = critical,CA:FALSE
extendedKeyUsage=serverAuth,clientAuth
keyUsage = critical, digitalSignature, keyEncipherment
subjectAltName = DNS:notary-escrow, DNS:notaryescrow, DNS:localhost, IP:127.0.0.1
subjectKeyIdentifier=hash
EOL

openssl x509 -req -days 750 -in "notary-escrow.csr" -sha256 \
        -CA "intermediate-ca.crt" -CAkey "intermediate-ca.key"  -CAcreateserial \
        -out "notary-escrow.crt" -extfile "notary-escrow.cnf" -extensions notary_escrow
# append the intermediate cert to this one to make it a proper bundle
cat "intermediate-ca.crt" >> "notary-escrow.crt"
