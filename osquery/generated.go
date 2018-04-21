package osquery

import (
	"context"

	"github.com/kolide/osquery-go/plugin/table"
)

func table_acpi_tables() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("size"),

		table.TextColumn("md5"),
	}
	return table.NewPlugin("acpi_tables", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"size": "",

			"md5": "",
		}}, nil
	})
}

func table_ad_config() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("domain"),

		table.TextColumn("option"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("ad_config", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"domain": "",

			"option": "",

			"value": "",
		}}, nil
	})
}

func table_alf() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("allow_signed_enabled"),

		table.TextColumn("firewall_unload"),

		table.TextColumn("global_state"),

		table.TextColumn("logging_enabled"),

		table.TextColumn("logging_option"),

		table.TextColumn("stealth_enabled"),

		table.TextColumn("version"),
	}
	return table.NewPlugin("alf", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"allow_signed_enabled": "",

			"firewall_unload": "",

			"global_state": "",

			"logging_enabled": "",

			"logging_option": "",

			"stealth_enabled": "",

			"version": "",
		}}, nil
	})
}

func table_alf_exceptions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("state"),
	}
	return table.NewPlugin("alf_exceptions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"state": "",
		}}, nil
	})
}

func table_alf_explicit_auths() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("process"),
	}
	return table.NewPlugin("alf_explicit_auths", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"process": "",
		}}, nil
	})
}

func table_alf_services() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("service"),

		table.TextColumn("process"),

		table.TextColumn("state"),
	}
	return table.NewPlugin("alf_services", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"service": "",

			"process": "",

			"state": "",
		}}, nil
	})
}

func table_app_schemes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("scheme"),

		table.TextColumn("handler"),

		table.TextColumn("enabled"),

		table.TextColumn("external"),

		table.TextColumn("protected"),
	}
	return table.NewPlugin("app_schemes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"scheme": "",

			"handler": "",

			"enabled": "",

			"external": "",

			"protected": "",
		}}, nil
	})
}

func table_appcompat_shims() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("executable"),

		table.TextColumn("path"),

		table.TextColumn("description"),

		table.TextColumn("install_time"),

		table.TextColumn("type"),

		table.TextColumn("sdb_id"),
	}
	return table.NewPlugin("appcompat_shims", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"executable": "",

			"path": "",

			"description": "",

			"install_time": "",

			"type": "",

			"sdb_id": "",
		}}, nil
	})
}

func table_apps() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("path"),

		table.TextColumn("bundle_executable"),

		table.TextColumn("bundle_identifier"),

		table.TextColumn("bundle_name"),

		table.TextColumn("bundle_short_version"),

		table.TextColumn("bundle_version"),

		table.TextColumn("bundle_package_type"),

		table.TextColumn("environment"),

		table.TextColumn("element"),

		table.TextColumn("compiler"),

		table.TextColumn("development_region"),

		table.TextColumn("display_name"),

		table.TextColumn("info_string"),

		table.TextColumn("minimum_system_version"),

		table.TextColumn("category"),

		table.TextColumn("applescript_enabled"),

		table.TextColumn("copyright"),

		table.TextColumn("last_opened_time"),
	}
	return table.NewPlugin("apps", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"path": "",

			"bundle_executable": "",

			"bundle_identifier": "",

			"bundle_name": "",

			"bundle_short_version": "",

			"bundle_version": "",

			"bundle_package_type": "",

			"environment": "",

			"element": "",

			"compiler": "",

			"development_region": "",

			"display_name": "",

			"info_string": "",

			"minimum_system_version": "",

			"category": "",

			"applescript_enabled": "",

			"copyright": "",

			"last_opened_time": "",
		}}, nil
	})
}

func table_apt_sources() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("base_uri"),

		table.TextColumn("package_cache_file"),

		table.TextColumn("release"),

		table.TextColumn("component"),

		table.TextColumn("version"),

		table.TextColumn("maintainer"),

		table.TextColumn("site"),
	}
	return table.NewPlugin("apt_sources", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"base_uri": "",

			"package_cache_file": "",

			"release": "",

			"component": "",

			"version": "",

			"maintainer": "",

			"site": "",
		}}, nil
	})
}

func table_arp_cache() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("address"),

		table.TextColumn("mac"),

		table.TextColumn("interface"),

		table.TextColumn("permanent"),
	}
	return table.NewPlugin("arp_cache", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"address": "",

			"mac": "",

			"interface": "",

			"permanent": "",
		}}, nil
	})
}

func table_asl() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("time"),

		table.TextColumn("time_nano_sec"),

		table.TextColumn("host"),

		table.TextColumn("sender"),

		table.TextColumn("facility"),

		table.TextColumn("pid"),

		table.TextColumn("gid"),

		table.TextColumn("uid"),

		table.TextColumn("level"),

		table.TextColumn("message"),

		table.TextColumn("ref_pid"),

		table.TextColumn("ref_proc"),

		table.TextColumn("extra"),
	}
	return table.NewPlugin("asl", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"time": "",

			"time_nano_sec": "",

			"host": "",

			"sender": "",

			"facility": "",

			"pid": "",

			"gid": "",

			"uid": "",

			"level": "",

			"message": "",

			"ref_pid": "",

			"ref_proc": "",

			"extra": "",
		}}, nil
	})
}

func table_augeas() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("node"),

		table.TextColumn("value"),

		table.TextColumn("label"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("augeas", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"node": "",

			"value": "",

			"label": "",

			"path": "",
		}}, nil
	})
}

func table_authenticode() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("original_program_name"),

		table.TextColumn("serial_number"),

		table.TextColumn("issuer_name"),

		table.TextColumn("subject_name"),

		table.TextColumn("result"),
	}
	return table.NewPlugin("authenticode", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"original_program_name": "",

			"serial_number": "",

			"issuer_name": "",

			"subject_name": "",

			"result": "",
		}}, nil
	})
}

func table_authorization_mechanisms() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("label"),

		table.TextColumn("plugin"),

		table.TextColumn("mechanism"),

		table.TextColumn("privileged"),

		table.TextColumn("entry"),
	}
	return table.NewPlugin("authorization_mechanisms", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"label": "",

			"plugin": "",

			"mechanism": "",

			"privileged": "",

			"entry": "",
		}}, nil
	})
}

func table_authorizations() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("label"),

		table.TextColumn("modified"),

		table.TextColumn("allow_root"),

		table.TextColumn("timeout"),

		table.TextColumn("version"),

		table.TextColumn("tries"),

		table.TextColumn("authenticate_user"),

		table.TextColumn("shared"),

		table.TextColumn("comment"),

		table.TextColumn("created"),

		table.TextColumn("class"),

		table.TextColumn("session_owner"),
	}
	return table.NewPlugin("authorizations", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"label": "",

			"modified": "",

			"allow_root": "",

			"timeout": "",

			"version": "",

			"tries": "",

			"authenticate_user": "",

			"shared": "",

			"comment": "",

			"created": "",

			"class": "",

			"session_owner": "",
		}}, nil
	})
}

func table_authorized_keys() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("algorithm"),

		table.TextColumn("key"),

		table.TextColumn("key_file"),
	}
	return table.NewPlugin("authorized_keys", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"algorithm": "",

			"key": "",

			"key_file": "",
		}}, nil
	})
}

func table_autoexec() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("name"),

		table.TextColumn("source"),
	}
	return table.NewPlugin("autoexec", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"name": "",

			"source": "",
		}}, nil
	})
}

func table_block_devices() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("parent"),

		table.TextColumn("vendor"),

		table.TextColumn("model"),

		table.TextColumn("size"),

		table.TextColumn("block_size"),

		table.TextColumn("uuid"),

		table.TextColumn("type"),

		table.TextColumn("label"),
	}
	return table.NewPlugin("block_devices", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"parent": "",

			"vendor": "",

			"model": "",

			"size": "",

			"block_size": "",

			"uuid": "",

			"type": "",

			"label": "",
		}}, nil
	})
}

func table_browser_plugins() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("name"),

		table.TextColumn("identifier"),

		table.TextColumn("version"),

		table.TextColumn("sdk"),

		table.TextColumn("description"),

		table.TextColumn("development_region"),

		table.TextColumn("native"),

		table.TextColumn("path"),

		table.TextColumn("disabled"),
	}
	return table.NewPlugin("browser_plugins", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"name": "",

			"identifier": "",

			"version": "",

			"sdk": "",

			"description": "",

			"development_region": "",

			"native": "",

			"path": "",

			"disabled": "",
		}}, nil
	})
}

func table_carbon_black_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("sensor_id"),

		table.TextColumn("config_name"),

		table.TextColumn("collect_store_files"),

		table.TextColumn("collect_module_loads"),

		table.TextColumn("collect_module_info"),

		table.TextColumn("collect_file_mods"),

		table.TextColumn("collect_reg_mods"),

		table.TextColumn("collect_net_conns"),

		table.TextColumn("collect_processes"),

		table.TextColumn("collect_cross_processes"),

		table.TextColumn("collect_emet_events"),

		table.TextColumn("collect_data_file_writes"),

		table.TextColumn("collect_process_user_context"),

		table.TextColumn("collect_sensor_operations"),

		table.TextColumn("log_file_disk_quota_mb"),

		table.TextColumn("log_file_disk_quota_percentage"),

		table.TextColumn("protection_disabled"),

		table.TextColumn("sensor_ip_addr"),

		table.TextColumn("sensor_backend_server"),

		table.TextColumn("event_queue"),

		table.TextColumn("binary_queue"),
	}
	return table.NewPlugin("carbon_black_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"sensor_id": "",

			"config_name": "",

			"collect_store_files": "",

			"collect_module_loads": "",

			"collect_module_info": "",

			"collect_file_mods": "",

			"collect_reg_mods": "",

			"collect_net_conns": "",

			"collect_processes": "",

			"collect_cross_processes": "",

			"collect_emet_events": "",

			"collect_data_file_writes": "",

			"collect_process_user_context": "",

			"collect_sensor_operations": "",

			"log_file_disk_quota_mb": "",

			"log_file_disk_quota_percentage": "",

			"protection_disabled": "",

			"sensor_ip_addr": "",

			"sensor_backend_server": "",

			"event_queue": "",

			"binary_queue": "",
		}}, nil
	})
}

func table_carves() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("time"),

		table.TextColumn("sha256"),

		table.TextColumn("size"),

		table.TextColumn("path"),

		table.TextColumn("status"),

		table.TextColumn("carve_guid"),

		table.TextColumn("carve"),
	}
	return table.NewPlugin("carves", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"time": "",

			"sha256": "",

			"size": "",

			"path": "",

			"status": "",

			"carve_guid": "",

			"carve": "",
		}}, nil
	})
}

func table_certificates() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("common_name"),

		table.TextColumn("subject"),

		table.TextColumn("issuer"),

		table.TextColumn("ca"),

		table.TextColumn("self_signed"),

		table.TextColumn("not_valid_before"),

		table.TextColumn("not_valid_after"),

		table.TextColumn("signing_algorithm"),

		table.TextColumn("key_algorithm"),

		table.TextColumn("key_strength"),

		table.TextColumn("key_usage"),

		table.TextColumn("subject_key_id"),

		table.TextColumn("authority_key_id"),

		table.TextColumn("sha1"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("certificates", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"common_name": "",

			"subject": "",

			"issuer": "",

			"ca": "",

			"self_signed": "",

			"not_valid_before": "",

			"not_valid_after": "",

			"signing_algorithm": "",

			"key_algorithm": "",

			"key_strength": "",

			"key_usage": "",

			"subject_key_id": "",

			"authority_key_id": "",

			"sha1": "",

			"path": "",
		}}, nil
	})
}

func table_chocolatey_packages() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("summary"),

		table.TextColumn("author"),

		table.TextColumn("license"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("chocolatey_packages", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"version": "",

			"summary": "",

			"author": "",

			"license": "",

			"path": "",
		}}, nil
	})
}

func table_chrome_extensions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("name"),

		table.TextColumn("identifier"),

		table.TextColumn("version"),

		table.TextColumn("description"),

		table.TextColumn("locale"),

		table.TextColumn("update_url"),

		table.TextColumn("author"),

		table.TextColumn("persistent"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("chrome_extensions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"name": "",

			"identifier": "",

			"version": "",

			"description": "",

			"locale": "",

			"update_url": "",

			"author": "",

			"persistent": "",

			"path": "",
		}}, nil
	})
}

func table_cpu_time() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("core"),

		table.TextColumn("user"),

		table.TextColumn("nice"),

		table.TextColumn("system"),

		table.TextColumn("idle"),

		table.TextColumn("iowait"),

		table.TextColumn("irq"),

		table.TextColumn("softirq"),

		table.TextColumn("steal"),

		table.TextColumn("guest"),

		table.TextColumn("guest_nice"),
	}
	return table.NewPlugin("cpu_time", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"core": "",

			"user": "",

			"nice": "",

			"system": "",

			"idle": "",

			"iowait": "",

			"irq": "",

			"softirq": "",

			"steal": "",

			"guest": "",

			"guest_nice": "",
		}}, nil
	})
}

func table_cpuid() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("feature"),

		table.TextColumn("value"),

		table.TextColumn("output_register"),

		table.TextColumn("output_bit"),

		table.TextColumn("input_eax"),
	}
	return table.NewPlugin("cpuid", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"feature": "",

			"value": "",

			"output_register": "",

			"output_bit": "",

			"input_eax": "",
		}}, nil
	})
}

func table_crashes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("type"),

		table.TextColumn("pid"),

		table.TextColumn("path"),

		table.TextColumn("crash_path"),

		table.TextColumn("identifier"),

		table.TextColumn("version"),

		table.TextColumn("parent"),

		table.TextColumn("responsible"),

		table.TextColumn("uid"),

		table.TextColumn("datetime"),

		table.TextColumn("crashed_thread"),

		table.TextColumn("stack_trace"),

		table.TextColumn("exception_type"),

		table.TextColumn("exception_codes"),

		table.TextColumn("exception_notes"),

		table.TextColumn("registers"),
	}
	return table.NewPlugin("crashes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"type": "",

			"pid": "",

			"path": "",

			"crash_path": "",

			"identifier": "",

			"version": "",

			"parent": "",

			"responsible": "",

			"uid": "",

			"datetime": "",

			"crashed_thread": "",

			"stack_trace": "",

			"exception_type": "",

			"exception_codes": "",

			"exception_notes": "",

			"registers": "",
		}}, nil
	})
}

func table_crontab() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("event"),

		table.TextColumn("minute"),

		table.TextColumn("hour"),

		table.TextColumn("day_of_month"),

		table.TextColumn("month"),

		table.TextColumn("day_of_week"),

		table.TextColumn("command"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("crontab", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"event": "",

			"minute": "",

			"hour": "",

			"day_of_month": "",

			"month": "",

			"day_of_week": "",

			"command": "",

			"path": "",
		}}, nil
	})
}

func table_curl() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("url"),

		table.TextColumn("method"),

		table.TextColumn("user_agent"),

		table.TextColumn("response_code"),

		table.TextColumn("round_trip_time"),

		table.TextColumn("bytes"),

		table.TextColumn("result"),
	}
	return table.NewPlugin("curl", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"url": "",

			"method": "",

			"user_agent": "",

			"response_code": "",

			"round_trip_time": "",

			"bytes": "",

			"result": "",
		}}, nil
	})
}

func table_curl_certificate() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("hostname"),

		table.TextColumn("common_name"),

		table.TextColumn("organization"),

		table.TextColumn("organization_unit"),

		table.TextColumn("serial_number"),

		table.TextColumn("issuer_common_name"),

		table.TextColumn("issuer_organization"),

		table.TextColumn("issuer_organization_unit"),

		table.TextColumn("valid_from"),

		table.TextColumn("valid_to"),

		table.TextColumn("sha256_fingerprint"),

		table.TextColumn("sha1_fingerprint"),
	}
	return table.NewPlugin("curl_certificate", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"hostname": "",

			"common_name": "",

			"organization": "",

			"organization_unit": "",

			"serial_number": "",

			"issuer_common_name": "",

			"issuer_organization": "",

			"issuer_organization_unit": "",

			"valid_from": "",

			"valid_to": "",

			"sha256_fingerprint": "",

			"sha1_fingerprint": "",
		}}, nil
	})
}

func table_deb_packages() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("source"),

		table.TextColumn("size"),

		table.TextColumn("arch"),

		table.TextColumn("revision"),
	}
	return table.NewPlugin("deb_packages", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"version": "",

			"source": "",

			"size": "",

			"arch": "",

			"revision": "",
		}}, nil
	})
}

func table_device_file() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("device"),

		table.TextColumn("partition"),

		table.TextColumn("path"),

		table.TextColumn("filename"),

		table.TextColumn("inode"),

		table.TextColumn("uid"),

		table.TextColumn("gid"),

		table.TextColumn("mode"),

		table.TextColumn("size"),

		table.TextColumn("block_size"),

		table.TextColumn("atime"),

		table.TextColumn("mtime"),

		table.TextColumn("ctime"),

		table.TextColumn("hard_links"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("device_file", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"device": "",

			"partition": "",

			"path": "",

			"filename": "",

			"inode": "",

			"uid": "",

			"gid": "",

			"mode": "",

			"size": "",

			"block_size": "",

			"atime": "",

			"mtime": "",

			"ctime": "",

			"hard_links": "",

			"type": "",
		}}, nil
	})
}

func table_device_firmware() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("type"),

		table.TextColumn("device"),

		table.TextColumn("version"),
	}
	return table.NewPlugin("device_firmware", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"type": "",

			"device": "",

			"version": "",
		}}, nil
	})
}

func table_device_hash() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("device"),

		table.TextColumn("partition"),

		table.TextColumn("inode"),

		table.TextColumn("md5"),

		table.TextColumn("sha1"),

		table.TextColumn("sha256"),
	}
	return table.NewPlugin("device_hash", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"device": "",

			"partition": "",

			"inode": "",

			"md5": "",

			"sha1": "",

			"sha256": "",
		}}, nil
	})
}

func table_device_partitions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("device"),

		table.TextColumn("partition"),

		table.TextColumn("label"),

		table.TextColumn("type"),

		table.TextColumn("offset"),

		table.TextColumn("blocks_size"),

		table.TextColumn("blocks"),

		table.TextColumn("inodes"),

		table.TextColumn("flags"),
	}
	return table.NewPlugin("device_partitions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"device": "",

			"partition": "",

			"label": "",

			"type": "",

			"offset": "",

			"blocks_size": "",

			"blocks": "",

			"inodes": "",

			"flags": "",
		}}, nil
	})
}

func table_disk_encryption() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("uuid"),

		table.TextColumn("encrypted"),

		table.TextColumn("type"),

		table.TextColumn("uid"),

		table.TextColumn("user_uuid"),
	}
	return table.NewPlugin("disk_encryption", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"uuid": "",

			"encrypted": "",

			"type": "",

			"uid": "",

			"user_uuid": "",
		}}, nil
	})
}

func table_disk_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("action"),

		table.TextColumn("path"),

		table.TextColumn("name"),

		table.TextColumn("device"),

		table.TextColumn("uuid"),

		table.TextColumn("size"),

		table.TextColumn("ejectable"),

		table.TextColumn("mountable"),

		table.TextColumn("writable"),

		table.TextColumn("content"),

		table.TextColumn("media_name"),

		table.TextColumn("vendor"),

		table.TextColumn("filesystem"),

		table.TextColumn("checksum"),

		table.TextColumn("time"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("disk_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"action": "",

			"path": "",

			"name": "",

			"device": "",

			"uuid": "",

			"size": "",

			"ejectable": "",

			"mountable": "",

			"writable": "",

			"content": "",

			"media_name": "",

			"vendor": "",

			"filesystem": "",

			"checksum": "",

			"time": "",

			"eid": "",
		}}, nil
	})
}

func table_dns_resolvers() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("type"),

		table.TextColumn("address"),

		table.TextColumn("netmask"),

		table.TextColumn("options"),
	}
	return table.NewPlugin("dns_resolvers", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"type": "",

			"address": "",

			"netmask": "",

			"options": "",
		}}, nil
	})
}

func table_docker_container_labels() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("key"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("docker_container_labels", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"key": "",

			"value": "",
		}}, nil
	})
}

func table_docker_container_mounts() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("type"),

		table.TextColumn("name"),

		table.TextColumn("source"),

		table.TextColumn("destination"),

		table.TextColumn("driver"),

		table.TextColumn("mode"),

		table.TextColumn("rw"),

		table.TextColumn("propagation"),
	}
	return table.NewPlugin("docker_container_mounts", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"type": "",

			"name": "",

			"source": "",

			"destination": "",

			"driver": "",

			"mode": "",

			"rw": "",

			"propagation": "",
		}}, nil
	})
}

func table_docker_container_networks() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("name"),

		table.TextColumn("network_id"),

		table.TextColumn("endpoint_id"),

		table.TextColumn("gateway"),

		table.TextColumn("ip_address"),

		table.TextColumn("ip_prefix_len"),

		table.TextColumn("ipv6_gateway"),

		table.TextColumn("ipv6_address"),

		table.TextColumn("ipv6_prefix_len"),

		table.TextColumn("mac_address"),
	}
	return table.NewPlugin("docker_container_networks", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"name": "",

			"network_id": "",

			"endpoint_id": "",

			"gateway": "",

			"ip_address": "",

			"ip_prefix_len": "",

			"ipv6_gateway": "",

			"ipv6_address": "",

			"ipv6_prefix_len": "",

			"mac_address": "",
		}}, nil
	})
}

func table_docker_container_ports() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("type"),

		table.TextColumn("port"),

		table.TextColumn("host_ip"),

		table.TextColumn("host_port"),
	}
	return table.NewPlugin("docker_container_ports", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"type": "",

			"port": "",

			"host_ip": "",

			"host_port": "",
		}}, nil
	})
}

func table_docker_container_processes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("pid"),

		table.TextColumn("name"),

		table.TextColumn("cmdline"),

		table.TextColumn("state"),

		table.TextColumn("uid"),

		table.TextColumn("gid"),

		table.TextColumn("euid"),

		table.TextColumn("egid"),

		table.TextColumn("suid"),

		table.TextColumn("sgid"),

		table.TextColumn("wired_size"),

		table.TextColumn("resident_size"),

		table.TextColumn("total_size"),

		table.TextColumn("start_time"),

		table.TextColumn("parent"),

		table.TextColumn("pgroup"),

		table.TextColumn("threads"),

		table.TextColumn("nice"),

		table.TextColumn("user"),

		table.TextColumn("time"),

		table.TextColumn("cpu"),

		table.TextColumn("mem"),
	}
	return table.NewPlugin("docker_container_processes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"pid": "",

			"name": "",

			"cmdline": "",

			"state": "",

			"uid": "",

			"gid": "",

			"euid": "",

			"egid": "",

			"suid": "",

			"sgid": "",

			"wired_size": "",

			"resident_size": "",

			"total_size": "",

			"start_time": "",

			"parent": "",

			"pgroup": "",

			"threads": "",

			"nice": "",

			"user": "",

			"time": "",

			"cpu": "",

			"mem": "",
		}}, nil
	})
}

func table_docker_container_stats() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("name"),

		table.TextColumn("pids"),

		table.TextColumn("read"),

		table.TextColumn("preread"),

		table.TextColumn("interval"),

		table.TextColumn("disk_read"),

		table.TextColumn("disk_write"),

		table.TextColumn("num_procs"),

		table.TextColumn("cpu_total_usage"),

		table.TextColumn("cpu_kernelmode_usage"),

		table.TextColumn("cpu_usermode_usage"),

		table.TextColumn("system_cpu_usage"),

		table.TextColumn("online_cpus"),

		table.TextColumn("pre_cpu_total_usage"),

		table.TextColumn("pre_cpu_kernelmode_usage"),

		table.TextColumn("pre_cpu_usermode_usage"),

		table.TextColumn("pre_system_cpu_usage"),

		table.TextColumn("pre_online_cpus"),

		table.TextColumn("memory_usage"),

		table.TextColumn("memory_max_usage"),

		table.TextColumn("memory_limit"),

		table.TextColumn("network_rx_bytes"),

		table.TextColumn("network_tx_bytes"),
	}
	return table.NewPlugin("docker_container_stats", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"name": "",

			"pids": "",

			"read": "",

			"preread": "",

			"interval": "",

			"disk_read": "",

			"disk_write": "",

			"num_procs": "",

			"cpu_total_usage": "",

			"cpu_kernelmode_usage": "",

			"cpu_usermode_usage": "",

			"system_cpu_usage": "",

			"online_cpus": "",

			"pre_cpu_total_usage": "",

			"pre_cpu_kernelmode_usage": "",

			"pre_cpu_usermode_usage": "",

			"pre_system_cpu_usage": "",

			"pre_online_cpus": "",

			"memory_usage": "",

			"memory_max_usage": "",

			"memory_limit": "",

			"network_rx_bytes": "",

			"network_tx_bytes": "",
		}}, nil
	})
}

func table_docker_containers() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("name"),

		table.TextColumn("image"),

		table.TextColumn("image_id"),

		table.TextColumn("command"),

		table.TextColumn("created"),

		table.TextColumn("state"),

		table.TextColumn("status"),
	}
	return table.NewPlugin("docker_containers", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"name": "",

			"image": "",

			"image_id": "",

			"command": "",

			"created": "",

			"state": "",

			"status": "",
		}}, nil
	})
}

func table_docker_image_labels() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("key"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("docker_image_labels", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"key": "",

			"value": "",
		}}, nil
	})
}

func table_docker_images() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("created"),

		table.TextColumn("size_bytes"),

		table.TextColumn("tags"),
	}
	return table.NewPlugin("docker_images", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"created": "",

			"size_bytes": "",

			"tags": "",
		}}, nil
	})
}

func table_docker_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("containers"),

		table.TextColumn("containers_running"),

		table.TextColumn("containers_paused"),

		table.TextColumn("containers_stopped"),

		table.TextColumn("images"),

		table.TextColumn("storage_driver"),

		table.TextColumn("memory_limit"),

		table.TextColumn("swap_limit"),

		table.TextColumn("kernel_memory"),

		table.TextColumn("cpu_cfs_period"),

		table.TextColumn("cpu_cfs_quota"),

		table.TextColumn("cpu_shares"),

		table.TextColumn("cpu_set"),

		table.TextColumn("ipv4_forwarding"),

		table.TextColumn("bridge_nf_iptables"),

		table.TextColumn("bridge_nf_ip6tables"),

		table.TextColumn("oom_kill_disable"),

		table.TextColumn("logging_driver"),

		table.TextColumn("cgroup_driver"),

		table.TextColumn("kernel_version"),

		table.TextColumn("os"),

		table.TextColumn("os_type"),

		table.TextColumn("architecture"),

		table.TextColumn("cpus"),

		table.TextColumn("memory"),

		table.TextColumn("http_proxy"),

		table.TextColumn("https_proxy"),

		table.TextColumn("no_proxy"),

		table.TextColumn("name"),

		table.TextColumn("server_version"),

		table.TextColumn("root_dir"),
	}
	return table.NewPlugin("docker_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"containers": "",

			"containers_running": "",

			"containers_paused": "",

			"containers_stopped": "",

			"images": "",

			"storage_driver": "",

			"memory_limit": "",

			"swap_limit": "",

			"kernel_memory": "",

			"cpu_cfs_period": "",

			"cpu_cfs_quota": "",

			"cpu_shares": "",

			"cpu_set": "",

			"ipv4_forwarding": "",

			"bridge_nf_iptables": "",

			"bridge_nf_ip6tables": "",

			"oom_kill_disable": "",

			"logging_driver": "",

			"cgroup_driver": "",

			"kernel_version": "",

			"os": "",

			"os_type": "",

			"architecture": "",

			"cpus": "",

			"memory": "",

			"http_proxy": "",

			"https_proxy": "",

			"no_proxy": "",

			"name": "",

			"server_version": "",

			"root_dir": "",
		}}, nil
	})
}

func table_docker_network_labels() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("key"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("docker_network_labels", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"key": "",

			"value": "",
		}}, nil
	})
}

func table_docker_networks() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("id"),

		table.TextColumn("name"),

		table.TextColumn("driver"),

		table.TextColumn("created"),

		table.TextColumn("enable_ipv6"),

		table.TextColumn("subnet"),

		table.TextColumn("gateway"),
	}
	return table.NewPlugin("docker_networks", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"id": "",

			"name": "",

			"driver": "",

			"created": "",

			"enable_ipv6": "",

			"subnet": "",

			"gateway": "",
		}}, nil
	})
}

func table_docker_version() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("version"),

		table.TextColumn("api_version"),

		table.TextColumn("min_api_version"),

		table.TextColumn("git_commit"),

		table.TextColumn("go_version"),

		table.TextColumn("os"),

		table.TextColumn("arch"),

		table.TextColumn("kernel_version"),

		table.TextColumn("build_time"),
	}
	return table.NewPlugin("docker_version", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"version": "",

			"api_version": "",

			"min_api_version": "",

			"git_commit": "",

			"go_version": "",

			"os": "",

			"arch": "",

			"kernel_version": "",

			"build_time": "",
		}}, nil
	})
}

func table_docker_volume_labels() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("key"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("docker_volume_labels", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"key": "",

			"value": "",
		}}, nil
	})
}

func table_docker_volumes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("driver"),

		table.TextColumn("mount_point"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("docker_volumes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"driver": "",

			"mount_point": "",

			"type": "",
		}}, nil
	})
}

func table_drivers() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("device_id"),

		table.TextColumn("device_name"),

		table.TextColumn("image"),

		table.TextColumn("description"),

		table.TextColumn("service"),

		table.TextColumn("service_key"),

		table.TextColumn("version"),

		table.TextColumn("inf"),

		table.TextColumn("class"),

		table.TextColumn("provider"),

		table.TextColumn("manufacturer"),

		table.TextColumn("driver_key"),

		table.TextColumn("date"),
	}
	return table.NewPlugin("drivers", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"device_id": "",

			"device_name": "",

			"image": "",

			"description": "",

			"service": "",

			"service_key": "",

			"version": "",

			"inf": "",

			"class": "",

			"provider": "",

			"manufacturer": "",

			"driver_key": "",

			"date": "",
		}}, nil
	})
}

func table_ec2_instance_metadata() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("instance_id"),

		table.TextColumn("instance_type"),

		table.TextColumn("architecture"),

		table.TextColumn("region"),

		table.TextColumn("availability_zone"),

		table.TextColumn("local_hostname"),

		table.TextColumn("local_ipv4"),

		table.TextColumn("mac"),

		table.TextColumn("security_groups"),

		table.TextColumn("iam_arn"),

		table.TextColumn("ami_id"),

		table.TextColumn("reservation_id"),

		table.TextColumn("account_id"),

		table.TextColumn("ssh_public_key"),
	}
	return table.NewPlugin("ec2_instance_metadata", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"instance_id": "",

			"instance_type": "",

			"architecture": "",

			"region": "",

			"availability_zone": "",

			"local_hostname": "",

			"local_ipv4": "",

			"mac": "",

			"security_groups": "",

			"iam_arn": "",

			"ami_id": "",

			"reservation_id": "",

			"account_id": "",

			"ssh_public_key": "",
		}}, nil
	})
}

func table_ec2_instance_tags() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("instance_id"),

		table.TextColumn("key"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("ec2_instance_tags", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"instance_id": "",

			"key": "",

			"value": "",
		}}, nil
	})
}

func table_etc_hosts() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("address"),

		table.TextColumn("hostnames"),
	}
	return table.NewPlugin("etc_hosts", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"address": "",

			"hostnames": "",
		}}, nil
	})
}

func table_etc_protocols() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("number"),

		table.TextColumn("alias"),

		table.TextColumn("comment"),
	}
	return table.NewPlugin("etc_protocols", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"number": "",

			"alias": "",

			"comment": "",
		}}, nil
	})
}

func table_etc_services() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("port"),

		table.TextColumn("protocol"),

		table.TextColumn("aliases"),

		table.TextColumn("comment"),
	}
	return table.NewPlugin("etc_services", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"port": "",

			"protocol": "",

			"aliases": "",

			"comment": "",
		}}, nil
	})
}

func table_event_taps() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("enabled"),

		table.TextColumn("event_tap_id"),

		table.TextColumn("event_tapped"),

		table.TextColumn("process_being_tapped"),

		table.TextColumn("tapping_process"),
	}
	return table.NewPlugin("event_taps", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"enabled": "",

			"event_tap_id": "",

			"event_tapped": "",

			"process_being_tapped": "",

			"tapping_process": "",
		}}, nil
	})
}

func table_example() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("points"),

		table.TextColumn("size"),

		table.TextColumn("action"),

		table.TextColumn("id"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("example", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"points": "",

			"size": "",

			"action": "",

			"id": "",

			"path": "",
		}}, nil
	})
}

func table_extended_attributes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("directory"),

		table.TextColumn("key"),

		table.TextColumn("value"),

		table.TextColumn("base64"),
	}
	return table.NewPlugin("extended_attributes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"directory": "",

			"key": "",

			"value": "",

			"base64": "",
		}}, nil
	})
}

func table_fan_speed_sensors() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("fan"),

		table.TextColumn("name"),

		table.TextColumn("actual"),

		table.TextColumn("min"),

		table.TextColumn("max"),

		table.TextColumn("target"),
	}
	return table.NewPlugin("fan_speed_sensors", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"fan": "",

			"name": "",

			"actual": "",

			"min": "",

			"max": "",

			"target": "",
		}}, nil
	})
}

func table_fbsd_kmods() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("size"),

		table.TextColumn("refs"),

		table.TextColumn("address"),
	}
	return table.NewPlugin("fbsd_kmods", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"size": "",

			"refs": "",

			"address": "",
		}}, nil
	})
}

func table_file() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("directory"),

		table.TextColumn("filename"),

		table.TextColumn("inode"),

		table.TextColumn("uid"),

		table.TextColumn("gid"),

		table.TextColumn("mode"),

		table.TextColumn("device"),

		table.TextColumn("size"),

		table.TextColumn("block_size"),

		table.TextColumn("atime"),

		table.TextColumn("mtime"),

		table.TextColumn("ctime"),

		table.TextColumn("btime"),

		table.TextColumn("hard_links"),

		table.TextColumn("symlink"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("file", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"directory": "",

			"filename": "",

			"inode": "",

			"uid": "",

			"gid": "",

			"mode": "",

			"device": "",

			"size": "",

			"block_size": "",

			"atime": "",

			"mtime": "",

			"ctime": "",

			"btime": "",

			"hard_links": "",

			"symlink": "",

			"type": "",
		}}, nil
	})
}

func table_file_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("target_path"),

		table.TextColumn("category"),

		table.TextColumn("action"),

		table.TextColumn("transaction_id"),

		table.TextColumn("inode"),

		table.TextColumn("uid"),

		table.TextColumn("gid"),

		table.TextColumn("mode"),

		table.TextColumn("size"),

		table.TextColumn("atime"),

		table.TextColumn("mtime"),

		table.TextColumn("ctime"),

		table.TextColumn("md5"),

		table.TextColumn("sha1"),

		table.TextColumn("sha256"),

		table.TextColumn("hashed"),

		table.TextColumn("time"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("file_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"target_path": "",

			"category": "",

			"action": "",

			"transaction_id": "",

			"inode": "",

			"uid": "",

			"gid": "",

			"mode": "",

			"size": "",

			"atime": "",

			"mtime": "",

			"ctime": "",

			"md5": "",

			"sha1": "",

			"sha256": "",

			"hashed": "",

			"time": "",

			"eid": "",
		}}, nil
	})
}

func table_firefox_addons() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("name"),

		table.TextColumn("identifier"),

		table.TextColumn("creator"),

		table.TextColumn("type"),

		table.TextColumn("version"),

		table.TextColumn("description"),

		table.TextColumn("source_url"),

		table.TextColumn("visible"),

		table.TextColumn("active"),

		table.TextColumn("disabled"),

		table.TextColumn("autoupdate"),

		table.TextColumn("native"),

		table.TextColumn("location"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("firefox_addons", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"name": "",

			"identifier": "",

			"creator": "",

			"type": "",

			"version": "",

			"description": "",

			"source_url": "",

			"visible": "",

			"active": "",

			"disabled": "",

			"autoupdate": "",

			"native": "",

			"location": "",

			"path": "",
		}}, nil
	})
}

func table_gatekeeper() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("assessments_enabled"),

		table.TextColumn("dev_id_enabled"),

		table.TextColumn("version"),

		table.TextColumn("opaque_version"),
	}
	return table.NewPlugin("gatekeeper", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"assessments_enabled": "",

			"dev_id_enabled": "",

			"version": "",

			"opaque_version": "",
		}}, nil
	})
}

func table_gatekeeper_approved_apps() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("requirement"),

		table.TextColumn("ctime"),

		table.TextColumn("mtime"),
	}
	return table.NewPlugin("gatekeeper_approved_apps", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"requirement": "",

			"ctime": "",

			"mtime": "",
		}}, nil
	})
}

func table_groups() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("gid"),

		table.TextColumn("gid_signed"),

		table.TextColumn("groupname"),

		table.TextColumn("group_sid"),

		table.TextColumn("comment"),
	}
	return table.NewPlugin("groups", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"gid": "",

			"gid_signed": "",

			"groupname": "",

			"group_sid": "",

			"comment": "",
		}}, nil
	})
}

func table_hardware_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("action"),

		table.TextColumn("path"),

		table.TextColumn("type"),

		table.TextColumn("driver"),

		table.TextColumn("vendor"),

		table.TextColumn("vendor_id"),

		table.TextColumn("model"),

		table.TextColumn("model_id"),

		table.TextColumn("serial"),

		table.TextColumn("revision"),

		table.TextColumn("time"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("hardware_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"action": "",

			"path": "",

			"type": "",

			"driver": "",

			"vendor": "",

			"vendor_id": "",

			"model": "",

			"model_id": "",

			"serial": "",

			"revision": "",

			"time": "",

			"eid": "",
		}}, nil
	})
}

func table_hash() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("directory"),

		table.TextColumn("md5"),

		table.TextColumn("sha1"),

		table.TextColumn("sha256"),
	}
	return table.NewPlugin("hash", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"directory": "",

			"md5": "",

			"sha1": "",

			"sha256": "",
		}}, nil
	})
}

func table_homebrew_packages() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("path"),

		table.TextColumn("version"),
	}
	return table.NewPlugin("homebrew_packages", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"path": "",

			"version": "",
		}}, nil
	})
}

func table_ie_extensions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("registry_path"),

		table.TextColumn("version"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("ie_extensions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"registry_path": "",

			"version": "",

			"path": "",
		}}, nil
	})
}

func table_intel_me_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("version"),
	}
	return table.NewPlugin("intel_me_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"version": "",
		}}, nil
	})
}

func table_interface_addresses() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("interface"),

		table.TextColumn("address"),

		table.TextColumn("mask"),

		table.TextColumn("broadcast"),

		table.TextColumn("point_to_point"),

		table.TextColumn("type"),

		table.TextColumn("friendly_name"),
	}
	return table.NewPlugin("interface_addresses", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"interface": "",

			"address": "",

			"mask": "",

			"broadcast": "",

			"point_to_point": "",

			"type": "",

			"friendly_name": "",
		}}, nil
	})
}

func table_interface_details() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("interface"),

		table.TextColumn("mac"),

		table.TextColumn("type"),

		table.TextColumn("mtu"),

		table.TextColumn("metric"),

		table.TextColumn("flags"),

		table.TextColumn("ipackets"),

		table.TextColumn("opackets"),

		table.TextColumn("ibytes"),

		table.TextColumn("obytes"),

		table.TextColumn("ierrors"),

		table.TextColumn("oerrors"),

		table.TextColumn("idrops"),

		table.TextColumn("odrops"),

		table.TextColumn("collisions"),

		table.TextColumn("last_change"),

		table.TextColumn("friendly_name"),

		table.TextColumn("description"),

		table.TextColumn("manufacturer"),

		table.TextColumn("connection_id"),

		table.TextColumn("connection_status"),

		table.TextColumn("enabled"),

		table.TextColumn("physical_adapter"),

		table.TextColumn("speed"),

		table.TextColumn("dhcp_enabled"),

		table.TextColumn("dhcp_lease_expires"),

		table.TextColumn("dhcp_lease_obtained"),

		table.TextColumn("dhcp_server"),

		table.TextColumn("dns_domain"),

		table.TextColumn("dns_domain_suffix_search_order"),

		table.TextColumn("dns_host_name"),

		table.TextColumn("dns_server_search_order"),
	}
	return table.NewPlugin("interface_details", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"interface": "",

			"mac": "",

			"type": "",

			"mtu": "",

			"metric": "",

			"flags": "",

			"ipackets": "",

			"opackets": "",

			"ibytes": "",

			"obytes": "",

			"ierrors": "",

			"oerrors": "",

			"idrops": "",

			"odrops": "",

			"collisions": "",

			"last_change": "",

			"friendly_name": "",

			"description": "",

			"manufacturer": "",

			"connection_id": "",

			"connection_status": "",

			"enabled": "",

			"physical_adapter": "",

			"speed": "",

			"dhcp_enabled": "",

			"dhcp_lease_expires": "",

			"dhcp_lease_obtained": "",

			"dhcp_server": "",

			"dns_domain": "",

			"dns_domain_suffix_search_order": "",

			"dns_host_name": "",

			"dns_server_search_order": "",
		}}, nil
	})
}

func table_iokit_devicetree() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("class"),

		table.TextColumn("id"),

		table.TextColumn("parent"),

		table.TextColumn("device_path"),

		table.TextColumn("service"),

		table.TextColumn("busy_state"),

		table.TextColumn("retain_count"),

		table.TextColumn("depth"),
	}
	return table.NewPlugin("iokit_devicetree", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"class": "",

			"id": "",

			"parent": "",

			"device_path": "",

			"service": "",

			"busy_state": "",

			"retain_count": "",

			"depth": "",
		}}, nil
	})
}

func table_iokit_registry() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("class"),

		table.TextColumn("id"),

		table.TextColumn("parent"),

		table.TextColumn("busy_state"),

		table.TextColumn("retain_count"),

		table.TextColumn("depth"),
	}
	return table.NewPlugin("iokit_registry", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"class": "",

			"id": "",

			"parent": "",

			"busy_state": "",

			"retain_count": "",

			"depth": "",
		}}, nil
	})
}

func table_iptables() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("filter_name"),

		table.TextColumn("chain"),

		table.TextColumn("policy"),

		table.TextColumn("target"),

		table.TextColumn("protocol"),

		table.TextColumn("src_port"),

		table.TextColumn("dst_port"),

		table.TextColumn("src_ip"),

		table.TextColumn("src_mask"),

		table.TextColumn("iniface"),

		table.TextColumn("iniface_mask"),

		table.TextColumn("dst_ip"),

		table.TextColumn("dst_mask"),

		table.TextColumn("outiface"),

		table.TextColumn("outiface_mask"),

		table.TextColumn("match"),

		table.TextColumn("packets"),

		table.TextColumn("bytes"),
	}
	return table.NewPlugin("iptables", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"filter_name": "",

			"chain": "",

			"policy": "",

			"target": "",

			"protocol": "",

			"src_port": "",

			"dst_port": "",

			"src_ip": "",

			"src_mask": "",

			"iniface": "",

			"iniface_mask": "",

			"dst_ip": "",

			"dst_mask": "",

			"outiface": "",

			"outiface_mask": "",

			"match": "",

			"packets": "",

			"bytes": "",
		}}, nil
	})
}

func table_kernel_extensions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("idx"),

		table.TextColumn("refs"),

		table.TextColumn("size"),

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("linked_against"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("kernel_extensions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"idx": "",

			"refs": "",

			"size": "",

			"name": "",

			"version": "",

			"linked_against": "",

			"path": "",
		}}, nil
	})
}

func table_kernel_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("version"),

		table.TextColumn("arguments"),

		table.TextColumn("path"),

		table.TextColumn("device"),
	}
	return table.NewPlugin("kernel_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"version": "",

			"arguments": "",

			"path": "",

			"device": "",
		}}, nil
	})
}

func table_kernel_integrity() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("sycall_addr_modified"),

		table.TextColumn("text_segment_hash"),
	}
	return table.NewPlugin("kernel_integrity", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"sycall_addr_modified": "",

			"text_segment_hash": "",
		}}, nil
	})
}

func table_kernel_modules() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("size"),

		table.TextColumn("used_by"),

		table.TextColumn("status"),

		table.TextColumn("address"),
	}
	return table.NewPlugin("kernel_modules", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"size": "",

			"used_by": "",

			"status": "",

			"address": "",
		}}, nil
	})
}

func table_kernel_panics() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("time"),

		table.TextColumn("registers"),

		table.TextColumn("frame_backtrace"),

		table.TextColumn("module_backtrace"),

		table.TextColumn("dependencies"),

		table.TextColumn("name"),

		table.TextColumn("os_version"),

		table.TextColumn("kernel_version"),

		table.TextColumn("system_model"),

		table.TextColumn("uptime"),

		table.TextColumn("last_loaded"),

		table.TextColumn("last_unloaded"),
	}
	return table.NewPlugin("kernel_panics", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"time": "",

			"registers": "",

			"frame_backtrace": "",

			"module_backtrace": "",

			"dependencies": "",

			"name": "",

			"os_version": "",

			"kernel_version": "",

			"system_model": "",

			"uptime": "",

			"last_loaded": "",

			"last_unloaded": "",
		}}, nil
	})
}

func table_keychain_acls() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("keychain_path"),

		table.TextColumn("authorizations"),

		table.TextColumn("path"),

		table.TextColumn("description"),

		table.TextColumn("label"),
	}
	return table.NewPlugin("keychain_acls", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"keychain_path": "",

			"authorizations": "",

			"path": "",

			"description": "",

			"label": "",
		}}, nil
	})
}

func table_keychain_items() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("label"),

		table.TextColumn("description"),

		table.TextColumn("comment"),

		table.TextColumn("created"),

		table.TextColumn("modified"),

		table.TextColumn("type"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("keychain_items", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"label": "",

			"description": "",

			"comment": "",

			"created": "",

			"modified": "",

			"type": "",

			"path": "",
		}}, nil
	})
}

func table_known_hosts() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("key"),

		table.TextColumn("key_file"),
	}
	return table.NewPlugin("known_hosts", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"key": "",

			"key_file": "",
		}}, nil
	})
}

func table_last() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("username"),

		table.TextColumn("tty"),

		table.TextColumn("pid"),

		table.TextColumn("type"),

		table.TextColumn("time"),

		table.TextColumn("host"),
	}
	return table.NewPlugin("last", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"username": "",

			"tty": "",

			"pid": "",

			"type": "",

			"time": "",

			"host": "",
		}}, nil
	})
}

func table_launchd() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("name"),

		table.TextColumn("label"),

		table.TextColumn("program"),

		table.TextColumn("run_at_load"),

		table.TextColumn("keep_alive"),

		table.TextColumn("on_demand"),

		table.TextColumn("disabled"),

		table.TextColumn("username"),

		table.TextColumn("groupname"),

		table.TextColumn("stdout_path"),

		table.TextColumn("stderr_path"),

		table.TextColumn("start_interval"),

		table.TextColumn("program_arguments"),

		table.TextColumn("watch_paths"),

		table.TextColumn("queue_directories"),

		table.TextColumn("inetd_compatibility"),

		table.TextColumn("start_on_mount"),

		table.TextColumn("root_directory"),

		table.TextColumn("working_directory"),

		table.TextColumn("process_type"),
	}
	return table.NewPlugin("launchd", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"name": "",

			"label": "",

			"program": "",

			"run_at_load": "",

			"keep_alive": "",

			"on_demand": "",

			"disabled": "",

			"username": "",

			"groupname": "",

			"stdout_path": "",

			"stderr_path": "",

			"start_interval": "",

			"program_arguments": "",

			"watch_paths": "",

			"queue_directories": "",

			"inetd_compatibility": "",

			"start_on_mount": "",

			"root_directory": "",

			"working_directory": "",

			"process_type": "",
		}}, nil
	})
}

func table_launchd_overrides() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("label"),

		table.TextColumn("key"),

		table.TextColumn("value"),

		table.TextColumn("uid"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("launchd_overrides", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"label": "",

			"key": "",

			"value": "",

			"uid": "",

			"path": "",
		}}, nil
	})
}

func table_listening_ports() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("port"),

		table.TextColumn("protocol"),

		table.TextColumn("family"),

		table.TextColumn("address"),
	}
	return table.NewPlugin("listening_ports", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"port": "",

			"protocol": "",

			"family": "",

			"address": "",
		}}, nil
	})
}

func table_lldp_neighbors() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("interface"),

		table.TextColumn("rid"),

		table.TextColumn("chassis_id_type"),

		table.TextColumn("chassis_id"),

		table.TextColumn("chassis_sysname"),

		table.TextColumn("chassis_sys_description"),

		table.TextColumn("chassis_bridge_capability_available"),

		table.TextColumn("chassis_bridge_capability_enabled"),

		table.TextColumn("chassis_router_capability_available"),

		table.TextColumn("chassis_router_capability_enabled"),

		table.TextColumn("chassis_repeater_capability_available"),

		table.TextColumn("chassis_repeater_capability_enabled"),

		table.TextColumn("chassis_wlan_capability_available"),

		table.TextColumn("chassis_wlan_capability_enabled"),

		table.TextColumn("chassis_tel_capability_available"),

		table.TextColumn("chassis_tel_capability_enabled"),

		table.TextColumn("chassis_docsis_capability_available"),

		table.TextColumn("chassis_docsis_capability_enabled"),

		table.TextColumn("chassis_station_capability_available"),

		table.TextColumn("chassis_station_capability_enabled"),

		table.TextColumn("chassis_other_capability_available"),

		table.TextColumn("chassis_other_capability_enabled"),

		table.TextColumn("chassis_mgmt_ips"),

		table.TextColumn("port_id_type"),

		table.TextColumn("port_id"),

		table.TextColumn("port_description"),

		table.TextColumn("port_ttl"),

		table.TextColumn("port_mfs"),

		table.TextColumn("port_aggregation_id"),

		table.TextColumn("port_autoneg_supported"),

		table.TextColumn("port_autoneg_enabled"),

		table.TextColumn("port_mau_type"),

		table.TextColumn("port_autoneg_10baset_hd_enabled"),

		table.TextColumn("port_autoneg_10baset_fd_enabled"),

		table.TextColumn("port_autoneg_100basetx_hd_enabled"),

		table.TextColumn("port_autoneg_100basetx_fd_enabled"),

		table.TextColumn("port_autoneg_100baset2_hd_enabled"),

		table.TextColumn("port_autoneg_100baset2_fd_enabled"),

		table.TextColumn("port_autoneg_100baset4_hd_enabled"),

		table.TextColumn("port_autoneg_100baset4_fd_enabled"),

		table.TextColumn("port_autoneg_1000basex_hd_enabled"),

		table.TextColumn("port_autoneg_1000basex_fd_enabled"),

		table.TextColumn("port_autoneg_1000baset_hd_enabled"),

		table.TextColumn("port_autoneg_1000baset_fd_enabled"),

		table.TextColumn("power_device_type"),

		table.TextColumn("power_mdi_supported"),

		table.TextColumn("power_mdi_enabled"),

		table.TextColumn("power_paircontrol_enabled"),

		table.TextColumn("power_pairs"),

		table.TextColumn("power_class"),

		table.TextColumn("power_8023at_enabled"),

		table.TextColumn("power_8023at_power_type"),

		table.TextColumn("power_8023at_power_source"),

		table.TextColumn("power_8023at_power_priority"),

		table.TextColumn("power_8023at_power_allocated"),

		table.TextColumn("power_8023at_power_requested"),

		table.TextColumn("med_device_type"),

		table.TextColumn("med_capability_capabilities"),

		table.TextColumn("med_capability_policy"),

		table.TextColumn("med_capability_location"),

		table.TextColumn("med_capability_mdi_pse"),

		table.TextColumn("med_capability_mdi_pd"),

		table.TextColumn("med_capability_inventory"),

		table.TextColumn("med_policies"),

		table.TextColumn("vlans"),

		table.TextColumn("pvid"),

		table.TextColumn("ppvids_supported"),

		table.TextColumn("ppvids_enabled"),

		table.TextColumn("pids"),
	}
	return table.NewPlugin("lldp_neighbors", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"interface": "",

			"rid": "",

			"chassis_id_type": "",

			"chassis_id": "",

			"chassis_sysname": "",

			"chassis_sys_description": "",

			"chassis_bridge_capability_available": "",

			"chassis_bridge_capability_enabled": "",

			"chassis_router_capability_available": "",

			"chassis_router_capability_enabled": "",

			"chassis_repeater_capability_available": "",

			"chassis_repeater_capability_enabled": "",

			"chassis_wlan_capability_available": "",

			"chassis_wlan_capability_enabled": "",

			"chassis_tel_capability_available": "",

			"chassis_tel_capability_enabled": "",

			"chassis_docsis_capability_available": "",

			"chassis_docsis_capability_enabled": "",

			"chassis_station_capability_available": "",

			"chassis_station_capability_enabled": "",

			"chassis_other_capability_available": "",

			"chassis_other_capability_enabled": "",

			"chassis_mgmt_ips": "",

			"port_id_type": "",

			"port_id": "",

			"port_description": "",

			"port_ttl": "",

			"port_mfs": "",

			"port_aggregation_id": "",

			"port_autoneg_supported": "",

			"port_autoneg_enabled": "",

			"port_mau_type": "",

			"port_autoneg_10baset_hd_enabled": "",

			"port_autoneg_10baset_fd_enabled": "",

			"port_autoneg_100basetx_hd_enabled": "",

			"port_autoneg_100basetx_fd_enabled": "",

			"port_autoneg_100baset2_hd_enabled": "",

			"port_autoneg_100baset2_fd_enabled": "",

			"port_autoneg_100baset4_hd_enabled": "",

			"port_autoneg_100baset4_fd_enabled": "",

			"port_autoneg_1000basex_hd_enabled": "",

			"port_autoneg_1000basex_fd_enabled": "",

			"port_autoneg_1000baset_hd_enabled": "",

			"port_autoneg_1000baset_fd_enabled": "",

			"power_device_type": "",

			"power_mdi_supported": "",

			"power_mdi_enabled": "",

			"power_paircontrol_enabled": "",

			"power_pairs": "",

			"power_class": "",

			"power_8023at_enabled": "",

			"power_8023at_power_type": "",

			"power_8023at_power_source": "",

			"power_8023at_power_priority": "",

			"power_8023at_power_allocated": "",

			"power_8023at_power_requested": "",

			"med_device_type": "",

			"med_capability_capabilities": "",

			"med_capability_policy": "",

			"med_capability_location": "",

			"med_capability_mdi_pse": "",

			"med_capability_mdi_pd": "",

			"med_capability_inventory": "",

			"med_policies": "",

			"vlans": "",

			"pvid": "",

			"ppvids_supported": "",

			"ppvids_enabled": "",

			"pids": "",
		}}, nil
	})
}

func table_load_average() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("period"),

		table.TextColumn("average"),
	}
	return table.NewPlugin("load_average", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"period": "",

			"average": "",
		}}, nil
	})
}

func table_logged_in_users() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("type"),

		table.TextColumn("user"),

		table.TextColumn("tty"),

		table.TextColumn("host"),

		table.TextColumn("time"),

		table.TextColumn("pid"),
	}
	return table.NewPlugin("logged_in_users", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"type": "",

			"user": "",

			"tty": "",

			"host": "",

			"time": "",

			"pid": "",
		}}, nil
	})
}

func table_logical_drives() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("device_id"),

		table.TextColumn("type"),

		table.TextColumn("free_space"),

		table.TextColumn("size"),

		table.TextColumn("file_system"),

		table.TextColumn("boot_partition"),
	}
	return table.NewPlugin("logical_drives", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"device_id": "",

			"type": "",

			"free_space": "",

			"size": "",

			"file_system": "",

			"boot_partition": "",
		}}, nil
	})
}

func table_magic() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("data"),

		table.TextColumn("mime_type"),

		table.TextColumn("mime_encoding"),
	}
	return table.NewPlugin("magic", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"data": "",

			"mime_type": "",

			"mime_encoding": "",
		}}, nil
	})
}

func table_managed_policies() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("domain"),

		table.TextColumn("uuid"),

		table.TextColumn("name"),

		table.TextColumn("value"),

		table.TextColumn("username"),

		table.TextColumn("manual"),
	}
	return table.NewPlugin("managed_policies", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"domain": "",

			"uuid": "",

			"name": "",

			"value": "",

			"username": "",

			"manual": "",
		}}, nil
	})
}

func table_md_devices() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("device_name"),

		table.TextColumn("status"),

		table.TextColumn("raid_level"),

		table.TextColumn("size"),

		table.TextColumn("chunk_size"),

		table.TextColumn("raid_disks"),

		table.TextColumn("nr_raid_disks"),

		table.TextColumn("working_disks"),

		table.TextColumn("active_disks"),

		table.TextColumn("failed_disks"),

		table.TextColumn("spare_disks"),

		table.TextColumn("superblock_state"),

		table.TextColumn("superblock_version"),

		table.TextColumn("superblock_update_time"),

		table.TextColumn("bitmap_on_mem"),

		table.TextColumn("bitmap_chunk_size"),

		table.TextColumn("bitmap_external_file"),

		table.TextColumn("recovery_progress"),

		table.TextColumn("recovery_finish"),

		table.TextColumn("recovery_speed"),

		table.TextColumn("resync_progress"),

		table.TextColumn("resync_finish"),

		table.TextColumn("resync_speed"),

		table.TextColumn("reshape_progress"),

		table.TextColumn("reshape_finish"),

		table.TextColumn("reshape_speed"),

		table.TextColumn("check_array_progress"),

		table.TextColumn("check_array_finish"),

		table.TextColumn("check_array_speed"),

		table.TextColumn("unused_devices"),

		table.TextColumn("other"),
	}
	return table.NewPlugin("md_devices", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"device_name": "",

			"status": "",

			"raid_level": "",

			"size": "",

			"chunk_size": "",

			"raid_disks": "",

			"nr_raid_disks": "",

			"working_disks": "",

			"active_disks": "",

			"failed_disks": "",

			"spare_disks": "",

			"superblock_state": "",

			"superblock_version": "",

			"superblock_update_time": "",

			"bitmap_on_mem": "",

			"bitmap_chunk_size": "",

			"bitmap_external_file": "",

			"recovery_progress": "",

			"recovery_finish": "",

			"recovery_speed": "",

			"resync_progress": "",

			"resync_finish": "",

			"resync_speed": "",

			"reshape_progress": "",

			"reshape_finish": "",

			"reshape_speed": "",

			"check_array_progress": "",

			"check_array_finish": "",

			"check_array_speed": "",

			"unused_devices": "",

			"other": "",
		}}, nil
	})
}

func table_md_drives() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("md_device_name"),

		table.TextColumn("drive_name"),

		table.TextColumn("slot"),

		table.TextColumn("state"),
	}
	return table.NewPlugin("md_drives", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"md_device_name": "",

			"drive_name": "",

			"slot": "",

			"state": "",
		}}, nil
	})
}

func table_md_personalities() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),
	}
	return table.NewPlugin("md_personalities", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",
		}}, nil
	})
}

func table_memory_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("memory_total"),

		table.TextColumn("memory_free"),

		table.TextColumn("buffers"),

		table.TextColumn("cached"),

		table.TextColumn("swap_cached"),

		table.TextColumn("active"),

		table.TextColumn("inactive"),

		table.TextColumn("swap_total"),

		table.TextColumn("swap_free"),
	}
	return table.NewPlugin("memory_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"memory_total": "",

			"memory_free": "",

			"buffers": "",

			"cached": "",

			"swap_cached": "",

			"active": "",

			"inactive": "",

			"swap_total": "",

			"swap_free": "",
		}}, nil
	})
}

func table_memory_map() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("start"),

		table.TextColumn("end"),
	}
	return table.NewPlugin("memory_map", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"start": "",

			"end": "",
		}}, nil
	})
}

func table_mounts() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("device"),

		table.TextColumn("device_alias"),

		table.TextColumn("path"),

		table.TextColumn("type"),

		table.TextColumn("blocks_size"),

		table.TextColumn("blocks"),

		table.TextColumn("blocks_free"),

		table.TextColumn("blocks_available"),

		table.TextColumn("inodes"),

		table.TextColumn("inodes_free"),

		table.TextColumn("flags"),
	}
	return table.NewPlugin("mounts", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"device": "",

			"device_alias": "",

			"path": "",

			"type": "",

			"blocks_size": "",

			"blocks": "",

			"blocks_free": "",

			"blocks_available": "",

			"inodes": "",

			"inodes_free": "",

			"flags": "",
		}}, nil
	})
}

func table_msr() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("processor_number"),

		table.TextColumn("turbo_disabled"),

		table.TextColumn("turbo_ratio_limit"),

		table.TextColumn("platform_info"),

		table.TextColumn("perf_ctl"),

		table.TextColumn("perf_status"),

		table.TextColumn("feature_control"),

		table.TextColumn("rapl_power_limit"),

		table.TextColumn("rapl_energy_status"),

		table.TextColumn("rapl_power_units"),
	}
	return table.NewPlugin("msr", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"processor_number": "",

			"turbo_disabled": "",

			"turbo_ratio_limit": "",

			"platform_info": "",

			"perf_ctl": "",

			"perf_status": "",

			"feature_control": "",

			"rapl_power_limit": "",

			"rapl_energy_status": "",

			"rapl_power_units": "",
		}}, nil
	})
}

func table_nfs_shares() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("share"),

		table.TextColumn("options"),

		table.TextColumn("readonly"),
	}
	return table.NewPlugin("nfs_shares", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"share": "",

			"options": "",

			"readonly": "",
		}}, nil
	})
}

func table_nvram() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("type"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("nvram", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"type": "",

			"value": "",
		}}, nil
	})
}

func table_opera_extensions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("name"),

		table.TextColumn("identifier"),

		table.TextColumn("version"),

		table.TextColumn("description"),

		table.TextColumn("locale"),

		table.TextColumn("update_url"),

		table.TextColumn("author"),

		table.TextColumn("persistent"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("opera_extensions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"name": "",

			"identifier": "",

			"version": "",

			"description": "",

			"locale": "",

			"update_url": "",

			"author": "",

			"persistent": "",

			"path": "",
		}}, nil
	})
}

func table_os_version() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("major"),

		table.TextColumn("minor"),

		table.TextColumn("patch"),

		table.TextColumn("build"),

		table.TextColumn("platform"),

		table.TextColumn("platform_like"),

		table.TextColumn("codename"),
	}
	return table.NewPlugin("os_version", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"version": "",

			"major": "",

			"minor": "",

			"patch": "",

			"build": "",

			"platform": "",

			"platform_like": "",

			"codename": "",
		}}, nil
	})
}

func table_osquery_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("publisher"),

		table.TextColumn("type"),

		table.TextColumn("subscriptions"),

		table.TextColumn("events"),

		table.TextColumn("refreshes"),

		table.TextColumn("active"),
	}
	return table.NewPlugin("osquery_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"publisher": "",

			"type": "",

			"subscriptions": "",

			"events": "",

			"refreshes": "",

			"active": "",
		}}, nil
	})
}

func table_osquery_extensions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uuid"),

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("sdk_version"),

		table.TextColumn("path"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("osquery_extensions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uuid": "",

			"name": "",

			"version": "",

			"sdk_version": "",

			"path": "",

			"type": "",
		}}, nil
	})
}

func table_osquery_flags() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("type"),

		table.TextColumn("description"),

		table.TextColumn("default_value"),

		table.TextColumn("value"),

		table.TextColumn("shell_only"),
	}
	return table.NewPlugin("osquery_flags", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"type": "",

			"description": "",

			"default_value": "",

			"value": "",

			"shell_only": "",
		}}, nil
	})
}

func table_osquery_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("uuid"),

		table.TextColumn("instance_id"),

		table.TextColumn("version"),

		table.TextColumn("config_hash"),

		table.TextColumn("config_valid"),

		table.TextColumn("extensions"),

		table.TextColumn("build_platform"),

		table.TextColumn("build_distro"),

		table.TextColumn("start_time"),

		table.TextColumn("watcher"),
	}
	return table.NewPlugin("osquery_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"uuid": "",

			"instance_id": "",

			"version": "",

			"config_hash": "",

			"config_valid": "",

			"extensions": "",

			"build_platform": "",

			"build_distro": "",

			"start_time": "",

			"watcher": "",
		}}, nil
	})
}

func table_osquery_packs() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("platform"),

		table.TextColumn("version"),

		table.TextColumn("shard"),

		table.TextColumn("discovery_cache_hits"),

		table.TextColumn("discovery_executions"),

		table.TextColumn("active"),
	}
	return table.NewPlugin("osquery_packs", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"platform": "",

			"version": "",

			"shard": "",

			"discovery_cache_hits": "",

			"discovery_executions": "",

			"active": "",
		}}, nil
	})
}

func table_osquery_registry() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("registry"),

		table.TextColumn("name"),

		table.TextColumn("owner_uuid"),

		table.TextColumn("internal"),

		table.TextColumn("active"),
	}
	return table.NewPlugin("osquery_registry", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"registry": "",

			"name": "",

			"owner_uuid": "",

			"internal": "",

			"active": "",
		}}, nil
	})
}

func table_osquery_schedule() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("query"),

		table.TextColumn("interval"),

		table.TextColumn("executions"),

		table.TextColumn("last_executed"),

		table.TextColumn("blacklisted"),

		table.TextColumn("output_size"),

		table.TextColumn("wall_time"),

		table.TextColumn("user_time"),

		table.TextColumn("system_time"),

		table.TextColumn("average_memory"),
	}
	return table.NewPlugin("osquery_schedule", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"query": "",

			"interval": "",

			"executions": "",

			"last_executed": "",

			"blacklisted": "",

			"output_size": "",

			"wall_time": "",

			"user_time": "",

			"system_time": "",

			"average_memory": "",
		}}, nil
	})
}

func table_package_bom() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("filepath"),

		table.TextColumn("uid"),

		table.TextColumn("gid"),

		table.TextColumn("mode"),

		table.TextColumn("size"),

		table.TextColumn("modified_time"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("package_bom", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"filepath": "",

			"uid": "",

			"gid": "",

			"mode": "",

			"size": "",

			"modified_time": "",

			"path": "",
		}}, nil
	})
}

func table_package_install_history() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("package_id"),

		table.TextColumn("time"),

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("source"),

		table.TextColumn("content_type"),
	}
	return table.NewPlugin("package_install_history", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"package_id": "",

			"time": "",

			"name": "",

			"version": "",

			"source": "",

			"content_type": "",
		}}, nil
	})
}

func table_package_receipts() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("package_id"),

		table.TextColumn("package_filename"),

		table.TextColumn("version"),

		table.TextColumn("location"),

		table.TextColumn("install_time"),

		table.TextColumn("installer_name"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("package_receipts", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"package_id": "",

			"package_filename": "",

			"version": "",

			"location": "",

			"install_time": "",

			"installer_name": "",

			"path": "",
		}}, nil
	})
}

func table_patches() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("csname"),

		table.TextColumn("hotfix_id"),

		table.TextColumn("caption"),

		table.TextColumn("description"),

		table.TextColumn("fix_comments"),

		table.TextColumn("installed_by"),

		table.TextColumn("install_date"),

		table.TextColumn("installed_on"),
	}
	return table.NewPlugin("patches", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"csname": "",

			"hotfix_id": "",

			"caption": "",

			"description": "",

			"fix_comments": "",

			"installed_by": "",

			"install_date": "",

			"installed_on": "",
		}}, nil
	})
}

func table_pci_devices() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pci_slot"),

		table.TextColumn("pci_class"),

		table.TextColumn("driver"),

		table.TextColumn("vendor"),

		table.TextColumn("vendor_id"),

		table.TextColumn("model"),

		table.TextColumn("model_id"),
	}
	return table.NewPlugin("pci_devices", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pci_slot": "",

			"pci_class": "",

			"driver": "",

			"vendor": "",

			"vendor_id": "",

			"model": "",

			"model_id": "",
		}}, nil
	})
}

func table_physical_disk_performance() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("avg_disk_bytes_per_read"),

		table.TextColumn("avg_disk_bytes_per_write"),

		table.TextColumn("avg_disk_read_queue_length"),

		table.TextColumn("avg_disk_write_queue_length"),

		table.TextColumn("avg_disk_sec_per_read"),

		table.TextColumn("avg_disk_sec_per_write"),

		table.TextColumn("current_disk_queue_length"),

		table.TextColumn("percent_disk_read_time"),

		table.TextColumn("percent_disk_write_time"),

		table.TextColumn("percent_disk_time"),

		table.TextColumn("percent_idle_time"),
	}
	return table.NewPlugin("physical_disk_performance", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"avg_disk_bytes_per_read": "",

			"avg_disk_bytes_per_write": "",

			"avg_disk_read_queue_length": "",

			"avg_disk_write_queue_length": "",

			"avg_disk_sec_per_read": "",

			"avg_disk_sec_per_write": "",

			"current_disk_queue_length": "",

			"percent_disk_read_time": "",

			"percent_disk_write_time": "",

			"percent_disk_time": "",

			"percent_idle_time": "",
		}}, nil
	})
}

func table_pipes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("name"),

		table.TextColumn("instances"),

		table.TextColumn("max_instances"),

		table.TextColumn("flags"),
	}
	return table.NewPlugin("pipes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"name": "",

			"instances": "",

			"max_instances": "",

			"flags": "",
		}}, nil
	})
}

func table_pkg_packages() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("flatsize"),

		table.TextColumn("arch"),
	}
	return table.NewPlugin("pkg_packages", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"version": "",

			"flatsize": "",

			"arch": "",
		}}, nil
	})
}

func table_platform_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("vendor"),

		table.TextColumn("version"),

		table.TextColumn("date"),

		table.TextColumn("revision"),

		table.TextColumn("address"),

		table.TextColumn("size"),

		table.TextColumn("volume_size"),

		table.TextColumn("extra"),
	}
	return table.NewPlugin("platform_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"vendor": "",

			"version": "",

			"date": "",

			"revision": "",

			"address": "",

			"size": "",

			"volume_size": "",

			"extra": "",
		}}, nil
	})
}

func table_plist() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("key"),

		table.TextColumn("subkey"),

		table.TextColumn("value"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("plist", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"key": "",

			"subkey": "",

			"value": "",

			"path": "",
		}}, nil
	})
}

func table_portage_keywords() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("package"),

		table.TextColumn("version"),

		table.TextColumn("keyword"),

		table.TextColumn("mask"),

		table.TextColumn("unmask"),
	}
	return table.NewPlugin("portage_keywords", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"package": "",

			"version": "",

			"keyword": "",

			"mask": "",

			"unmask": "",
		}}, nil
	})
}

func table_portage_packages() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("package"),

		table.TextColumn("version"),

		table.TextColumn("slot"),

		table.TextColumn("build_time"),

		table.TextColumn("repository"),

		table.TextColumn("eapi"),

		table.TextColumn("size"),

		table.TextColumn("world"),
	}
	return table.NewPlugin("portage_packages", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"package": "",

			"version": "",

			"slot": "",

			"build_time": "",

			"repository": "",

			"eapi": "",

			"size": "",

			"world": "",
		}}, nil
	})
}

func table_portage_use() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("package"),

		table.TextColumn("version"),

		table.TextColumn("use"),
	}
	return table.NewPlugin("portage_use", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"package": "",

			"version": "",

			"use": "",
		}}, nil
	})
}

func table_power_sensors() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("key"),

		table.TextColumn("category"),

		table.TextColumn("name"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("power_sensors", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"key": "",

			"category": "",

			"name": "",

			"value": "",
		}}, nil
	})
}

func table_preferences() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("domain"),

		table.TextColumn("key"),

		table.TextColumn("subkey"),

		table.TextColumn("value"),

		table.TextColumn("forced"),

		table.TextColumn("username"),

		table.TextColumn("host"),
	}
	return table.NewPlugin("preferences", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		res := []map[string]string{map[string]string{

			"domain": "",

			"key": "",

			"subkey": "",

			"value": "",

			"forced": "",

			"username": "",

			"host": "",
		}}
		q, ok := queryContext.Constraints["domain"]
		if !ok || len(q.Constraints) == 0 {
			return res, nil
		}
		res[0]["domain"] = q.Constraints[0].Expression
		return res, nil
	})
}

func table_process_envs() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("key"),

		table.TextColumn("value"),
	}
	return table.NewPlugin("process_envs", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"key": "",

			"value": "",
		}}, nil
	})
}

func table_process_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("path"),

		table.TextColumn("mode"),

		table.TextColumn("cmdline"),

		table.TextColumn("cmdline_size"),

		table.TextColumn("env"),

		table.TextColumn("env_count"),

		table.TextColumn("env_size"),

		table.TextColumn("cwd"),

		table.TextColumn("auid"),

		table.TextColumn("uid"),

		table.TextColumn("euid"),

		table.TextColumn("gid"),

		table.TextColumn("egid"),

		table.TextColumn("owner_uid"),

		table.TextColumn("owner_gid"),

		table.TextColumn("atime"),

		table.TextColumn("mtime"),

		table.TextColumn("ctime"),

		table.TextColumn("btime"),

		table.TextColumn("overflows"),

		table.TextColumn("parent"),

		table.TextColumn("time"),

		table.TextColumn("uptime"),

		table.TextColumn("status"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("process_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"path": "",

			"mode": "",

			"cmdline": "",

			"cmdline_size": "",

			"env": "",

			"env_count": "",

			"env_size": "",

			"cwd": "",

			"auid": "",

			"uid": "",

			"euid": "",

			"gid": "",

			"egid": "",

			"owner_uid": "",

			"owner_gid": "",

			"atime": "",

			"mtime": "",

			"ctime": "",

			"btime": "",

			"overflows": "",

			"parent": "",

			"time": "",

			"uptime": "",

			"status": "",

			"eid": "",
		}}, nil
	})
}

func table_process_file_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("action"),

		table.TextColumn("pid"),

		table.TextColumn("path"),

		table.TextColumn("parent"),

		table.TextColumn("uid"),

		table.TextColumn("euid"),

		table.TextColumn("gid"),

		table.TextColumn("egid"),

		table.TextColumn("mode"),

		table.TextColumn("owner_uid"),

		table.TextColumn("owner_gid"),

		table.TextColumn("atime"),

		table.TextColumn("mtime"),

		table.TextColumn("ctime"),

		table.TextColumn("time"),

		table.TextColumn("uptime"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("process_file_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"action": "",

			"pid": "",

			"path": "",

			"parent": "",

			"uid": "",

			"euid": "",

			"gid": "",

			"egid": "",

			"mode": "",

			"owner_uid": "",

			"owner_gid": "",

			"atime": "",

			"mtime": "",

			"ctime": "",

			"time": "",

			"uptime": "",

			"eid": "",
		}}, nil
	})
}

func table_process_memory_map() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("start"),

		table.TextColumn("end"),

		table.TextColumn("permissions"),

		table.TextColumn("offset"),

		table.TextColumn("device"),

		table.TextColumn("inode"),

		table.TextColumn("path"),

		table.TextColumn("pseudo"),
	}
	return table.NewPlugin("process_memory_map", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"start": "",

			"end": "",

			"permissions": "",

			"offset": "",

			"device": "",

			"inode": "",

			"path": "",

			"pseudo": "",
		}}, nil
	})
}

func table_process_open_files() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("fd"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("process_open_files", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"fd": "",

			"path": "",
		}}, nil
	})
}

func table_process_open_sockets() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("fd"),

		table.TextColumn("socket"),

		table.TextColumn("family"),

		table.TextColumn("protocol"),

		table.TextColumn("local_address"),

		table.TextColumn("remote_address"),

		table.TextColumn("local_port"),

		table.TextColumn("remote_port"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("process_open_sockets", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"fd": "",

			"socket": "",

			"family": "",

			"protocol": "",

			"local_address": "",

			"remote_address": "",

			"local_port": "",

			"remote_port": "",

			"path": "",
		}}, nil
	})
}

func table_processes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("pid"),

		table.TextColumn("name"),

		table.TextColumn("path"),

		table.TextColumn("cmdline"),

		table.TextColumn("state"),

		table.TextColumn("cwd"),

		table.TextColumn("root"),

		table.TextColumn("uid"),

		table.TextColumn("gid"),

		table.TextColumn("euid"),

		table.TextColumn("egid"),

		table.TextColumn("suid"),

		table.TextColumn("sgid"),

		table.TextColumn("on_disk"),

		table.TextColumn("wired_size"),

		table.TextColumn("resident_size"),

		table.TextColumn("total_size"),

		table.TextColumn("user_time"),

		table.TextColumn("system_time"),

		table.TextColumn("start_time"),

		table.TextColumn("parent"),

		table.TextColumn("pgroup"),

		table.TextColumn("threads"),

		table.TextColumn("nice"),
	}
	return table.NewPlugin("processes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"pid": "",

			"name": "",

			"path": "",

			"cmdline": "",

			"state": "",

			"cwd": "",

			"root": "",

			"uid": "",

			"gid": "",

			"euid": "",

			"egid": "",

			"suid": "",

			"sgid": "",

			"on_disk": "",

			"wired_size": "",

			"resident_size": "",

			"total_size": "",

			"user_time": "",

			"system_time": "",

			"start_time": "",

			"parent": "",

			"pgroup": "",

			"threads": "",

			"nice": "",
		}}, nil
	})
}

func table_programs() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("install_location"),

		table.TextColumn("install_source"),

		table.TextColumn("language"),

		table.TextColumn("publisher"),

		table.TextColumn("uninstall_string"),

		table.TextColumn("install_date"),

		table.TextColumn("identifying_number"),
	}
	return table.NewPlugin("programs", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"version": "",

			"install_location": "",

			"install_source": "",

			"language": "",

			"publisher": "",

			"uninstall_string": "",

			"install_date": "",

			"identifying_number": "",
		}}, nil
	})
}

func table_prometheus_metrics() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("target_name"),

		table.TextColumn("metric_name"),

		table.TextColumn("metric_value"),

		table.TextColumn("timestamp_ms"),
	}
	return table.NewPlugin("prometheus_metrics", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"target_name": "",

			"metric_name": "",

			"metric_value": "",

			"timestamp_ms": "",
		}}, nil
	})
}

func table_python_packages() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("summary"),

		table.TextColumn("author"),

		table.TextColumn("license"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("python_packages", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"version": "",

			"summary": "",

			"author": "",

			"license": "",

			"path": "",
		}}, nil
	})
}

func table_quicklook_cache() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("rowid"),

		table.TextColumn("fs_id"),

		table.TextColumn("volume_id"),

		table.TextColumn("inode"),

		table.TextColumn("mtime"),

		table.TextColumn("size"),

		table.TextColumn("label"),

		table.TextColumn("last_hit_date"),

		table.TextColumn("hit_count"),

		table.TextColumn("icon_mode"),

		table.TextColumn("cache_path"),
	}
	return table.NewPlugin("quicklook_cache", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"rowid": "",

			"fs_id": "",

			"volume_id": "",

			"inode": "",

			"mtime": "",

			"size": "",

			"label": "",

			"last_hit_date": "",

			"hit_count": "",

			"icon_mode": "",

			"cache_path": "",
		}}, nil
	})
}

func table_registry() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("key"),

		table.TextColumn("path"),

		table.TextColumn("name"),

		table.TextColumn("type"),

		table.TextColumn("data"),

		table.TextColumn("mtime"),
	}
	return table.NewPlugin("registry", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"key": "",

			"path": "",

			"name": "",

			"type": "",

			"data": "",

			"mtime": "",
		}}, nil
	})
}

func table_routes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("destination"),

		table.TextColumn("netmask"),

		table.TextColumn("gateway"),

		table.TextColumn("source"),

		table.TextColumn("flags"),

		table.TextColumn("interface"),

		table.TextColumn("mtu"),

		table.TextColumn("metric"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("routes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"destination": "",

			"netmask": "",

			"gateway": "",

			"source": "",

			"flags": "",

			"interface": "",

			"mtu": "",

			"metric": "",

			"type": "",
		}}, nil
	})
}

func table_rpm_package_files() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("package"),

		table.TextColumn("path"),

		table.TextColumn("username"),

		table.TextColumn("groupname"),

		table.TextColumn("mode"),

		table.TextColumn("size"),

		table.TextColumn("sha256"),
	}
	return table.NewPlugin("rpm_package_files", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"package": "",

			"path": "",

			"username": "",

			"groupname": "",

			"mode": "",

			"size": "",

			"sha256": "",
		}}, nil
	})
}

func table_rpm_packages() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("version"),

		table.TextColumn("release"),

		table.TextColumn("source"),

		table.TextColumn("size"),

		table.TextColumn("sha1"),

		table.TextColumn("arch"),
	}
	return table.NewPlugin("rpm_packages", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"version": "",

			"release": "",

			"source": "",

			"size": "",

			"sha1": "",

			"arch": "",
		}}, nil
	})
}

func table_safari_extensions() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("name"),

		table.TextColumn("identifier"),

		table.TextColumn("version"),

		table.TextColumn("sdk"),

		table.TextColumn("update_url"),

		table.TextColumn("author"),

		table.TextColumn("developer_id"),

		table.TextColumn("description"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("safari_extensions", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"name": "",

			"identifier": "",

			"version": "",

			"sdk": "",

			"update_url": "",

			"author": "",

			"developer_id": "",

			"description": "",

			"path": "",
		}}, nil
	})
}

func table_sandboxes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("label"),

		table.TextColumn("user"),

		table.TextColumn("enabled"),

		table.TextColumn("build_id"),

		table.TextColumn("bundle_path"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("sandboxes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"label": "",

			"user": "",

			"enabled": "",

			"build_id": "",

			"bundle_path": "",

			"path": "",
		}}, nil
	})
}

func table_scheduled_tasks() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("action"),

		table.TextColumn("path"),

		table.TextColumn("enabled"),

		table.TextColumn("state"),

		table.TextColumn("hidden"),

		table.TextColumn("last_run_time"),

		table.TextColumn("next_run_time"),

		table.TextColumn("last_run_message"),

		table.TextColumn("last_run_code"),
	}
	return table.NewPlugin("scheduled_tasks", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"action": "",

			"path": "",

			"enabled": "",

			"state": "",

			"hidden": "",

			"last_run_time": "",

			"next_run_time": "",

			"last_run_message": "",

			"last_run_code": "",
		}}, nil
	})
}

func table_services() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("service_type"),

		table.TextColumn("display_name"),

		table.TextColumn("status"),

		table.TextColumn("pid"),

		table.TextColumn("start_type"),

		table.TextColumn("win32_exit_code"),

		table.TextColumn("service_exit_code"),

		table.TextColumn("path"),

		table.TextColumn("module_path"),

		table.TextColumn("description"),

		table.TextColumn("user_account"),
	}
	return table.NewPlugin("services", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"service_type": "",

			"display_name": "",

			"status": "",

			"pid": "",

			"start_type": "",

			"win32_exit_code": "",

			"service_exit_code": "",

			"path": "",

			"module_path": "",

			"description": "",

			"user_account": "",
		}}, nil
	})
}

func table_shadow() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("password_status"),

		table.TextColumn("hash_alg"),

		table.TextColumn("last_change"),

		table.TextColumn("min"),

		table.TextColumn("max"),

		table.TextColumn("warning"),

		table.TextColumn("inactive"),

		table.TextColumn("expire"),

		table.TextColumn("flag"),

		table.TextColumn("username"),
	}
	return table.NewPlugin("shadow", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"password_status": "",

			"hash_alg": "",

			"last_change": "",

			"min": "",

			"max": "",

			"warning": "",

			"inactive": "",

			"expire": "",

			"flag": "",

			"username": "",
		}}, nil
	})
}

func table_shared_folders() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("path"),
	}
	return table.NewPlugin("shared_folders", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"path": "",
		}}, nil
	})
}

func table_shared_memory() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("shmid"),

		table.TextColumn("owner_uid"),

		table.TextColumn("creator_uid"),

		table.TextColumn("pid"),

		table.TextColumn("creator_pid"),

		table.TextColumn("atime"),

		table.TextColumn("dtime"),

		table.TextColumn("ctime"),

		table.TextColumn("permissions"),

		table.TextColumn("size"),

		table.TextColumn("attached"),

		table.TextColumn("status"),

		table.TextColumn("locked"),
	}
	return table.NewPlugin("shared_memory", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"shmid": "",

			"owner_uid": "",

			"creator_uid": "",

			"pid": "",

			"creator_pid": "",

			"atime": "",

			"dtime": "",

			"ctime": "",

			"permissions": "",

			"size": "",

			"attached": "",

			"status": "",

			"locked": "",
		}}, nil
	})
}

func table_shared_resources() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("description"),

		table.TextColumn("install_date"),

		table.TextColumn("status"),

		table.TextColumn("allow_maximum"),

		table.TextColumn("maximum_allowed"),

		table.TextColumn("name"),

		table.TextColumn("path"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("shared_resources", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"description": "",

			"install_date": "",

			"status": "",

			"allow_maximum": "",

			"maximum_allowed": "",

			"name": "",

			"path": "",

			"type": "",
		}}, nil
	})
}

func table_sharing_preferences() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("screen_sharing"),

		table.TextColumn("file_sharing"),

		table.TextColumn("printer_sharing"),

		table.TextColumn("remote_login"),

		table.TextColumn("remote_management"),

		table.TextColumn("remote_apple_events"),

		table.TextColumn("internet_sharing"),

		table.TextColumn("bluetooth_sharing"),

		table.TextColumn("disc_sharing"),
	}
	return table.NewPlugin("sharing_preferences", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"screen_sharing": "",

			"file_sharing": "",

			"printer_sharing": "",

			"remote_login": "",

			"remote_management": "",

			"remote_apple_events": "",

			"internet_sharing": "",

			"bluetooth_sharing": "",

			"disc_sharing": "",
		}}, nil
	})
}

func table_shell_history() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("time"),

		table.TextColumn("command"),

		table.TextColumn("history_file"),
	}
	return table.NewPlugin("shell_history", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"time": "",

			"command": "",

			"history_file": "",
		}}, nil
	})
}

func table_signature() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("signed"),

		table.TextColumn("identifier"),

		table.TextColumn("cdhash"),

		table.TextColumn("team_identifier"),

		table.TextColumn("authority"),
	}
	return table.NewPlugin("signature", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"signed": "",

			"identifier": "",

			"cdhash": "",

			"team_identifier": "",

			"authority": "",
		}}, nil
	})
}

func table_sip_config() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("config_flag"),

		table.TextColumn("enabled"),

		table.TextColumn("enabled_nvram"),
	}
	return table.NewPlugin("sip_config", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"config_flag": "",

			"enabled": "",

			"enabled_nvram": "",
		}}, nil
	})
}

func table_smbios_tables() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("number"),

		table.TextColumn("type"),

		table.TextColumn("description"),

		table.TextColumn("handle"),

		table.TextColumn("header_size"),

		table.TextColumn("size"),

		table.TextColumn("md5"),
	}
	return table.NewPlugin("smbios_tables", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"number": "",

			"type": "",

			"description": "",

			"handle": "",

			"header_size": "",

			"size": "",

			"md5": "",
		}}, nil
	})
}

func table_smc_keys() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("key"),

		table.TextColumn("type"),

		table.TextColumn("size"),

		table.TextColumn("value"),

		table.TextColumn("hidden"),
	}
	return table.NewPlugin("smc_keys", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"key": "",

			"type": "",

			"size": "",

			"value": "",

			"hidden": "",
		}}, nil
	})
}

func table_socket_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("action"),

		table.TextColumn("pid"),

		table.TextColumn("path"),

		table.TextColumn("fd"),

		table.TextColumn("auid"),

		table.TextColumn("success"),

		table.TextColumn("family"),

		table.TextColumn("protocol"),

		table.TextColumn("local_address"),

		table.TextColumn("remote_address"),

		table.TextColumn("local_port"),

		table.TextColumn("remote_port"),

		table.TextColumn("socket"),

		table.TextColumn("time"),

		table.TextColumn("uptime"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("socket_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"action": "",

			"pid": "",

			"path": "",

			"fd": "",

			"auid": "",

			"success": "",

			"family": "",

			"protocol": "",

			"local_address": "",

			"remote_address": "",

			"local_port": "",

			"remote_port": "",

			"socket": "",

			"time": "",

			"uptime": "",

			"eid": "",
		}}, nil
	})
}

func table_startup_items() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("path"),

		table.TextColumn("args"),

		table.TextColumn("type"),

		table.TextColumn("source"),

		table.TextColumn("status"),

		table.TextColumn("username"),
	}
	return table.NewPlugin("startup_items", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"path": "",

			"args": "",

			"type": "",

			"source": "",

			"status": "",

			"username": "",
		}}, nil
	})
}

func table_sudoers() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("header"),

		table.TextColumn("rule_details"),
	}
	return table.NewPlugin("sudoers", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"header": "",

			"rule_details": "",
		}}, nil
	})
}

func table_suid_bin() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("username"),

		table.TextColumn("groupname"),

		table.TextColumn("permissions"),
	}
	return table.NewPlugin("suid_bin", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"username": "",

			"groupname": "",

			"permissions": "",
		}}, nil
	})
}

func table_syslog_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("time"),

		table.TextColumn("datetime"),

		table.TextColumn("host"),

		table.TextColumn("severity"),

		table.TextColumn("facility"),

		table.TextColumn("tag"),

		table.TextColumn("message"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("syslog_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"time": "",

			"datetime": "",

			"host": "",

			"severity": "",

			"facility": "",

			"tag": "",

			"message": "",

			"eid": "",
		}}, nil
	})
}

func table_system_controls() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("oid"),

		table.TextColumn("subsystem"),

		table.TextColumn("current_value"),

		table.TextColumn("config_value"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("system_controls", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"oid": "",

			"subsystem": "",

			"current_value": "",

			"config_value": "",

			"type": "",
		}}, nil
	})
}

func table_system_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("hostname"),

		table.TextColumn("uuid"),

		table.TextColumn("cpu_type"),

		table.TextColumn("cpu_subtype"),

		table.TextColumn("cpu_brand"),

		table.TextColumn("cpu_physical_cores"),

		table.TextColumn("cpu_logical_cores"),

		table.TextColumn("physical_memory"),

		table.TextColumn("hardware_vendor"),

		table.TextColumn("hardware_model"),

		table.TextColumn("hardware_version"),

		table.TextColumn("hardware_serial"),

		table.TextColumn("computer_name"),

		table.TextColumn("local_hostname"),
	}
	return table.NewPlugin("system_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"hostname": "",

			"uuid": "",

			"cpu_type": "",

			"cpu_subtype": "",

			"cpu_brand": "",

			"cpu_physical_cores": "",

			"cpu_logical_cores": "",

			"physical_memory": "",

			"hardware_vendor": "",

			"hardware_model": "",

			"hardware_version": "",

			"hardware_serial": "",

			"computer_name": "",

			"local_hostname": "",
		}}, nil
	})
}

func table_temperature_sensors() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("key"),

		table.TextColumn("name"),

		table.TextColumn("celsius"),

		table.TextColumn("fahrenheit"),
	}
	return table.NewPlugin("temperature_sensors", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"key": "",

			"name": "",

			"celsius": "",

			"fahrenheit": "",
		}}, nil
	})
}

func table_time() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("weekday"),

		table.TextColumn("year"),

		table.TextColumn("month"),

		table.TextColumn("day"),

		table.TextColumn("hour"),

		table.TextColumn("minutes"),

		table.TextColumn("seconds"),

		table.TextColumn("timezone"),

		table.TextColumn("local_time"),

		table.TextColumn("local_timezone"),

		table.TextColumn("unix_time"),

		table.TextColumn("timestamp"),

		table.TextColumn("datetime"),

		table.TextColumn("iso_8601"),
	}
	return table.NewPlugin("time", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"weekday": "",

			"year": "",

			"month": "",

			"day": "",

			"hour": "",

			"minutes": "",

			"seconds": "",

			"timezone": "",

			"local_time": "",

			"local_timezone": "",

			"unix_time": "",

			"timestamp": "",

			"datetime": "",

			"iso_8601": "",
		}}, nil
	})
}

func table_time_machine_backups() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("destination_id"),

		table.TextColumn("backup_date"),
	}
	return table.NewPlugin("time_machine_backups", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"destination_id": "",

			"backup_date": "",
		}}, nil
	})
}

func table_time_machine_destinations() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("alias"),

		table.TextColumn("destination_id"),

		table.TextColumn("consistency_scan_date"),

		table.TextColumn("root_volume_uuid"),

		table.TextColumn("bytes_available"),

		table.TextColumn("bytes_used"),

		table.TextColumn("encryption"),
	}
	return table.NewPlugin("time_machine_destinations", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"alias": "",

			"destination_id": "",

			"consistency_scan_date": "",

			"root_volume_uuid": "",

			"bytes_available": "",

			"bytes_used": "",

			"encryption": "",
		}}, nil
	})
}

func table_uptime() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("days"),

		table.TextColumn("hours"),

		table.TextColumn("minutes"),

		table.TextColumn("seconds"),

		table.TextColumn("total_seconds"),
	}
	return table.NewPlugin("uptime", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"days": "",

			"hours": "",

			"minutes": "",

			"seconds": "",

			"total_seconds": "",
		}}, nil
	})
}

func table_usb_devices() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("usb_address"),

		table.TextColumn("usb_port"),

		table.TextColumn("vendor"),

		table.TextColumn("vendor_id"),

		table.TextColumn("version"),

		table.TextColumn("model"),

		table.TextColumn("model_id"),

		table.TextColumn("serial"),

		table.TextColumn("class"),

		table.TextColumn("subclass"),

		table.TextColumn("protocol"),

		table.TextColumn("removable"),
	}
	return table.NewPlugin("usb_devices", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"usb_address": "",

			"usb_port": "",

			"vendor": "",

			"vendor_id": "",

			"version": "",

			"model": "",

			"model_id": "",

			"serial": "",

			"class": "",

			"subclass": "",

			"protocol": "",

			"removable": "",
		}}, nil
	})
}

func table_user_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("auid"),

		table.TextColumn("pid"),

		table.TextColumn("message"),

		table.TextColumn("type"),

		table.TextColumn("path"),

		table.TextColumn("address"),

		table.TextColumn("terminal"),

		table.TextColumn("time"),

		table.TextColumn("uptime"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("user_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"auid": "",

			"pid": "",

			"message": "",

			"type": "",

			"path": "",

			"address": "",

			"terminal": "",

			"time": "",

			"uptime": "",

			"eid": "",
		}}, nil
	})
}

func table_user_groups() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("gid"),
	}
	return table.NewPlugin("user_groups", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"gid": "",
		}}, nil
	})
}

func table_user_interaction_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("time"),
	}
	return table.NewPlugin("user_interaction_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"time": "",
		}}, nil
	})
}

func table_user_ssh_keys() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("path"),

		table.TextColumn("encrypted"),
	}
	return table.NewPlugin("user_ssh_keys", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"path": "",

			"encrypted": "",
		}}, nil
	})
}

func table_users() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("uid"),

		table.TextColumn("gid"),

		table.TextColumn("uid_signed"),

		table.TextColumn("gid_signed"),

		table.TextColumn("username"),

		table.TextColumn("description"),

		table.TextColumn("directory"),

		table.TextColumn("shell"),

		table.TextColumn("uuid"),

		table.TextColumn("type"),
	}
	return table.NewPlugin("users", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"uid": "",

			"gid": "",

			"uid_signed": "",

			"gid_signed": "",

			"username": "",

			"description": "",

			"directory": "",

			"shell": "",

			"uuid": "",

			"type": "",
		}}, nil
	})
}

func table_virtual_memory_info() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("free"),

		table.TextColumn("active"),

		table.TextColumn("inactive"),

		table.TextColumn("speculative"),

		table.TextColumn("throttled"),

		table.TextColumn("wired"),

		table.TextColumn("purgeable"),

		table.TextColumn("faults"),

		table.TextColumn("copy"),

		table.TextColumn("zero_fill"),

		table.TextColumn("reactivated"),

		table.TextColumn("purged"),

		table.TextColumn("file_backed"),

		table.TextColumn("anonymous"),

		table.TextColumn("uncompressed"),

		table.TextColumn("compressor"),

		table.TextColumn("decompressed"),

		table.TextColumn("compressed"),

		table.TextColumn("page_ins"),

		table.TextColumn("page_outs"),

		table.TextColumn("swap_ins"),

		table.TextColumn("swap_outs"),
	}
	return table.NewPlugin("virtual_memory_info", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"free": "",

			"active": "",

			"inactive": "",

			"speculative": "",

			"throttled": "",

			"wired": "",

			"purgeable": "",

			"faults": "",

			"copy": "",

			"zero_fill": "",

			"reactivated": "",

			"purged": "",

			"file_backed": "",

			"anonymous": "",

			"uncompressed": "",

			"compressor": "",

			"decompressed": "",

			"compressed": "",

			"page_ins": "",

			"page_outs": "",

			"swap_ins": "",

			"swap_outs": "",
		}}, nil
	})
}

func table_wifi_networks() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("ssid"),

		table.TextColumn("network_name"),

		table.TextColumn("security_type"),

		table.TextColumn("last_connected"),

		table.TextColumn("passpoint"),

		table.TextColumn("possibly_hidden"),

		table.TextColumn("roaming"),

		table.TextColumn("roaming_profile"),

		table.TextColumn("captive_portal"),

		table.TextColumn("auto_login"),

		table.TextColumn("temporarily_disabled"),

		table.TextColumn("disabled"),
	}
	return table.NewPlugin("wifi_networks", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"ssid": "",

			"network_name": "",

			"security_type": "",

			"last_connected": "",

			"passpoint": "",

			"possibly_hidden": "",

			"roaming": "",

			"roaming_profile": "",

			"captive_portal": "",

			"auto_login": "",

			"temporarily_disabled": "",

			"disabled": "",
		}}, nil
	})
}

func table_wifi_status() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("interface"),

		table.TextColumn("ssid"),

		table.TextColumn("bssid"),

		table.TextColumn("network_name"),

		table.TextColumn("country_code"),

		table.TextColumn("security_type"),

		table.TextColumn("rssi"),

		table.TextColumn("noise"),

		table.TextColumn("channel"),

		table.TextColumn("channel_width"),

		table.TextColumn("channel_band"),

		table.TextColumn("transmit_rate"),

		table.TextColumn("mode"),
	}
	return table.NewPlugin("wifi_status", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"interface": "",

			"ssid": "",

			"bssid": "",

			"network_name": "",

			"country_code": "",

			"security_type": "",

			"rssi": "",

			"noise": "",

			"channel": "",

			"channel_width": "",

			"channel_band": "",

			"transmit_rate": "",

			"mode": "",
		}}, nil
	})
}

func table_wifi_survey() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("interface"),

		table.TextColumn("ssid"),

		table.TextColumn("bssid"),

		table.TextColumn("network_name"),

		table.TextColumn("country_code"),

		table.TextColumn("rssi"),

		table.TextColumn("noise"),

		table.TextColumn("channel"),

		table.TextColumn("channel_width"),

		table.TextColumn("channel_band"),
	}
	return table.NewPlugin("wifi_survey", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"interface": "",

			"ssid": "",

			"bssid": "",

			"network_name": "",

			"country_code": "",

			"rssi": "",

			"noise": "",

			"channel": "",

			"channel_width": "",

			"channel_band": "",
		}}, nil
	})
}

func table_windows_crashes() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("datetime"),

		table.TextColumn("module"),

		table.TextColumn("path"),

		table.TextColumn("pid"),

		table.TextColumn("tid"),

		table.TextColumn("version"),

		table.TextColumn("process_uptime"),

		table.TextColumn("stack_trace"),

		table.TextColumn("exception_code"),

		table.TextColumn("exception_message"),

		table.TextColumn("exception_address"),

		table.TextColumn("registers"),

		table.TextColumn("command_line"),

		table.TextColumn("current_directory"),

		table.TextColumn("username"),

		table.TextColumn("machine_name"),

		table.TextColumn("major_version"),

		table.TextColumn("minor_version"),

		table.TextColumn("build_number"),

		table.TextColumn("type"),

		table.TextColumn("crash_path"),
	}
	return table.NewPlugin("windows_crashes", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"datetime": "",

			"module": "",

			"path": "",

			"pid": "",

			"tid": "",

			"version": "",

			"process_uptime": "",

			"stack_trace": "",

			"exception_code": "",

			"exception_message": "",

			"exception_address": "",

			"registers": "",

			"command_line": "",

			"current_directory": "",

			"username": "",

			"machine_name": "",

			"major_version": "",

			"minor_version": "",

			"build_number": "",

			"type": "",

			"crash_path": "",
		}}, nil
	})
}

func table_windows_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("time"),

		table.TextColumn("datetime"),

		table.TextColumn("source"),

		table.TextColumn("provider_name"),

		table.TextColumn("provider_guid"),

		table.TextColumn("eventid"),

		table.TextColumn("task"),

		table.TextColumn("level"),

		table.TextColumn("keywords"),

		table.TextColumn("data"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("windows_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"time": "",

			"datetime": "",

			"source": "",

			"provider_name": "",

			"provider_guid": "",

			"eventid": "",

			"task": "",

			"level": "",

			"keywords": "",

			"data": "",

			"eid": "",
		}}, nil
	})
}

func table_wmi_cli_event_consumers() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("command_line_template"),

		table.TextColumn("executable_path"),

		table.TextColumn("class"),

		table.TextColumn("relative_path"),
	}
	return table.NewPlugin("wmi_cli_event_consumers", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"command_line_template": "",

			"executable_path": "",

			"class": "",

			"relative_path": "",
		}}, nil
	})
}

func table_wmi_event_filters() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("query"),

		table.TextColumn("query_language"),

		table.TextColumn("class"),

		table.TextColumn("relative_path"),
	}
	return table.NewPlugin("wmi_event_filters", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"query": "",

			"query_language": "",

			"class": "",

			"relative_path": "",
		}}, nil
	})
}

func table_wmi_filter_consumer_binding() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("consumer"),

		table.TextColumn("filter"),

		table.TextColumn("class"),

		table.TextColumn("relative_path"),
	}
	return table.NewPlugin("wmi_filter_consumer_binding", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"consumer": "",

			"filter": "",

			"class": "",

			"relative_path": "",
		}}, nil
	})
}

func table_wmi_script_event_consumers() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("scripting_engine"),

		table.TextColumn("script_file_name"),

		table.TextColumn("script_text"),

		table.TextColumn("class"),

		table.TextColumn("relative_path"),
	}
	return table.NewPlugin("wmi_script_event_consumers", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"scripting_engine": "",

			"script_file_name": "",

			"script_text": "",

			"class": "",

			"relative_path": "",
		}}, nil
	})
}

func table_xprotect_entries() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("launch_type"),

		table.TextColumn("identity"),

		table.TextColumn("filename"),

		table.TextColumn("filetype"),

		table.TextColumn("optional"),

		table.TextColumn("uses_pattern"),
	}
	return table.NewPlugin("xprotect_entries", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"launch_type": "",

			"identity": "",

			"filename": "",

			"filetype": "",

			"optional": "",

			"uses_pattern": "",
		}}, nil
	})
}

func table_xprotect_meta() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("identifier"),

		table.TextColumn("type"),

		table.TextColumn("developer_id"),

		table.TextColumn("min_version"),
	}
	return table.NewPlugin("xprotect_meta", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"identifier": "",

			"type": "",

			"developer_id": "",

			"min_version": "",
		}}, nil
	})
}

func table_xprotect_reports() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("name"),

		table.TextColumn("user_action"),

		table.TextColumn("time"),
	}
	return table.NewPlugin("xprotect_reports", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"name": "",

			"user_action": "",

			"time": "",
		}}, nil
	})
}

func table_yara() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("path"),

		table.TextColumn("matches"),

		table.TextColumn("count"),

		table.TextColumn("sig_group"),

		table.TextColumn("sigfile"),

		table.TextColumn("strings"),

		table.TextColumn("tags"),
	}
	return table.NewPlugin("yara", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"path": "",

			"matches": "",

			"count": "",

			"sig_group": "",

			"sigfile": "",

			"strings": "",

			"tags": "",
		}}, nil
	})
}

func table_yara_events() *table.Plugin {
	columns := []table.ColumnDefinition{

		table.TextColumn("target_path"),

		table.TextColumn("category"),

		table.TextColumn("action"),

		table.TextColumn("transaction_id"),

		table.TextColumn("matches"),

		table.TextColumn("count"),

		table.TextColumn("strings"),

		table.TextColumn("tags"),

		table.TextColumn("time"),

		table.TextColumn("eid"),
	}
	return table.NewPlugin("yara_events", columns, func(ctx context.Context, queryContext table.QueryContext) ([]map[string]string, error) {
		return []map[string]string{map[string]string{

			"target_path": "",

			"category": "",

			"action": "",

			"transaction_id": "",

			"matches": "",

			"count": "",

			"strings": "",

			"tags": "",

			"time": "",

			"eid": "",
		}}, nil
	})
}
