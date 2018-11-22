load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    urls = ["https://github.com/bazelbuild/rules_go/releases/download/0.16.2/rules_go-0.16.2.tar.gz"],
    sha256 = "f87fa87475ea107b3c69196f39c82b7bbf58fe27c62a338684c20ca17d1d8613",
)

http_archive(
    name = "bazel_gazelle",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.15.0/bazel-gazelle-0.15.0.tar.gz"],
    sha256 = "6e875ab4b6bf64a38c352887760f21203ab054676d9c1b274963907e0768740d",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains")

go_rules_dependencies()

go_register_toolchains()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

go_repository(
    name = "com_github_go_kit_kit",
    importpath = "github.com/go-kit/kit",
    tag = "v0.7.0",
)

go_repository(
    name = "com_github_agl_ed25519",
    commit = "5312a6153412",
    importpath = "github.com/agl/ed25519",
)

go_repository(
    name = "com_github_beorn7_perks",
    commit = "3a771d992973",
    importpath = "github.com/beorn7/perks",
)

go_repository(
    name = "com_github_bitly_go_hostpool",
    commit = "a3a6125de932",
    importpath = "github.com/bitly/go-hostpool",
)

go_repository(
    name = "com_github_bitly_go_simplejson",
    importpath = "github.com/bitly/go-simplejson",
    tag = "v0.5.0",
)

go_repository(
    name = "com_github_bmizerany_assert",
    commit = "b7ed37b82869",
    importpath = "github.com/bmizerany/assert",
)

go_repository(
    name = "com_github_boltdb_bolt",
    importpath = "github.com/boltdb/bolt",
    tag = "v1.3.1",
)

go_repository(
    name = "com_github_bugsnag_bugsnag_go",
    importpath = "github.com/bugsnag/bugsnag-go",
    tag = "v1.3.2",
)

go_repository(
    name = "com_github_bugsnag_panicwrap",
    importpath = "github.com/bugsnag/panicwrap",
    tag = "v1.2.0",
)

go_repository(
    name = "com_github_burntsushi_toml",
    importpath = "github.com/BurntSushi/toml",
    tag = "v0.3.1",
)

go_repository(
    name = "com_github_cenkalti_backoff",
    importpath = "github.com/cenkalti/backoff",
    tag = "v2.0.0",
)

go_repository(
    name = "com_github_cloudflare_cfssl",
    commit = "ea4033a214e7",
    importpath = "github.com/cloudflare/cfssl",
)

go_repository(
    name = "com_github_davecgh_go_spew",
    importpath = "github.com/davecgh/go-spew",
    tag = "v1.1.1",
)

go_repository(
    name = "com_github_denisenkom_go_mssqldb",
    commit = "4e0d7dc8888f",
    importpath = "github.com/denisenkom/go-mssqldb",
)

go_repository(
    name = "com_github_docker_distribution",
    importpath = "github.com/docker/distribution",
    tag = "v2.6.2",
)

go_repository(
    name = "com_github_docker_go",
    importpath = "github.com/docker/go",
    tag = "v1.5.1-1",
)

go_repository(
    name = "com_github_docker_go_connections",
    importpath = "github.com/docker/go-connections",
    tag = "v0.4.0",
)

go_repository(
    name = "com_github_erikstmartin_go_testdb",
    commit = "8d10e4a1bae5",
    importpath = "github.com/erikstmartin/go-testdb",
)

go_repository(
    name = "com_github_fsnotify_fsnotify",
    importpath = "github.com/fsnotify/fsnotify",
    tag = "v1.4.7",
)

go_repository(
    name = "com_github_ghodss_yaml",
    importpath = "github.com/ghodss/yaml",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_go_logfmt_logfmt",
    importpath = "github.com/go-logfmt/logfmt",
    tag = "v0.3.0",
)

go_repository(
    name = "com_github_go_sql_driver_mysql",
    importpath = "github.com/go-sql-driver/mysql",
    tag = "v1.4.1",
)

go_repository(
    name = "com_github_go_stack_stack",
    importpath = "github.com/go-stack/stack",
    tag = "v1.7.0",
)

go_repository(
    name = "com_github_gogo_protobuf",
    importpath = "github.com/gogo/protobuf",
    tag = "v1.1.1",
)

go_repository(
    name = "com_github_golang_glog",
    commit = "23def4e6c14b",
    importpath = "github.com/golang/glog",
)

go_repository(
    name = "com_github_golang_protobuf",
    importpath = "github.com/golang/protobuf",
    tag = "v1.2.0",
)

go_repository(
    name = "com_github_google_certificate_transparency_go",
    importpath = "github.com/google/certificate-transparency-go",
    tag = "v1.0.21",
)

go_repository(
    name = "com_github_google_go_cmp",
    importpath = "github.com/google/go-cmp",
    tag = "v0.2.0",
)

go_repository(
    name = "com_github_google_uuid",
    commit = "064e2069ce9c",
    importpath = "github.com/google/uuid",
)

go_repository(
    name = "com_github_gorilla_context",
    importpath = "github.com/gorilla/context",
    tag = "v1.1.1",
)

go_repository(
    name = "com_github_gorilla_mux",
    importpath = "github.com/gorilla/mux",
    tag = "v1.6.2",
)

go_repository(
    name = "com_github_gorilla_websocket",
    importpath = "github.com/gorilla/websocket",
    tag = "v1.2.0",
)

go_repository(
    name = "com_github_groob_plist",
    commit = "2805b357fb23",
    importpath = "github.com/groob/plist",
)

go_repository(
    name = "com_github_hailocab_go_hostpool",
    commit = "e80d13ce29ed",
    importpath = "github.com/hailocab/go-hostpool",
)

go_repository(
    name = "com_github_hashicorp_hcl",
    importpath = "github.com/hashicorp/hcl",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_hpcloud_tail",
    importpath = "github.com/hpcloud/tail",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_jinzhu_gorm",
    importpath = "github.com/jinzhu/gorm",
    tag = "v1.9.1",
)

go_repository(
    name = "com_github_jinzhu_inflection",
    commit = "04140366298a",
    importpath = "github.com/jinzhu/inflection",
)

go_repository(
    name = "com_github_jinzhu_now",
    commit = "8ec929ed50c3",
    importpath = "github.com/jinzhu/now",
)

go_repository(
    name = "com_github_kardianos_osext",
    commit = "ae77be60afb1",
    importpath = "github.com/kardianos/osext",
)

go_repository(
    name = "com_github_kolide_kit",
    commit = "efbc10368270",
    importpath = "github.com/kolide/kit",
)

go_repository(
    name = "com_github_kolide_osquery_go",
    commit = "30c843ecb33a",
    importpath = "github.com/kolide/osquery-go",
)

go_repository(
    name = "com_github_kolide_updater",
    commit = "2c509cadd035",
    importpath = "github.com/kolide/updater",
)

go_repository(
    name = "com_github_konsorten_go_windows_terminal_sequences",
    importpath = "github.com/konsorten/go-windows-terminal-sequences",
    tag = "v1.0.1",
)

go_repository(
    name = "com_github_kr_logfmt",
    commit = "b84e30acd515",
    importpath = "github.com/kr/logfmt",
)

go_repository(
    name = "com_github_kr_pretty",
    importpath = "github.com/kr/pretty",
    tag = "v0.1.0",
)

go_repository(
    name = "com_github_kr_pty",
    importpath = "github.com/kr/pty",
    tag = "v1.1.2",
)

go_repository(
    name = "com_github_kr_text",
    importpath = "github.com/kr/text",
    tag = "v0.1.0",
)

go_repository(
    name = "com_github_lib_pq",
    importpath = "github.com/lib/pq",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_magiconair_properties",
    importpath = "github.com/magiconair/properties",
    tag = "v1.8.0",
)

go_repository(
    name = "com_github_mattn_go_sqlite3",
    importpath = "github.com/mattn/go-sqlite3",
    tag = "v1.9.0",
)

go_repository(
    name = "com_github_matttproud_golang_protobuf_extensions",
    importpath = "github.com/matttproud/golang_protobuf_extensions",
    tag = "v1.0.1",
)

go_repository(
    name = "com_github_microsoft_go_winio",
    importpath = "github.com/Microsoft/go-winio",
    tag = "v0.4.11",
)

go_repository(
    name = "com_github_miekg_pkcs11",
    commit = "57296583125b",
    importpath = "github.com/miekg/pkcs11",
)

go_repository(
    name = "com_github_mitchellh_mapstructure",
    importpath = "github.com/mitchellh/mapstructure",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_mixer_clock",
    commit = "b08e6b4da7ea",
    importpath = "github.com/mixer/clock",
)

go_repository(
    name = "com_github_oklog_run",
    importpath = "github.com/oklog/run",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_oklog_ulid",
    importpath = "github.com/oklog/ulid",
    tag = "v0.3.0",
)

go_repository(
    name = "com_github_onsi_ginkgo",
    importpath = "github.com/onsi/ginkgo",
    tag = "v1.7.0",
)

go_repository(
    name = "com_github_onsi_gomega",
    importpath = "github.com/onsi/gomega",
    tag = "v1.4.3",
)

go_repository(
    name = "com_github_pelletier_go_toml",
    importpath = "github.com/pelletier/go-toml",
    tag = "v1.2.0",
)

go_repository(
    name = "com_github_pkg_errors",
    importpath = "github.com/pkg/errors",
    tag = "v0.8.0",
)

go_repository(
    name = "com_github_pmezard_go_difflib",
    importpath = "github.com/pmezard/go-difflib",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_prometheus_client_golang",
    importpath = "github.com/prometheus/client_golang",
    tag = "v0.9.1",
)

go_repository(
    name = "com_github_prometheus_client_model",
    commit = "5c3871d89910",
    importpath = "github.com/prometheus/client_model",
)

go_repository(
    name = "com_github_prometheus_common",
    commit = "aeab699e26f4",
    importpath = "github.com/prometheus/common",
)

go_repository(
    name = "com_github_prometheus_procfs",
    commit = "185b4288413d",
    importpath = "github.com/prometheus/procfs",
)

go_repository(
    name = "com_github_shopify_logrus_bugsnag",
    commit = "577dee27f20d",
    importpath = "github.com/Shopify/logrus-bugsnag",
)

go_repository(
    name = "com_github_sirupsen_logrus",
    importpath = "github.com/Sirupsen/logrus",
    tag = "v1.0.3",
)

go_repository(
    name = "com_github_sirupsen_logrus",
    importpath = "github.com/sirupsen/logrus",
    tag = "v1.2.0",
)

go_repository(
    name = "com_github_spf13_afero",
    importpath = "github.com/spf13/afero",
    tag = "v1.1.2",
)

go_repository(
    name = "com_github_spf13_cast",
    importpath = "github.com/spf13/cast",
    tag = "v1.2.0",
)

go_repository(
    name = "com_github_spf13_jwalterweatherman",
    importpath = "github.com/spf13/jwalterweatherman",
    tag = "v1.0.0",
)

go_repository(
    name = "com_github_spf13_pflag",
    importpath = "github.com/spf13/pflag",
    tag = "v1.0.2",
)

go_repository(
    name = "com_github_spf13_viper",
    importpath = "github.com/spf13/viper",
    tag = "v1.2.1",
)

go_repository(
    name = "com_github_stretchr_objx",
    importpath = "github.com/stretchr/objx",
    tag = "v0.1.1",
)

go_repository(
    name = "com_github_stretchr_testify",
    importpath = "github.com/stretchr/testify",
    tag = "v1.2.2",
)

go_repository(
    name = "com_github_theupdateframework_notary",
    commit = "f234167994cf",
    importpath = "github.com/theupdateframework/notary",
)

go_repository(
    name = "com_github_watchbeam_clock",
    commit = "b08e6b4da7ea",
    importpath = "github.com/WatchBeam/clock",
)

go_repository(
    name = "com_google_cloud_go",
    importpath = "cloud.google.com/go",
    tag = "v0.33.1",
)

go_repository(
    name = "in_gopkg_airbrake_gobrake_v2",
    importpath = "gopkg.in/airbrake/gobrake.v2",
    tag = "v2.0.9",
)

go_repository(
    name = "in_gopkg_check_v1",
    commit = "788fd7840127",
    importpath = "gopkg.in/check.v1",
)

go_repository(
    name = "in_gopkg_dancannon_gorethink_v3",
    importpath = "gopkg.in/dancannon/gorethink.v3",
    tag = "v3.0.5",
)

go_repository(
    name = "in_gopkg_fatih_pool_v2",
    importpath = "gopkg.in/fatih/pool.v2",
    tag = "v2.0.0",
)

go_repository(
    name = "in_gopkg_fsnotify_v1",
    importpath = "gopkg.in/fsnotify.v1",
    tag = "v1.4.7",
)

go_repository(
    name = "in_gopkg_gemnasium_logrus_airbrake_hook_v2",
    importpath = "gopkg.in/gemnasium/logrus-airbrake-hook.v2",
    tag = "v2.1.2",
)

go_repository(
    name = "in_gopkg_gorethink_gorethink_v3",
    importpath = "gopkg.in/gorethink/gorethink.v3",
    tag = "v3.0.5",
)

go_repository(
    name = "in_gopkg_tomb_v1",
    commit = "dd632973f1e7",
    importpath = "gopkg.in/tomb.v1",
)

go_repository(
    name = "in_gopkg_yaml_v2",
    importpath = "gopkg.in/yaml.v2",
    tag = "v2.2.1",
)

go_repository(
    name = "org_apache_git_thrift_git",
    commit = "f12cacf56145",
    importpath = "git.apache.org/thrift.git",
    remote = "git@github.com:apache/thrift.git",
    vcs = "git",
)

go_repository(
    name = "org_golang_google_appengine",
    importpath = "google.golang.org/appengine",
    tag = "v1.3.0",
)

go_repository(
    name = "org_golang_google_genproto",
    commit = "4eb30f4778ee",
    importpath = "google.golang.org/genproto",
)

go_repository(
    name = "org_golang_google_grpc",
    importpath = "google.golang.org/grpc",
    tag = "v1.12.0",
)

go_repository(
    name = "org_golang_x_crypto",
    commit = "0709b304e793",
    importpath = "golang.org/x/crypto",
)

go_repository(
    name = "org_golang_x_net",
    commit = "161cd47e91fd",
    importpath = "golang.org/x/net",
)

go_repository(
    name = "org_golang_x_sync",
    commit = "1d60e4601c6f",
    importpath = "golang.org/x/sync",
)

go_repository(
    name = "org_golang_x_sys",
    commit = "d0be0721c37e",
    importpath = "golang.org/x/sys",
)

go_repository(
    name = "org_golang_x_text",
    importpath = "golang.org/x/text",
    tag = "v0.3.0",
)

go_repository(
    name = "org_golang_x_time",
    commit = "6dc17368e09b",
    importpath = "golang.org/x/time",
)
