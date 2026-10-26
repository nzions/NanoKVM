# NanoKVM Server

This is the backend server implementation for NanoKVM.

For detailed documentation, please visit our [Wiki](https://wiki.sipeed.com/nanokvm).

## Structure

```shell
server
├── common       // Common utility components
├── config       // Server configuration
├── dl_lib       // Shared object libraries
├── include      // Header files for shared objects
├── logger       // Logging system
├── middleware   // Server middleware components
├── proto        // API request/response definitions
├── router       // API route handlers
├── service      // Core service implementations
├── utils        // Utility functions
└── main.go
```

## Configuration

The configuration file path is `/etc/kvm/server.yaml`.

```yaml
# Network Settings
proto: https           # Access protocol, `https` or `http`. Default is `https`; a self-signed certificate is generated at startup when the files below are missing. Existing `http` configs are left as they are
host: ""               # The listening address for the HTTP/HTTPS service. If left empty, all network interfaces will be bound
port:
    http: 80           # The listening port for the HTTP service. Default is `80`
    https: 443         # The listening port for the HTTPS service (effective when HTTPS is enabled). Default is `443`
cert:
    crt: /etc/kvm/server.crt  # The path to the public key certificate for HTTPS
    key: /etc/kvm/server.key  # The path to the private key file for HTTPS


# Logging Configuration
logger:
    level: info     # Global log output level. Evaluated options from highest to lowest detail: `trace`, `debug`, `info`, `warn`, `error`, `fatal`, `panic`. Default is `info`
    file: stdout    # Log output destination. `stdout` outputs to the standard console. A file path directs log output to that file. Default is `stdout`


# Authentication & Security
authentication: enable              # Whether to enable identity verification for HTTP API and Web endpoints. Options are `enable` or `disable`. Default is `enable`. Highly recommended to leave this enabled for internet-facing devices!
jwt:
   secretKey: ""                    # The secret key used to sign and verify JWT Tokens. If left empty, a random key is generated once and kept in `/etc/kvm/.jwt_secret` (0600), so restarts do not log users out
   refreshTokenDuration: 2678400    # The token refresh duration threshold in seconds before forcing a re-login. Default is `2678400` (~31 days)
   revokeTokensOnLogout: true       # Whether logout invalidates all sessions belonging to that user. Other users are never logged out. Setting this to false only clears the browser cookie and is not recommended. Default is `true`
security:
   loginLockoutDuration: 0,         # The duration (in seconds) to ban an IP from attempting to log in again after reaching the failure limit. If set to `0` or left empty, brute-force protection is disabled. Default is `0`
   loginMaxFailures:     5,         # The maximum number of continuous failed login attempts allowed per IP before triggering protection. Default is `5`
   trustedProxies: []               # IPs/CIDRs of reverse proxies whose `X-Forwarded-Proto` header is believed (matched against the TCP peer). Empty trusts nobody. Default is empty
   corsOrigins: []                  # Origins allowed cross-origin access. Empty means same-origin only. Default is empty


# WebRTC Traversal Settings
stun: stun.l.google.com:19302 # The default STUN server address used for NAT hole-punching to establish P2P streams
turn:
    turnAddr: example_addr    # The relay (TURN) server address (format `ip:port`) used as a fallback when P2P connection fails. Leave empty to disable TURN relay
    turnUser: example_user    # The username required for authorization to the TURN server
    turnCred: example_cred    # The credential/password required for authorization to the TURN server
```

## Web Login

There is one web account, `root`, and its password is root's Linux password: the hash in
`/etc/shadow` is the only copy. The login verifies crypt `$6$` (SHA-512), `$5$` (SHA-256) and
`$1$` (MD5) hashes; changing the password in **Settings > Account** (current password
required) rewrites root's shadow entry with a `$6$` hash and revokes every session. A password
is 1-256 bytes and contains no `:`, newline or NUL (crypt runs thousands of hashing rounds, so
the length is bounded on the unauthenticated login route).

`/etc/kvm/pwd` (mode `0600`) holds only the session generation and whether the next login must
change the password. Releases up to 2.5.15 kept a multi-user account store there; the first
start of a later release replaces it (root's shadow entry is kept as it is, other accounts are
dropped) and refuses, leaving the old file, if root's hash is not one of the three types above.
Deleting the file (the physical BOOT-button password reset does) re-seeds root.

First boot: `/boot/root.pass` (one line, written from a Mac) becomes root's password and is
consumed. Without it a random 12-character password is set, shown on the OLED (it is also
`/etc/kvm/initial.pass`, `0600`), and the first login must change it.

## Compile & Deploy

Note: The manual steps below require a Linux x86-64 host with Go 1.25 or newer; they are not compatible with ARM, Windows or macOS. With Docker you can skip them entirely and use the containerized flow instead — the root [Makefile](../Makefile) (`make shell`) or the dev container (see "Development" in the root [README](../README.md)) — which works on any host OS; run `server/build.sh` inside the container for a release-equivalent build.

1. Install the Toolchain
    1. Download the toolchain from the following link: [Download Link](https://sophon-file.sophon.cn/sophon-prod-s3/drive/23/03/07/16/host-tools.tar.gz).
    2. Extract the file and add the `host-tools/gcc/riscv64-linux-musl-x86_64/bin` directory to your PATH environment variable.
    3. Run `riscv64-unknown-linux-musl-gcc -v`. If there is version information in the output, the installation is successful.

2. Compile the Project
    1. Run `cd server` from the project root directory.
    2. Run `go mod tidy` to install Go dependencies.
    3. (Optional) If you compiled `libkvm.so` yourself, you need to modify its RPATH by `patchelf --add-rpath \$ORIGIN ./dl_lib/libkvm.so`.
    4. Run `CGO_ENABLED=1 GOOS=linux GOARCH=riscv64 CC=riscv64-unknown-linux-musl-gcc CGO_CFLAGS="-mcpu=c906fdv -march=rv64imafdcv0p7xthead -mcmodel=medany -mabi=lp64d" go build` to compile the project.
    5. After compilation, an executable file named `NanoKVM-Server` will be generated.

3. Modify RPATH
    1. Run `sudo apt install patchelf` or `pip install patchelf` to install patchelf.
    2. Run `patchelf --version`. Ensure the version is 0.14 or higher`.
    3. Run `patchelf --add-rpath \$ORIGIN/dl_lib NanoKVM-Server` to modify the RPATH of the executable file.

4. Deploy the Application
    1. File uploads requires SSH. Please enable it in the Web Settings: `Settings > SSH`;
    2. Replace the original file in the NanoKVM `/kvmapp/server/` directory with the newly compiled `NanoKVM-Server`.
    3. Restart the service on NanoKVM by executing `/etc/init.d/S95nanokvm restart`.

## Manually Update

> File uploads requires SSH. Please enable it in the Web Settings: `Settings > SSH`;

1. Download the latest application from [GitHub](https://github.com/sipeed/NanoKVM/releases);
2. Unzip the downloaded file and rename the unzipped folder to `kvmapp`;
3. Back up the existing `/kvmapp` directory on your NanoKVM, then replace it with the new `kvmapp` folder;
4. Run `/etc/init.d/S95nanokvm restart` on your NanoKVM to restart the service.
