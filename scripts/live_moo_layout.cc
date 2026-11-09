// Build against the exact deployed headers/configuration. No server code runs.
#include <cstddef>
#include <cstdio>
#include "execute.h"

int main()
{
    std::printf("{\"version\":1,\"pointer_size\":%zu,\"num_size\":%zu,"
                "\"int_size\":%zu,\"activation_size\":%zu,\"type_obj\":%d,"
                "\"type_anon\":%d,\"var_value\":%zu,\"var_type\":%zu,\"offsets\":{",
                sizeof(void *), sizeof(Num), sizeof(int), sizeof(activation),
                TYPE_OBJ, TYPE_ANON, offsetof(Var, v), offsetof(Var, type));
#define FIELD(name) std::printf("\"" #name "\":%zu,", offsetof(activation, name))
    FIELD(prog);
    FIELD(error_pc);
    FIELD(player);
    FIELD(progr);
    FIELD(recv);
    FIELD(vloc);
    FIELD(verb);
    FIELD(verbname);
    std::printf("\"bi_func_pc\":%zu}}\n", offsetof(activation, bi_func_pc));
}
