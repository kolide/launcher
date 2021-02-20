#!/bin/bash

set -e
set -x

cd $(dirname $0)

##
## Make a good cert combo
##

GOODCNF="[dn]\nCN=localhost\n[req]\ndistinguished_name = dn\n[EXT]\nsubjectAltName=DNS:localhost\nkeyUsage=digitalSignature\nextendedKeyUsage=serverAuth"

openssl req -x509 -out good.crt -keyout good.key \
        -newkey rsa:2048 -nodes -sha256 -days 9999 \
        -subj '/CN=localhost' -extensions EXT\
        -config <(printf "$GOODCNF")

##
## Make a root ca / chain setup
##

cd certchain

openssl req -x509 -new -nodes -sha256 -days 99999 \
        -keyout root.key  -out root.crt \
        -extensions v3_req \
        -config root.cnf


# Intermediate

openssl genrsa -out intermediate.key 2048
openssl req \
        -new -key intermediate.key \
        -out intermediate.csr \
        -config intermediate.cnf

# Sign the CSR by our CA.
openssl x509 -req -sha256 -days 9999 \
  -in intermediate.csr \
  -CA root.crt -CAkey root.key \
  -CAcreateserial \
  -extensions v3_req \
  -extfile intermediate.cnf \
  -out intermediate.crt

# Leaf

openssl genrsa -out leaf.key 2048
openssl req \
        -new -key leaf.key \
        -out leaf.csr \
        -config leaf.cnf

# Sign the CSR by our CA.
openssl x509 -req -sha256 -days 9999 \
  -in leaf.csr \
  -CA intermediate.crt -CAkey intermediate.key \
  -CAcreateserial \
  -extensions v3_req \
  -extfile leaf.cnf \
  -out leaf.crt

# And finally, the chain
cat leaf.crt intermediate.crt root.crt > chain.pem
