# Running Tests

The files in this directory are used only for testing.

The secure enclave keyer requires apple entitlements in order to be able to access the secure enclave to generate keys and perform cryptographic operations. In order to do this we build the secure enclave go tests to a binary, sign that binary with the required MacOS entitlements, then execute the binary and inspect the output. This is all done via the `TestSecureEnclaveTestRunner` function.

In order to add entitlements we first need to create a MacOS app with the following structure:

```sh
launcher_test.app
    └── Contents
        ├── Info.plist
        ├── MacOS
        │   └── launcher_test # <- this is the go test binary mentioned above
        └── embedded.provisionprofile
```

Then we pass the top level directory to the MacOS codsign utility.

In order to succesfully sign the app with entitlements, there are a few steps that must be completed on the machine in order to run the tests.

1. Download and install a certificate from the Apple Developer account of type "Mac Development" https://developer.apple.com/account/resources/certificates/list
2. Add you device to the developer account using the "Provisioning UDID" found at Desktop Menu Applie Icon> About This Mac > More Info > System Report https://developer.apple.com/account/resources/devices/list
3. Create a provisioing profile that includes the device https://developer.apple.com/account/resources/profiles/list ... should probably include all devices on the team and be updated in the repo
4. Replace the `embedded.provisionprofile` file with the new profile

## Skipping Tests

- To skip these tests (e.g. while running tests on a machine which is not included in the provisioning profile), you can set the `SKIP_SECURE_ENCLAVE_TESTS` environment variable to any non-empty value
    - `SKIP_SECURE_ENCLAVE_TESTS=y make test`
