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
./build/package-builder dev --help
```

This command will build (macOS) packages for PR 350-399, master, and localhost for the first three tenants and upload them to the `gs://packaging/` bucket in the `kolide-ose-testing` gcloud project. You must be authenticated to gcloud with the `kolide-ose-testing` project set as active for this to work.

To authenticate to GCloud, use the following:

```
gcloud auth application-default login
```

To set the `kolide-ose-testing` project as active, use the following:

```
gcloud config set project kolide-ose-testing
```

Documentation on using and installing these tools can be found [here](https://cloud.google.com/sdk/gcloud/).

### Mirror Command

Osquery and Launcher binaries must be available on a remote mirror so the autoupdate can download them. The binaries also need to be available when building a new platform specific package. 
Binaries are packaged in a tarball in the format `<binary-name>-<update-channel or version>.tar.gz`
Example:
    ```
    osqueryd-stable.tar.gz
    osqueryd-2.6.0.tar.gz
    ```
The tarball are stored in the `gs://binaries-for-launcher` GCS bucket, and exposed at the `https://dl.kolide.com/kolide/<binary>/<platform>/<tarball>` url.

The `mirror` subcommand is used to help download and extract platform specific binaries from the osquery website.

```
./build/package-builder mirror --help
```

TODO: right now the `mirror` command only creates osqueryd tarballs, but should support packaging and uploading launcher binaries as well soon.

### Version info

```
./build/package-builder version
```
