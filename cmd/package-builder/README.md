# Building Packages

## Building the tool

From the root of the repository, run the following:

```
make deps
make package-builder
./build/package-builder --help
```

## General Usage

### Logging in

You must be authenticated to Kolide's GCloud organization for the various `package-builder` commands to work. To do this, you will need to install the GCloud tools. Documentation on using and installing these tools can be found [here](https://cloud.google.com/sdk/gcloud/).

To authenticate to GCloud, use the following:

```
gcloud auth application-default login
```

You can also use the `make` shortcut if you prefer:

```
make gcloud-login
```

### Dev Packages

To use the tool to generate development packages, run:

```
make package-builder
gcloud config set project kolide-ose-testing
./build/package-builder dev --debug
```

You can also use the `make` shortcut if you prefer:

```
make dev-packages
```

This command will build (macOS and Linux) packages for PRs, master, and localhost for the first tenant and upload them to the `gs://packaging/` bucket in the `kolide-ose-testing` GCloud project.

### Production Packages

To use the tool to generate production packages, run:

```
make package-builder
gcloud config set project kolide-website
./build/package-builder prod --enrollment_secret_signing_key=./key.pem --debug
```

You can also use the `make` shortcut if you prefer:

```
make prod-packages
```

This command will build (macOS and Linux) packages for production and upload them to the `gs://packaging/` bucket in the `kolide-website` GCloud project.
