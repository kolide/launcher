# System Profile Tables

This is a light wrapper around the `system_profile` macOS command. It
supports some basic arguments like `detaillevel` and requested data
types.

Note that some detail levels and data types will have performance
impact if requested.

As the returned data is a complex nested plist, this uses the
[dataflatten](https://godoc.org/github.com/kolide/launcher/pkg/dataflatten)
tooling.


## Examples

Everything, with minimal details:
```
osquery> select count(*) from kolide_system_profiler where datatype like "%" and detaillevel = "mini";
+----------+
| count(*) |
+----------+
| 1270     |
+----------+
```

Multiple data types (slightly redacted):
```
osquery> select * from kolide_system_profiler where datatype in ("SPCameraDataType", "SPiBridgeDataType");
+----------------------+--------+--------------------+------------------------------------------+--------------------+-------+-------------------+-------------+
| fullkey              | parent | key                | value                                    | parentdatatype     | query | datatype          | detaillevel |
+----------------------+--------+--------------------+------------------------------------------+--------------------+-------+-------------------+-------------+
| 0/spcamera_unique-id | 0      | spcamera_unique-id | 0x1111111111111111                       | SPHardwareDataType |       | SPCameraDataType  |             |
| 0/_name              | 0      | _name              | FaceTime HD Camera                       | SPHardwareDataType |       | SPCameraDataType  |             |
| 0/spcamera_model-id  | 0      | spcamera_model-id  | UVC Camera VendorID_1452 ProductID_34304 | SPHardwareDataType |       | SPCameraDataType  |             |
| 0/ibridge_build      | 0      | ibridge_build      | 14Y000                                   | SPHardwareDataType |       | SPiBridgeDataType |             |
| 0/ibridge_model_name | 0      | ibridge_model_name | Apple T1 Security Chip                   | SPHardwareDataType |       | SPiBridgeDataType |             |
| 0/_name              | 0      | _name              | Controller Information                   | SPHardwareDataType |       | SPiBridgeDataType |             |
+----------------------+--------+--------------------+------------------------------------------+--------------------+-------+-------------------+-------------+

```
