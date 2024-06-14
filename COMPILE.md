# Compilation instructions

These are my raw instructions to compile nrpe_exporter with openssl statically linked but not glibc.

## openssl 1.1.1v

* Get openssl 1.1.1v:

  ```bash
  wget https://github.com/openssl/openssl/releases/download/OpenSSL_1_1_1v/openssl-1.1.1v.tar.gz
  ```

* untar archive file:

  ```bash
  tar xzf openssl-1.1.1v.tar.gz
  ```

* enter directory:

  ```bash
  cd openssl-1.1.1v
  ```

* init and compile:

  ```bash
  ./config --shared
  make
  ```

* alter pk config files for libraries:

  ```bash
  vi libssl.pc

  prefix=[where the directory_stand]/openssl-1.1.1v
  #prefix=/usr/local
  exec_prefix=${prefix}
  libdir=${exec_prefix}/lib64
  includedir=${prefix}/include

  Name: OpenSSL-libssl
  Description: Secure Sockets Layer and cryptography libraries
  Version: 1.1.1v
  Requires.private: libcrypto
  Libs: -L${libdir} -lssl
  Cflags: -I${includedir}



  vi libcrypto.pc

  prefix=[where the directory_stand]/openssl-1.1.1v
  #prefix=/usr/local
  exec_prefix=${prefix}
  libdir=${exec_prefix}/lib64
  includedir=${prefix}/include
  enginesdir=${libdir}/engines-1.1

  Name: OpenSSL-libcrypto
  Description: OpenSSL cryptography library
  Version: 1.1.1v
  Libs: -L${libdir} -lcrypto
  Libs.private: -ldl -pthread
  Cflags: -I${includedir}

  ```

* create a directory lib64:

  ```bash
  mkdir lib64
  ```

* add .pc and .a file to this directory:

  ```bash
  cd lib64
  ln -s ../libcrypto.a libcrypto.a
  ln -s ../libcrypto.pc libcrypto.pc
  ln -s ../libssl.pc libssl.pc
  ln -s ../libssl.a libssl.a
  ```

* enter nrpe_exporter directory:

  ```bash
  cd $GOPATH/src/nrpe_exporter
  ```

* set pkg confile path for openssl

  ```bash
  cd $GOPATH/src/nrpe_exporter
  export PKG_CONFIG_PATH=[where the directory_stand]/openssl-1.1.1v
  ```

* launch compilation either with promu tool or directly with go build:
  * promu:

    ```bash
    cd $GOPATH/src/nrpe_exporter
    export PKG_CONFIG_PATH=[where the directory_stand]/openssl-1.1.1v
    $GOBIN/promu build
    ```

  with promu installed with "go install github.com/prometheus/promu@latest"

  * go build:

    ```bash
    VERSION=`cat VERSION` && DATE=`date -u +"%Y%m%d-%T"`  go build -o nrpe_exporter -ldflags "-X github.com/prometheus/common/version.Version=$VERSION -X github.com/prometheus/common/version.Revision=42e4069950c7fc15fa53c51e0479c84700c28515 -X github.com/prometheus/common/version.Branch=profile_with_nrped_jfpik -X github.com/prometheus/common/version.BuildDate=$DATE -X github.com/prometheus/common/version.BuildUser=$USERNAME@$HOSTNAME" -tags openssl_static,netgo,usergo
    ```

## openssl 3.1.2

use openssl 3.1.2 only if you know what you do: require to **alter locally the openssl go package**!

* same step than for openssl 1.1.1v:

  ```bash
  wget https://github.com/openssl/openssl/releases/download/openssl-3.1.2/openssl-3.1.2.tar.gz
  tar xzf openssl-3.1.2.tar.gz
  cd openssl-3.1.2/
  ./config --shared
  make
  vi libssl.pc

  prefix=[where the directory_stand]/openssl-3.1.2
  #prefix=/usr/local
  exec_prefix=${prefix}
  libdir=${exec_prefix}/lib64
  includedir=${prefix}/include

  Name: OpenSSL-libssl
  Description: Secure Sockets Layer and cryptography libraries
  Version: 3.1.2
  Requires.private: libcrypto
  Libs: -L${libdir} -lssl
  Cflags: -I${includedir}


  vi libcrypto.pc
  prefix=[where the directory_stand]/openssl-3.1.2
  #prefix=/usr/local
  exec_prefix=${prefix}
  libdir=${exec_prefix}/lib64
  includedir=${prefix}/include
  enginesdir=${libdir}/engines-3
  modulesdir=${libdir}/ossl-modules

  Name: OpenSSL-libcrypto
  Description: OpenSSL cryptography library
  Version: 3.1.2
  Libs: -L${libdir} -lcrypto
  Libs.private: -ldl -pthread
  Cflags: -I${includedir}


  mkdir lib64
  cd lib64
  ln -s ../libcrypto.a libcrypto.a
  ln -s ../libcrypto.pc libcrypto.pc
  ln -s ../libssl.pc libssl.pc
  ln -s ../libssl.a libssl.a

  cd $GOPATH/src/nrpe_exporter
  export PKG_CONFIG_PATH=[where the directory_stand]/openssl-3.1.2
  ```

* alter openssl go package

FIPS 140-2 support has been removed from openssl 3 and is present in openssl 1.x version; because go portage is now obsolete the C source code is obsolete too. We have to remove this part (without any consequence on nrpe modules!)

* change permission on file and alter it

  ```bash
  chmod u+w $GOPATH/pkg/mod/github.com/spacemonkeygo/openssl@v0.0.0-20181017203307-c2dcc5cca94a/fips.go
  ```

* then edit file

  ```bash
  vi $GOPATH/pkg/mod/github.com/spacemonkeygo/openssl@v0.0.0-20181017203307-c2dcc5cca94a/fips.go

  // https://wiki.openssl.org/index.php/FIPS_mode_set()
  func FIPSModeSet(mode bool) error {
          runtime.LockOSThread()
          defer runtime.UnlockOSThread()

          var r C.int = 1
  /*
          var r C.int
          if mode {
                  r = C.FIPS_mode_set(1)
          } else {
                  r = C.FIPS_mode_set(0)
          }
  */
          if r != 1 {
                  return errorFromErrorQueue()
          }
          return nil
  }

  ```

* then build the exporter

  ```bash
  cd $GOPATH/src/nrpe_exporter
  export PKG_CONFIG_PATH=[where the directory_stand]/openssl-3.1.2

  $GOBIN/promu build
  ```

* verify library dependencies

  ```bash
  ldd nrpe_exporter

  linux-vdso.so.1 =>  (0x00007ffd9b3b3000)
  libdl.so.2 => /lib64/libdl.so.2 (0x00007fee5fcd1000)
  libpthread.so.0 => /lib64/libpthread.so.0 (0x00007fee5fab5000)
  libc.so.6 => /lib64/libc.so.6 (0x00007fee5f6e7000)
  /lib64/ld-linux-x86-64.so.2 (0x00007fee5fed5000)
  
  ```
