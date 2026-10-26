# maixlite (not upstream): MaixCDK components/3rd_party/yaml (yaml-cpp's src/*.cpp, in the
# alphabetical order MaixCDK's aux_source_directory gave).
set(_y yaml-cpp/src)
add_library(yaml STATIC
    ${_y}/binary.cpp
    ${_y}/convert.cpp
    ${_y}/depthguard.cpp
    ${_y}/directives.cpp
    ${_y}/emit.cpp
    ${_y}/emitfromevents.cpp
    ${_y}/emitter.cpp
    ${_y}/emitterstate.cpp
    ${_y}/emitterutils.cpp
    ${_y}/exceptions.cpp
    ${_y}/exp.cpp
    ${_y}/memory.cpp
    ${_y}/node.cpp
    ${_y}/node_data.cpp
    ${_y}/nodebuilder.cpp
    ${_y}/nodeevents.cpp
    ${_y}/null.cpp
    ${_y}/ostream_wrapper.cpp
    ${_y}/parse.cpp
    ${_y}/parser.cpp
    ${_y}/regex_yaml.cpp
    ${_y}/scanner.cpp
    ${_y}/scanscalar.cpp
    ${_y}/scantag.cpp
    ${_y}/scantoken.cpp
    ${_y}/simplekey.cpp
    ${_y}/singledocparser.cpp
    ${_y}/stream.cpp
    ${_y}/tag.cpp)
target_include_directories(yaml PRIVATE "${INC_YAML_SRC}" "${INC_YAML_INC}" "${CFG_DIR}")
