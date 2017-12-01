# Osquery Server Load Testing

## Building the tool

From the root of the repository, run the following:

```
make deps
make launcher-pummel
./build/launcher-pummel --help
```

## Usage Instructions

```
./build/launcher-pummel \
  --host_path=./path/to/my/host/templates \
	--server_url=fleet.acme.co \
	--enroll_secret=mB3XE5wwLt3YryD9FAanjwhm02HoOqll \
	mac:100 windows:20 linux:5000
```
