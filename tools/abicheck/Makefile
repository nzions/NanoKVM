# abicheck - see README.md. Not wired into forge yet.
#
#   make check OSDRV_V2_TGZ=... OSDRV_V1_TGZ=... MW_4E231EF=... MW_CD8BB74=...
#
# Every variable is a path you provide; nothing is fetched here.
OSDRV_V2_TGZ ?= osdrv.tar.gz
OSDRV_V1_TGZ ?= osdrv2.tar.gz
MW_4E231EF   ?= mw-4e231ef
MW_CD8BB74   ?= mw-cd8bb74
KVM_SRC      ?= ../../support/sg2002/additional/kvm_mmf/src/kvm_mmf.cpp
OUT          ?= out

.PHONY: check clean
check:
	./abicheck.sh --osdrv-v2-tgz $(OSDRV_V2_TGZ) --osdrv-v1-tgz $(OSDRV_V1_TGZ) \
		--mw-4e231ef $(MW_4E231EF) --mw-cd8bb74 $(MW_CD8BB74) \
		--kvm-src $(KVM_SRC) --out $(OUT)

clean:
	rm -rf $(OUT)

# ioctls.inc is generated from ioctls.def (one #ifdef per request) and committed.
.PHONY: regen
regen:
	sed -n '/^IOCTL/s/^IOCTL(\(.*\))$$/#ifdef \1\n\tioctl_line("\1", (unsigned long)(\1));\n#else\n\tmissing_line("\1");\n#endif/p' ioctls.def > ioctls.inc
