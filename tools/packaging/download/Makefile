all: clean-darwin dl-darwin extract-darwin tar-darwin dl-linux tar-linux

clean-darwin:
	rm -rf *.pkg
	rm -rf tmp/darwin

dl-darwin:
	curl -LO https://osquery-packages.s3.amazonaws.com/darwin/osquery-latest.pkg

extract-darwin:
	mkdir -p tmp/
	pkgutil --expand osquery-latest.pkg tmp/darwin
	cd tmp/darwin && cat Payload|gunzip -dc |cpio -i && cd ..

VERSION:=$(shell ./tmp/darwin/usr/local/bin/osqueryd --version | awk '{print $$3}')
tar-darwin:
	cd tmp/darwin/usr/local/bin && tar -czvf osqueryd-stable.tar.gz osqueryd
	cd tmp/darwin/usr/local/bin && tar -czvf osqueryd-${VERSION}.tar.gz osqueryd
	mkdir -p mirror/binaries-for-launcher/kolide/osqueryd/darwin/
	mv tmp/darwin/usr/local/bin/*.tar.gz mirror/binaries-for-launcher/kolide/osqueryd/darwin/

dl-linux:
	curl -LO https://osquery-packages.s3.amazonaws.com/linux/osquery-${VERSION}_1.linux_x86_64.tar.gz
	mkdir -p tmp/linux
	tar -zxvf *.tar.gz -C tmp/linux/

VERSION:=$(shell ./tmp/darwin/usr/local/bin/osqueryd --version | awk '{print $$3}')
tar-linux:
	cd tmp/linux/usr/bin && tar -czvf osqueryd-stable.tar.gz osqueryd
	cd tmp/linux/usr/bin && tar -czvf osqueryd-${VERSION}.tar.gz osqueryd
	mkdir -p mirror/binaries-for-launcher/kolide/osqueryd/linux/
	mv tmp/linux/usr/bin/*.tar.gz mirror/binaries-for-launcher/kolide/osqueryd/linux/

