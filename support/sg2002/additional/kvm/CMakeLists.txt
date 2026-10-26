list(APPEND ADD_REQUIREMENTS vision basic peripheral)
# 1-byte __atomic_exchange_n (kvm_vision.cpp) and libsys.so need libatomic on riscv64.
# It used to come in through the OpenCV libraries' link line.
list(APPEND ADD_REQUIREMENTS atomic)
list(APPEND ADD_INCLUDE "include")
list(APPEND ADD_LINK_DEFINITIONS_PRIVATE -Wl,--as-needed)

append_srcs_dir(ADD_SRCS "src")

register_component(DYNAMIC)
