package process

/*
#include <string.h>
#include <libproc.h>
#include <sys/types.h>

// Defined in process_darwin.c
int find_process_path_by_pid(pid_t pid, char *buf, size_t buflen);
int find_process_name_by_pid(pid_t pid, char *buf, size_t buflen);
*/
import "C"

import (
	"fmt"
	"path/filepath"
)

// procPathMaxsize sets the buffer length for proc_name. It's not defined
// in libproc.h, and various codebases use values from 64 to 4096, but 1024 is likely ok.
const procPathMaxsize = 1024

func pidExecutablePath(pid PID) (string, error) {
	var pathBuf [C.PROC_PIDPATHINFO_MAXSIZE]C.char
	ret := C.find_process_path_by_pid(C.pid_t(pid), &pathBuf[0], C.size_t(len(pathBuf)))
	if ret < 0 {
		return "", fmt.Errorf("find path for pid %d: %s", pid, C.GoString(C.strerror(-ret)))
	}
	return C.GoString(&pathBuf[0]), nil
}

func pidName(pid PID, executablePath string) (string, error) {
	var nameBuf [procPathMaxsize]C.char
	ret := C.find_process_name_by_pid(C.pid_t(pid), &nameBuf[0], C.size_t(len(nameBuf)))
	if ret >= 0 {
		if name := C.GoString(&nameBuf[0]); name != "" {
			return name, nil
		}
	}

	path := executablePath
	var err error
	if path == "" {
		path, err = pidExecutablePath(pid)
	}
	if err != nil {
		if ret < 0 {
			return "", fmt.Errorf("find name for pid %d: %s", pid, C.GoString(C.strerror(-ret)))
		}
		return "", err
	}
	if path == "" && ret < 0 {
		return "", fmt.Errorf("find name for pid %d: %s", pid, C.GoString(C.strerror(-ret)))
	}
	return filepath.Base(path), nil
}
