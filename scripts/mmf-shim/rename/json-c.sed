s/json/cvi_json/g
s/cvi_cvi_/cvi_/g
s/\barray_list_\(new2\|new\|free\|get_idx\|put_idx\|add\|length\|sort\|bsearch\|del_idx\|shrink\)\b/cvi_array_list_\1/g
s/\blh_\(table_[a-z_0-9]*\|kchar_table_new\|char_hash\|char_equal\|ptr_hash\|ptr_equal\|abort\)\b/cvi_lh_\1/g
s/\bprintbuf_\(new\|memappend\|memset\|reset\|free\)\b/cvi_printbuf_\1/g
s/\bsprintbuf\b/cvi_sprintbuf/g
s/\bmc_\(set_debug\|get_debug\|set_syslog\|debug\|error\|info\)\b/cvi_mc_\1/g
s/#include "\(arraylist\|debug\|linkhash\|printbuf\)\.h"/#include "cvi_\1.h"/
