# `package-builder`

## Building the tool

From the root of the repository, run the following:

```
make deps
make package-builder
./build/package-builder --help
```

## General Usage

### Dev Command

To use the tool to generate development packages, run:

```
./build/package-builder dev --debug
```

This command will build (macOS) packages for PRs, master, and localhost for the first tenant and upload them to the `gs://packaging/` bucket in the `kolide-ose-testing` gcloud project. You must be authenticated to gcloud with the `kolide-ose-testing` project set as active for this to work.

To authenticate to GCloud, use the following:

```
gcloud auth application-default login
```

To set the `kolide-ose-testing` project as active, use the following:

```
gcloud config set project kolide-ose-testing
```

Documentation on using and installing these tools can be found [here](https://cloud.google.com/sdk/gcloud/).

### Version info

```
./build/package-builder version
```
