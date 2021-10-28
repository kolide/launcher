module github.com/kolide/launcher

require (
	cloud.google.com/go v0.43.0
	github.com/Masterminds/semver v1.4.2
	github.com/Microsoft/go-winio v0.4.11 // indirect
	github.com/Shopify/logrus-bugsnag v0.0.0-20171204204709-577dee27f20d // indirect
	github.com/WatchBeam/clock v0.0.0-20170901150240-b08e6b4da7ea // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/alexkohler/nakedret v0.0.0-20171106223215-c0e305a4f690
	github.com/bitly/go-hostpool v0.0.0-20171023180738-a3a6125de932 // indirect
	github.com/bitly/go-simplejson v0.5.0 // indirect
	github.com/bmizerany/assert v0.0.0-20160611221934-b7ed37b82869 // indirect
	github.com/bugsnag/bugsnag-go v1.3.2 // indirect
	github.com/bugsnag/panicwrap v1.2.0 // indirect
	github.com/cenkalti/backoff v2.0.0+incompatible // indirect
	github.com/clbanning/mxj v1.8.4
	github.com/client9/misspell v0.3.4
	github.com/cloudflare/cfssl v0.0.0-20181102015659-ea4033a214e7 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/denisenkom/go-mssqldb v0.0.0-20181014144952-4e0d7dc8888f // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-metrics v0.0.0-20181218153428-b84716841b82 // indirect
	github.com/docker/libtrust v0.0.0-20160708172513-aabc10ec26b7 // indirect
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5 // indirect
	github.com/ghodss/yaml v1.0.0
	github.com/go-bindata/go-bindata v1.0.0
	github.com/go-ini/ini v1.61.0
	github.com/go-kit/kit v0.8.0
	github.com/go-ole/go-ole v1.2.4
	github.com/gogo/protobuf v1.2.0
	github.com/golang/protobuf v1.3.2
	github.com/google/certificate-transparency-go v1.0.21 // indirect
	github.com/google/fscrypt v0.3.0
	github.com/google/uuid v1.1.0
	github.com/gorilla/context v1.1.1 // indirect
	github.com/gorilla/mux v1.6.2 // indirect
	github.com/gorilla/websocket v1.4.2
	github.com/groob/plist v0.0.0-20190114192801-a99fbe489d03
	github.com/hailocab/go-hostpool v0.0.0-20160125115350-e80d13ce29ed // indirect
	github.com/jinzhu/gorm v1.9.1 // indirect
	github.com/jinzhu/inflection v0.0.0-20180308033659-04140366298a // indirect
	github.com/jinzhu/now v0.0.0-20181116074157-8ec929ed50c3 // indirect
	github.com/kardianos/osext v0.0.0-20170510131534-ae77be60afb1
	github.com/knightsc/system_policy v1.1.1-0.20191030190822-139971392acb
	github.com/kolide/kit v0.0.0-20191023141830-6312ecc11c23
	github.com/kolide/osquery-go v0.0.0-20190904034940-a74aa860032d
	github.com/kolide/updater v0.0.0-20190315001611-15bbc19b5b80
	github.com/konsorten/go-windows-terminal-sequences v1.0.2 // indirect
	github.com/kr/pty v1.1.2
	github.com/mat/besticon v3.9.0+incompatible
	github.com/mattn/go-sqlite3 v1.10.0
	github.com/miekg/pkcs11 v0.0.0-20180208123018-5f6e0d0dad6f // indirect
	github.com/mitchellh/go-ps v0.0.0-20170309133038-4fdf99ab2936
	github.com/mixer/clock v0.0.0-20170901150240-b08e6b4da7ea
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646
	github.com/oklog/run v1.0.0
	github.com/onsi/ginkgo v1.7.0 // indirect
	github.com/onsi/gomega v1.4.3 // indirect
	github.com/opencontainers/go-digest v1.0.0-rc1 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/peterbourgon/ff/v3 v3.0.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/client_golang v0.9.2 // indirect
	github.com/scjalliance/comshim v0.0.0-20190308082608-cf06d2532c4e
	github.com/serenize/snaker v0.0.0-20171204205717-a683aaf2d516
	github.com/sirupsen/logrus v1.4.0 // indirect
	github.com/smartystreets/goconvey v1.6.4 // indirect
	github.com/spf13/viper v1.2.1 // indirect
	github.com/stretchr/testify v1.5.1
	github.com/theupdateframework/notary v0.6.1
	github.com/tsenart/deadcode v0.0.0-20160724212837-210d2dc333e9
	go.etcd.io/bbolt v1.3.5
	go.opencensus.io v0.22.1
	golang.org/x/crypto v0.0.0-20190605123033-f99c8df09eb5
	golang.org/x/image v0.0.0-20190227222117-0694c2d4d067
	golang.org/x/net v0.0.0-20190827160401-ba9fcec4b297
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20211025201205-69cdffdb9359
	golang.org/x/text v0.3.2
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	google.golang.org/grpc v1.23.0
	gopkg.in/dancannon/gorethink.v3 v3.0.5 // indirect
	gopkg.in/fatih/pool.v2 v2.0.0 // indirect
	gopkg.in/gorethink/gorethink.v3 v3.0.5 // indirect
	gopkg.in/ini.v1 v1.61.0 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb
)

go 1.16
