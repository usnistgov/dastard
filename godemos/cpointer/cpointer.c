#include <stdlib.h>
#include <stdio.h>
#include <string.h>

char* echo(char* o) {
    char* out = (char*)malloc(strlen(o)+1);
    strcpy(out, o); // signature is destination, source
    return out;
}

char* echoAlign(char* o) {
    void *vout;
    posix_memalign(&vout, 4096, strlen(o)+1);
    char *out = vout;
    strcpy(out, o); // signature is destination, source
    return out;
}
