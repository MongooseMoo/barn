//go:build !windows
// +build !windows

package builtins

/*
#cgo LDFLAGS: -lcrypt
#define _GNU_SOURCE
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// On some systems crypt() is in crypt.h
#ifdef __linux__
#include <crypt.h>
#endif

// crypt_wrapper calls the system crypt(3) function
// We use a wrapper because crypt() might return a static buffer
char* crypt_wrapper(const char* key, const char* salt) {
    char* result = crypt(key, salt);
    if (result == NULL) {
        return NULL;
    }
    // Duplicate the result since crypt() returns a static buffer
    return strdup(result);
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// cryptDESPlatform uses the system crypt(3) function for traditional DES crypt
// This provides 100% compatibility with ToastStunt on Unix platforms
func cryptDESPlatform(password, salt string) (string, error) {
	cPassword := C.CString(password)
	cSalt := C.CString(salt)
	defer C.free(unsafe.Pointer(cPassword))
	defer C.free(unsafe.Pointer(cSalt))

	result := C.crypt_wrapper(cPassword, cSalt)
	if result == nil {
		return "", fmt.Errorf("crypt(3) failed")
	}
	defer C.free(unsafe.Pointer(result))

	return C.GoString(result), nil
}
