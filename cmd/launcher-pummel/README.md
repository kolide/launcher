# Osquery Server Load Testing

## Building the tool

From the root of the repository, run the following:

```
make deps
make launcher-pummel
./build/launcher-pummel --help
```

## Tool Usage Instructions

```
./build/launcher-pummel \
  --host_path=./path/to/my/host/templates \
	--server_url=fleet.acme.co \
	--enroll_secret=mB3XE5kwLt3YryD9FAanjwhm02HoOqll \
	--hosts=mac:100,windows:20,linux:5000
```

You can also define the enroll secret via a file path (`--enroll_secret_path`) or an environment variable (`ENROLL_SECRET`). See `launcher-pummel --help` for more information.
