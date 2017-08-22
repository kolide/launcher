# Building Packages

## Building the tool

From the root of the repository, run the following:

```
make deps
make generate
make package-builder
./build/package-builder --help
```

## General Usage

### Creating a set of packages

```
make package-builder
./build/package-builder make --hostname=grpc.launcher.acme.biz:443 --secret=foobar123
```

If you'd like to customize the keys that are used to sign the enrollment secret and macOS package, consider the following usage:

```
make package-builder
./build/package-builder make \
  --hostname=localhost:8082 \
	--secret=foobar123
	--osquery_version=stable \
	--mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```

The macOS package will install a LaunchDaemon that will connect the launcher to the server specified by the `--hostname` flag, using an enrollment secret specified by the `--enrollment_secret` flag. The Linux packages will currently lay down the launcher and osquery binaries as well as the enrollment secret specified by the `--enrollment_secret` flag.

## Kolide Usage

### Authentication

You must be authenticated to Kolide's GCloud organization for the various `package-builder` commands to work. To do this, you will need to install the GCloud tools. Documentation on using and installing these tools can be found [here](https://cloud.google.com/sdk/gcloud/).

To authenticate to GCloud, use the following:

```
gcloud auth application-default login
```

You can also use the `make` shortcut if you prefer:

```
make gcloud-login
```

### Development Packages

To use the tool to generate internal development packages, run:

```
make package-builder
gcloud config set project kolide-ose-testing
./build/package-builder dev --debug
```

If you'd like the resultant macOS packages to be signed, specify the name of the developer key you'd like to use to do the signing. This key must exist in your keychain:

```
./build/package-builder dev --debug --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```

You can also use the `make` shortcut if you prefer:

```
MAC_PACKAGE_SIGNING_KEY="Developer ID Installer: Acme Inc (ABCDEF123456)" make dev-packages
```

This command will build (macOS and Linux) packages for PRs, master, and localhost for the first tenant and upload them to the appropriate bucket in the `kolide-ose-testing` GCloud project.

### Production Packages

To use the tool to generate production packages, run:

```
make package-builder
gcloud config set project kolide-website
./build/package-builder prod --debug \
  --enrollment_secret_signing_key=./key.pem \
  --mac_package_signing_key="Developer ID Installer: Acme Inc (ABCDEF123456)"
```

You can also use the `make` shortcut if you prefer:

```
ENROLLMENT_SECRET_SIGNING_KEY=/path/to/key.pem \
  MAC_PACKAGE_SIGNING_KEY="Developer ID Installer: Acme Inc (ABCDEF123456)" \
  make prod-packages
```

This command will build (macOS and Linux) packages for production and upload them to the appropriate bucket in the `kolide-website` GCloud project.
