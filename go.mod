module github.com/kolide/launcher

require (
	fyne.io/systray v1.10.1-0.20221115204952-d16a6177e6f1
	github.com/Masterminds/semver v1.4.2
	github.com/Microsoft/go-winio v0.4.11
	github.com/clbanning/mxj v1.8.4
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v1.0.0
	github.com/go-ini/ini v1.61.0
	github.com/go-kit/kit v0.8.0
	github.com/go-ole/go-ole v1.2.6
	github.com/godbus/dbus/v5 v5.1.0
	github.com/golang/protobuf v1.5.2
	github.com/google/fscrypt v0.3.3
	github.com/google/uuid v1.1.2
	github.com/gorilla/websocket v1.4.2
	github.com/groob/plist v0.0.0-20190114192801-a99fbe489d03
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/knightsc/system_policy v1.1.1-0.20211029142728-5f4c0d5419cc
	github.com/kolide/kit v0.0.0-20221107170827-fb85e3d59eab
	github.com/kolide/krypto v0.0.0-20230128032426-052c6d0e7dfb
	github.com/kolide/updater v0.0.0-20190315001611-15bbc19b5b80
	github.com/kr/pty v1.1.2
	github.com/mat/besticon v3.9.0+incompatible
	github.com/mattn/go-sqlite3 v1.10.0
	github.com/mixer/clock v0.0.0-20170901150240-b08e6b4da7ea
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/oklog/run v1.0.0
	github.com/osquery/osquery-go v0.0.0-20220706183148-4e1f83012b42
	github.com/peterbourgon/ff/v3 v3.0.0
	github.com/pkg/errors v0.9.1
	github.com/scjalliance/comshim v0.0.0-20190308082608-cf06d2532c4e
	github.com/serenize/snaker v0.0.0-20171204205717-a683aaf2d516
	github.com/shirou/gopsutil v3.21.11+incompatible
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/stretchr/testify v1.8.1
	github.com/theupdateframework/notary v0.6.1
	github.com/tklauser/go-sysconf v0.3.10 // indirect
	github.com/yusufpapurcu/wmi v1.2.2 // indirect
	go.etcd.io/bbolt v1.3.6
	go.opencensus.io v0.23.0
	golang.org/x/crypto v0.4.0
	golang.org/x/exp v0.0.0-20221126150942-6ab00d035af9
	golang.org/x/image v0.3.0
	golang.org/x/net v0.4.0
	golang.org/x/sync v0.0.0-20220722155255-886fb9371eb4
	golang.org/x/sys v0.4.0
	golang.org/x/text v0.6.0
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	google.golang.org/grpc v1.38.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/toast.v1 v1.0.0-20180812000517-0a84660828b2
	howett.net/plist v0.0.0-20181124034731-591f970eefbb
	software.sslmate.com/src/go-pkcs12 v0.0.0-20210415151418-c5206de65a78
)

require github.com/vmihailenco/msgpack/v5 v5.3.5

require (
	github.com/BurntSushi/toml v1.1.0 // indirect
	github.com/Shopify/logrus-bugsnag v0.0.0-20171204204709-577dee27f20d // indirect
	github.com/WatchBeam/clock v0.0.0-20170901150240-b08e6b4da7ea // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/apache/thrift v0.16.0 // indirect
	github.com/bugsnag/bugsnag-go v1.3.2 // indirect
	github.com/bugsnag/panicwrap v1.2.0 // indirect
	github.com/cenkalti/backoff v2.0.0+incompatible // indirect
	github.com/cloudflare/cfssl v0.0.0-20181102015659-ea4033a214e7 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.8.0+incompatible // indirect
	github.com/docker/go v1.5.1-1 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.0-20181218153428-b84716841b82 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/go-logfmt/logfmt v0.4.0 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/google/certificate-transparency-go v1.0.21 // indirect
	github.com/google/go-tpm v0.3.3 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.6.2 // indirect
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	github.com/jinzhu/gorm v1.9.1 // indirect
	github.com/jinzhu/inflection v0.0.0-20180308033659-04140366298a // indirect
	github.com/kr/logfmt v0.0.0-20140226030751-b84e30acd515 // indirect
	github.com/miekg/pkcs11 v0.0.0-20180208123018-5f6e0d0dad6f // indirect
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/onsi/ginkgo v1.7.0 // indirect
	github.com/onsi/gomega v1.4.3 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/smartystreets/goconvey v1.6.4 // indirect
	github.com/spf13/viper v1.8.1 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/tevino/abool v1.2.0 // indirect
	github.com/tklauser/numcpus v0.4.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210602131652-f16073e35f0c // indirect
	google.golang.org/protobuf v1.26.0 // indirect
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	gopkg.in/dancannon/gorethink.v3 v3.0.5 // indirect
	gopkg.in/fatih/pool.v2 v2.0.0 // indirect
	gopkg.in/gorethink/gorethink.v3 v3.0.5 // indirect
	gopkg.in/ini.v1 v1.62.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

go 1.19
