package picoclaw

import (
	"time"
)

const (
	picoclawBinaryPath = "/usr/bin/picoclaw"
	// picoclawExplicitStartEnv tells S96picoclaw this start is not the boot-time
	// init run, so the persisted enable need not be checked.
	picoclawExplicitStartEnv = "NANOKVM_PICOCLAW_EXPLICIT"
	picoclawCacheDir         = "/root/.picoclaw-cache"
	picoclawPIDFileName      = ".picoclaw.pid"
	// picoclawHomeDir is the gateway's home: owned by the unprivileged
	// picoclaw user, not world-readable (it holds the model api key).
	// picoclawLegacyHomeDir is where releases before F6 kept it, as root.
	picoclawHomeDir       = "/etc/kvm/picoclaw"
	picoclawLegacyHomeDir = "/root/.picoclaw"
	// The runtime is fetched from <update base>/picoclaw/v<version>/<archive>
	// with its signature at <archive URL>.sig. The signed name is
	// picoclaw-<version>-riscv64 (updatesign.PicoclawName).
	picoclawVersion         = "0.2.8"
	picoclawArchiveName     = "picoclaw_Linux_riscv64.tar.gz"
	picoclawMaxArchiveBytes = int64(256 << 20)
	etcInitPicoclawScript   = "/etc/init.d/S96picoclaw"
	kvmappPicoclawScript    = "/kvmapp/system/init.d/S96picoclaw"
	picoclawStartTimeout    = 15 * time.Second
	picoclawStopTimeout     = 15 * time.Second
	picoclawOnboardTimeout  = 60 * time.Second
	picoclawInstallTimeout  = 12 * time.Minute
	picoclawDownloadTimeout = 10 * time.Minute
	picoclawStartWaitPeriod = 1500 * time.Millisecond
	picoclawStopWaitPeriod  = 500 * time.Millisecond
	picoclawReadyPollPeriod = 500 * time.Millisecond
)
